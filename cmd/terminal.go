package cmd

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"

	"github.com/coder/websocket"
	"github.com/idapt/idapt-computer/internal/cmdutil"
	"github.com/idapt/idapt-computer/internal/tunnel"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var computerCmd = &cobra.Command{
	Use:   "computer",
	Short: "Work with paired computers",
}

var (
	terminalRunAs     string
	terminalTmux      bool
	terminalWorkspace string
)

var terminalCmd = &cobra.Command{
	Use:   "terminal <[user@]computer>",
	Short: "Open an interactive terminal on a computer",
	Long: "Open an interactive shell on a computer over the idapt tunnel — no\n" +
		"host sshd, no keys, no open inbound port. The daemon allocates a real\n" +
		"pseudo-terminal and runs the shell directly. Use --tmux to attach the\n" +
		"shared idapt-{user} session the agent uses; otherwise a fresh login\n" +
		"shell opens. Computer names are unique per workspace; set --workspace\n" +
		"(or IDAPT_WORKSPACE) to disambiguate a name used in more than one.",
	Args: cobra.ExactArgs(1),
	RunE: runTerminal,
}

func init() {
	terminalCmd.Flags().StringVar(&terminalRunAs, "runas", "",
		"Unix user to run the shell as (defaults to the computer's default user)")
	terminalCmd.Flags().BoolVar(&terminalTmux, "tmux", false,
		"Attach the shared idapt-{user} tmux session instead of a fresh shell")
	terminalCmd.Flags().StringVar(&terminalWorkspace, "workspace", "",
		"Workspace id to disambiguate a computer name used in multiple workspaces")
	computerCmd.AddCommand(terminalCmd)
	rootCmd.AddCommand(computerCmd)
}

func runTerminal(cmd *cobra.Command, args []string) error {
	computer := args[0]
	if at := strings.LastIndex(computer, "@"); at >= 0 {
		if terminalRunAs == "" {
			terminalRunAs = computer[:at]
		}
		computer = computer[at+1:]
	}
	if computer == "" {
		return fmt.Errorf("invalid target: expected [user@]computer")
	}

	f := cmdutil.FactoryFromCmd(cmd)
	client, err := f.APIClient()
	if err != nil {
		return err
	}

	mode := "shell"
	if terminalTmux {
		mode = "tmux"
	}

	cols, rows := 80, 24
	if w, h, gerr := term.GetSize(int(os.Stdout.Fd())); gerr == nil && w > 0 && h > 0 {
		cols, rows = w, h
	}

	tokenQuery := url.Values{"mode": {mode}}
	if terminalRunAs != "" {
		tokenQuery.Set("runAs", terminalRunAs)
	}
	ws := terminalWorkspace
	if ws == "" {
		ws = os.Getenv("IDAPT_WORKSPACE")
	}
	if ws != "" {
		tokenQuery.Set("workspace", ws)
	}
	tokenPath := "/api/v1/computers/" + url.PathEscape(computer) + "/pty-token?" + tokenQuery.Encode()

	var resp struct {
		Data struct {
			Token    string `json:"token"`
			ProxyURL string `json:"proxy_url"`
			RunAs    string `json:"run_as"`
			Mode     string `json:"mode"`
		} `json:"data"`
	}
	if err := client.Get(cmd.Context(), tokenPath, nil, &resp); err != nil {
		return fmt.Errorf("request terminal token: %w", err)
	}
	if resp.Data.Token == "" || resp.Data.ProxyURL == "" {
		return fmt.Errorf("backend did not return a terminal token for %q", computer)
	}

	wsURL := strings.TrimRight(resp.Data.ProxyURL, "/") + "/__tunnel/pty?" + url.Values{
		"mode": {resp.Data.Mode},
		"cols": {fmt.Sprintf("%d", cols)},
		"rows": {fmt.Sprintf("%d", rows)},
	}.Encode()

	ctx := cmd.Context()
	wsConn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": {"Bearer " + resp.Data.Token}},
	})
	if err != nil {
		return fmt.Errorf("connect terminal: %w", err)
	}
	defer func() { _ = wsConn.Close(websocket.StatusNormalClosure, "") }()
	conn := tunnel.WebSocketNetConn(ctx, wsConn)

	var writeMu sync.Mutex
	writeFrame := func(fn func(io.Writer) error) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return fn(conn)
	}

	stdinFd := int(os.Stdin.Fd())
	oldState, rawErr := term.MakeRaw(stdinFd)
	restore := func() {
		if rawErr == nil {
			_ = term.Restore(stdinFd, oldState)
		}
	}
	defer restore()

	winch := make(chan os.Signal, 1)
	notifyWinch(winch)
	defer signal.Stop(winch)
	go func() {
		for range winch {
			if w, h, gerr := term.GetSize(int(os.Stdout.Fd())); gerr == nil && w > 0 {
				_ = writeFrame(func(wr io.Writer) error {
					return tunnel.WritePTYResize(wr, uint16(w), uint16(h))
				})
			}
		}
	}()

	go func() {
		buf := make([]byte, 4096)
		for {
			n, rerr := os.Stdin.Read(buf)
			if n > 0 {
				chunk := append([]byte(nil), buf[:n]...)
				if werr := writeFrame(func(wr io.Writer) error {
					return tunnel.WritePTYData(wr, chunk)
				}); werr != nil {
					return
				}
			}
			if rerr != nil {
				return
			}
		}
	}()

	exitCode := 0
	for {
		ft, payload, rerr := tunnel.ReadPTYFrame(conn)
		if rerr != nil {
			break
		}
		if ft == tunnel.PTYFrameData {
			_, _ = os.Stdout.Write(payload)
		} else if ft == tunnel.PTYFrameExit {
			if code, ok := tunnel.ParsePTYExit(payload); ok {
				exitCode = int(code)
			}
			break
		}
	}

	restore()
	if exitCode != 0 {
		os.Exit(exitCode)
	}
	return nil
}
