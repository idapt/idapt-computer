package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

var (
	osOpenFile = func(path string) (io.ReadCloser, error) { return os.Open(path) }
	decodeJSON = func(r io.Reader, v any) error { return json.NewDecoder(r).Decode(v) }
)

func basename(p string) string { return filepath.Base(p) }

var runChatAskFromRootFlag = func(cmd *cobra.Command, prompt string) error {
	return runChatAsk(cmd, []string{prompt})
}

var chatAskCmd = &cobra.Command{
	Use:   "ask [message]",
	Short: "Send a one-shot message to a new (or existing) chat",
	Long: `Send a single message and stream the response.

If [message] is omitted (or "-"), the prompt is read from stdin. With --chat-id,
the message is appended to an existing chat; otherwise a fresh chat is created.

Streaming is enabled by default when stdout is a TTY; pipe-to-file or --no-stream
falls back to sync POST.

Examples:
  idapt chat ask "explain this regex"
  cat file.go | idapt chat ask "explain"
  idapt -p "explain this regex"   # equivalent root-flag shortcut

Exit codes:
  0  success
  2  auth failure
  3  network
  4  spending cap reached
  5  rate limit
`,
	RunE: runChatAsk,
}

func runChatAsk(cmd *cobra.Command, args []string) error {
	f := cmdutil.FactoryFromCmd(cmd)
	if err := cmdutil.RequireAuth(f); err != nil {
		return err
	}
	client, err := f.APIClient()
	if err != nil {
		return err
	}

	prompt, err := readPrompt(cmd, args)
	if err != nil {
		return err
	}
	if strings.TrimSpace(prompt) == "" {
		return errors.New("empty prompt")
	}

	chatID, _ := cmd.Flags().GetString("chat-id")
	model, _ := cmd.Flags().GetString("model")
	agent, _ := cmd.Flags().GetString("agent")
	workspace, _ := cmd.Flags().GetString("workspace")
	effort, _ := cmd.Flags().GetString("effort-level")
	branch, _ := cmd.Flags().GetString("branch-from")
	files, _ := cmd.Flags().GetStringArray("file")
	jsonMode := f.Format == "json" || f.Format == "jsonl"
	stream := defaultStreamMode(cmd)

	if effort != "" && !validEffortLevel(effort) {
		return fmt.Errorf("invalid --effort-level %q (must be fast, smart, or expert)", effort)
	}
	if agent == "" {
		agent = f.Config.LastAgentID
	}
	if model == "" {
		model = f.Config.LastModelID
	}
	if workspace == "" {
		workspace = f.Config.DefaultWorkspace
	}

	if workspace != "" {
		if resolved, rerr := resolveWorkspaceID(cmd, client, workspace); rerr == nil {
			workspace = resolved
		}
	}

	fileIDs := []string{}
	for _, path := range files {
		if path == "" || path == "[]" {
			continue
		}
		id, uerr := uploadOne(cmd, client, path, workspace)
		if uerr != nil {
			return fmt.Errorf("uploading %s: %w", path, uerr)
		}
		fileIDs = append(fileIDs, id)
	}

	req := ChatRunRequest{
		ChatID:      chatID,
		NewChat:     chatID == "",
		Text:        prompt,
		ModelID:     model,
		AgentID:     agent,
		WorkspaceID:   workspace,
		EffortLevel: effort,
		BranchFrom:  branch,
		FileIDs:     fileIDs,
		StreamMode:  stream,
		JSONMode:    jsonMode,
		NoColor:     f.NoColor,
		Out:         cmd.OutOrStdout(),
	}
	_, err = RunChat(cmd.Context(), client, req)
	if err != nil {
		if apiErr, ok := err.(*api.APIError); ok {
			return apiErr
		}
		return err
	}
	return nil
}

func defaultStreamMode(cmd *cobra.Command) bool {
	if v, _ := cmd.Flags().GetBool("no-stream"); v {
		return false
	}
	if cmd.Flags().Changed("stream") {
		v, _ := cmd.Flags().GetBool("stream")
		return v
	}
	out := os.Stdout
	if w, ok := cmd.OutOrStdout().(*os.File); ok && w != nil {
		out = w
	}
	return isatty.IsTerminal(out.Fd())
}

func readPrompt(cmd *cobra.Command, args []string) (string, error) {
	if len(args) > 0 && args[0] != "-" {
		return strings.Join(args, " "), nil
	}
	r := cmd.InOrStdin()
	if r == nil {
		return "", errors.New("no prompt: pass it as an arg, --json -, or via stdin")
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func uploadOne(cmd *cobra.Command, client *api.Client, path, workspaceID string) (string, error) {
	f, err := osOpenFile(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	uploadPath := "/api/v1/drive/files"
	fields := map[string]string{"name": basename(path)}
	if workspaceID != "" {
		fields["workspace_id"] = workspaceID
	}
	resp, err := client.Upload(cmd.Context(), uploadPath, basename(path), f, fields)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("upload failed: status %d", resp.StatusCode)
	}
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := decodeJSON(resp.Body, &env); err != nil {
		return "", fmt.Errorf("decoding upload response: %w", err)
	}
	id, _ := env.Data["id"].(string)
	if id == "" {
		return "", fmt.Errorf("upload response missing id")
	}
	return id, nil
}

func init() {
	chatAskCmd.Flags().String("chat-id", "", "Append to this existing chat instead of creating a fresh one")
	chatAskCmd.Flags().String("model", "", "Model id ('auto' or concrete)")
	chatAskCmd.Flags().String("agent", "", "Agent name or id")
	chatAskCmd.Flags().String("effort-level", "", "fast | smart | expert")
	chatAskCmd.Flags().String("branch-from", "", "Branch from this message id")
	chatAskCmd.Flags().StringArray("file", nil, "Attach a file (repeatable)")
	chatAskCmd.Flags().Bool("stream", true, "Stream the response (default true on TTY)")
	chatAskCmd.Flags().Bool("no-stream", false, "Force synchronous mode")

	_ = chatAskCmd.RegisterFlagCompletionFunc("chat-id", completeChatIDs)
	_ = chatAskCmd.RegisterFlagCompletionFunc("model", completeModelIDs)
	_ = chatAskCmd.RegisterFlagCompletionFunc("agent", completeAgentIDs)
	_ = chatAskCmd.RegisterFlagCompletionFunc("effort-level", completeEffortLevel)

}
