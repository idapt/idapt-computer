package cmd

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/coder/websocket"
	"github.com/idapt/idapt-computer/internal/cmdutil"
	"github.com/idapt/idapt-computer/internal/tunnel"
	"github.com/spf13/cobra"
)

var sshCmd = &cobra.Command{
	Use:   "ssh <[user@]computer> [ssh args...]",
	Short: "SSH into a computer through the idapt tunnel",
	Long: "Open an SSH session to a computer over the idapt tunnel — no public\n" +
		"IP or open firewall port required. Wraps the system `ssh` with an\n" +
		"`idapt-computer ssh-proxy` ProxyCommand; extra arguments pass through to ssh.\n" +
		"Computer names are unique per workspace; if a name exists in more than\n" +
		"one of your workspaces, set IDAPT_WORKSPACE=<workspaceId> to disambiguate.",
	Args:               cobra.MinimumNArgs(1),
	DisableFlagParsing: true,
	RunE:               runSSH,
}

var sshProxyWorkspace string

var sshProxyCmd = &cobra.Command{
	Use:    "ssh-proxy <computer>",
	Short:  "Pipe an SSH connection through the idapt tunnel (used as an ssh ProxyCommand)",
	Args:   cobra.ExactArgs(1),
	Hidden: true,
	RunE:   runSSHProxy,
}

func init() {
	sshProxyCmd.Flags().StringVar(&sshProxyWorkspace, "workspace", "",
		"Workspace id to disambiguate a computer name used in multiple workspaces")
	rootCmd.AddCommand(sshCmd)
	rootCmd.AddCommand(sshProxyCmd)
}

func runSSH(cmd *cobra.Command, args []string) error {
	target := args[0]
	computer := target
	if at := strings.LastIndex(target, "@"); at >= 0 {
		computer = target[at+1:]
	}
	if computer == "" {
		return fmt.Errorf("invalid target %q: expected [user@]computer", target)
	}
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve idapt binary path: %w", err)
	}

	sshArgs := []string{
		"-o", fmt.Sprintf("ProxyCommand=%s ssh-proxy %s", self, computer),
		target,
	}
	sshArgs = append(sshArgs, args[1:]...)

	sshProc := exec.Command("ssh", sshArgs...)
	sshProc.Stdin = os.Stdin
	sshProc.Stdout = os.Stdout
	sshProc.Stderr = os.Stderr
	return sshProc.Run()
}

func runSSHProxy(cmd *cobra.Command, args []string) error {
	computer := args[0]
	f := cmdutil.FactoryFromCmd(cmd)
	client, err := f.APIClient()
	if err != nil {
		return err
	}

	var resp struct {
		Data struct {
			Token    string `json:"token"`
			ProxyURL string `json:"proxy_url"`
		} `json:"data"`
	}
	tokenPath := "/api/v1/computers/" + url.PathEscape(computer) + "/ssh-token"
	workspace := sshProxyWorkspace
	if workspace == "" {
		workspace = os.Getenv("IDAPT_WORKSPACE")
	}
	if workspace != "" {
		tokenPath += "?workspace=" + url.QueryEscape(workspace)
	}
	if err := client.Get(cmd.Context(), tokenPath, nil, &resp); err != nil {
		return fmt.Errorf("request ssh token: %w", err)
	}
	if resp.Data.Token == "" || resp.Data.ProxyURL == "" {
		return fmt.Errorf("backend did not return an ssh token for %q", computer)
	}

	wsURL := strings.TrimRight(resp.Data.ProxyURL, "/") + "/__tunnel/ssh"
	ctx := cmd.Context()
	wsConn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": {"Bearer " + resp.Data.Token}},
	})
	if err != nil {
		return fmt.Errorf("connect tunnel-proxy: %w", err)
	}
	defer func() { _ = wsConn.Close(websocket.StatusNormalClosure, "") }()

	conn := tunnel.WebSocketNetConn(ctx, wsConn)
	errc := make(chan error, 2)
	go func() { _, e := io.Copy(conn, os.Stdin); errc <- e }()
	go func() { _, e := io.Copy(os.Stdout, conn); errc <- e }()
	<-errc
	return nil
}
