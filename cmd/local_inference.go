package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/idapt/idapt-cli/internal/resolve"
	"github.com/spf13/cobra"
)

const defaultLocalInferenceBaseURL = "http://127.0.0.1:11434/v1"
const defaultDaemonConfigPath = "/etc/idapt/config.json"
const localInferenceLongHTTPTimeout = 31 * time.Minute

var localInferenceCmd = &cobra.Command{
	Use:     "local-inference",
	Aliases: []string{"local"},
	Short:   "Manage daemon-backed local Ollama inference",
	Annotations: map[string]string{
		"instructions": `# local-inference — instructions

Use this for private self-hosted models attached to an idapt computer daemon.
Runtime traffic stays on that computer: the app publishes daemon commands and
the daemon talks to the local Ollama loopback port.

Recommended setup path:

1. ` + "`local-inference install`" + ` — downloads the managed Ollama bundle into the per-OS user data dir (Linux ` + "`~/.local/share/idapt/local-inference`" + `, macOS ` + "`~/Library/Application Support/idapt/local-inference`" + `, Windows ` + "`%LocalAppData%\\idapt\\local-inference`" + `). Override with ` + "`IDAPT_LOCAL_INFERENCE_HOME`" + `.
2. ` + "`local-inference start`" + ` — starts Ollama bound to 127.0.0.1 with models in the same idapt-owned directory.
3. ` + "`local-inference pull <model>`" + ` — pulls a model through Ollama.
4. ` + "`local-inference setup --model <ollama-model> --idapt-model <model-id>`" + ` — creates a private daemon provider endpoint for routing chat.

Use an existing Ollama daemon by passing ` + "`--managed=false`" + ` to start
and keeping ` + "`--base-url`" + ` pointed at its loopback OpenAI-compatible
URL. Use ` + "`--computer <computer>`" + ` to target a different paired daemon.
Public marketplace serving is intentionally not exposed here.`,
	},
}

type localInferenceDaemonConfig struct {
	ComputerID         string `json:"computerId"`
	ComputerResourceID string `json:"computerResourceId"`
	Domain            string `json:"domain"`
}

func localInferenceColumns() []output.Column {
	return []output.Column{
		{Header: "RUNTIME", Field: "runtime"},
		{Header: "MODE", Field: "mode"},
		{Header: "RUNNING", Field: "running"},
		{Header: "BASE_URL", Field: "base_url", Width: 40},
		{Header: "VERSION", Field: "version"},
		{Header: "GPU", Field: "gpu"},
		{Header: "MODELS_DIR", Field: "models_dir", Width: 60},
	}
}

func localInferenceComputerAction(cmd *cobra.Command, computerArg string, body map[string]interface{}) (map[string]interface{}, error) {
	f := cmdutil.FactoryFromCmd(cmd)
	client, err := f.APIClient()
	if err != nil {
		return nil, err
	}
	computerID, err := resolveLocalInferenceComputer(cmd, f, computerArg)
	if err != nil {
		return nil, err
	}
	if _, ok := body["base_url"]; !ok {
		baseURL, _ := cmd.Flags().GetString("base-url")
		if baseURL != "" {
			body["base_url"] = baseURL
		}
	}
	if action, _ := body["action"].(string); action == "install" || action == "pull_model" || action == "chat" {
		client = client.WithTimeout(localInferenceLongHTTPTimeout)
	}
	var resp api.V1ItemResponse
	if err := client.Post(cmd.Context(), "/api/v1/computers/"+computerID+"/local-inference", body, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

func localInferenceComputerActionStream(cmd *cobra.Command, computerArg string, body map[string]interface{}) (map[string]interface{}, error) {
	f := cmdutil.FactoryFromCmd(cmd)
	client, err := f.APIClient()
	if err != nil {
		return nil, err
	}
	client = client.WithTimeout(localInferenceLongHTTPTimeout)
	computerID, err := resolveLocalInferenceComputer(cmd, f, computerArg)
	if err != nil {
		return nil, err
	}
	if _, ok := body["base_url"]; !ok {
		baseURL, _ := cmd.Flags().GetString("base-url")
		if baseURL != "" {
			body["base_url"] = baseURL
		}
	}
	body["stream_progress"] = true
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(
		cmd.Context(),
		"POST",
		"/api/v1/computers/"+computerID+"/local-inference",
		bytes.NewReader(data),
		api.WithHeader("Content-Type", "application/json"),
		api.WithHeader("Accept", "application/x-ndjson"),
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	progressOut := io.Discard
	if f.Format == output.FormatTable {
		progressOut = f.ErrOut
	}
	var result map[string]interface{}
	state := localInferenceProgressState{lastPercent: -1}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var frame localInferenceStreamFrame
		if err := json.Unmarshal(line, &frame); err != nil {
			return nil, fmt.Errorf("local inference stream decode: %w", err)
		}
		switch frame.Type {
		case "progress":
			renderLocalInferenceProgress(progressOut, frame.Data, &state)
		case "result":
			result = frame.Data
		case "error":
			if frame.Error != nil && frame.Error.Message != "" {
				return nil, errors.New(frame.Error.Message)
			}
			return nil, fmt.Errorf("local inference command failed")
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("local inference stream ended without a result")
	}
	return result, nil
}

type localInferenceStreamFrame struct {
	Type  string                 `json:"type"`
	Data  map[string]interface{} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type localInferenceProgressState struct {
	lastPhase   string
	lastPercent int
}

func resolveLocalInferenceComputer(cmd *cobra.Command, f *cmdutil.Factory, positional string) (string, error) {
	if flagged, _ := cmd.Flags().GetString("computer"); flagged != "" {
		if positional != "" {
			return "", fmt.Errorf("use either --computer or a positional computer, not both")
		}
		return resolveComputer(cmd, f, flagged)
	}
	if positional != "" {
		return resolveComputer(cmd, f, positional)
	}
	cfg, path, err := readLocalDaemonConfig(cmd)
	if err != nil {
		return "", err
	}
	if cfg.ComputerResourceID != "" {
		return cfg.ComputerResourceID, nil
	}
	if cfg.Domain != "" {
		if slug := computerSlugFromDomain(cfg.Domain); slug != "" {
			if computerID, err := resolveComputer(cmd, f, slug); err == nil {
				return computerID, nil
			}
		}
	}
	if cfg.ComputerID != "" {
		return "", fmt.Errorf("local daemon config at %s is paired but does not contain a public computer id; re-run `idapt pair` with a current CLI or pass --computer <computer>", path)
	}
	return "", fmt.Errorf("local daemon config at %s does not contain a computer id; pass --computer <computer>", path)
}

func readLocalDaemonConfig(cmd *cobra.Command) (localInferenceDaemonConfig, string, error) {
	path, _ := cmd.Flags().GetString("daemon-config")
	if path == "" {
		path = os.Getenv("IDAPT_DAEMON_CONFIG")
	}
	if path == "" {
		path = defaultDaemonConfigPath
	}
	content, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return localInferenceDaemonConfig{}, path, fmt.Errorf("no paired Idapt daemon found on this computer; pass --computer <computer> or run `idapt pair --token <token>` first (looked for %s)", path)
		}
		return localInferenceDaemonConfig{}, path, fmt.Errorf("read local daemon config %s: %w", path, err)
	}
	var cfg localInferenceDaemonConfig
	if err := json.Unmarshal(content, &cfg); err != nil {
		return localInferenceDaemonConfig{}, path, fmt.Errorf("parse local daemon config %s: %w", path, err)
	}
	return cfg, path, nil
}

func computerSlugFromDomain(domain string) string {
	host := strings.TrimSpace(domain)
	if host == "" {
		return ""
	}
	if strings.Contains(host, "://") {
		return ""
	}
	host = strings.Split(host, ":")[0]
	parts := strings.Split(host, ".")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func optionalComputerArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func parseModelTargetArgs(cmd *cobra.Command, args []string, verb string) (string, string, error) {
	if len(args) < 1 || len(args) > 2 {
		return "", "", fmt.Errorf("usage: idapt local-inference %s <model> [--computer <computer>]", verb)
	}
	if flagged, _ := cmd.Flags().GetString("computer"); flagged != "" {
		if len(args) != 1 {
			return "", "", fmt.Errorf("when --computer is set, use `idapt local-inference %s <model> --computer <computer>`", verb)
		}
		return "", args[0], nil
	}
	if len(args) == 1 {
		return "", args[0], nil
	}
	return args[0], args[1], nil
}

func parseAskTargetArgs(cmd *cobra.Command, args []string) (string, string, string, error) {
	if len(args) < 2 {
		return "", "", "", fmt.Errorf("usage: idapt local-inference ask <model> <prompt> [--computer <computer>]")
	}
	if flagged, _ := cmd.Flags().GetString("computer"); flagged != "" {
		return "", args[0], strings.Join(args[1:], " "), nil
	}
	if len(args) >= 3 && isLikelyLegacyAskTarget(args[0], args[1]) {
		return args[0], args[1], strings.Join(args[2:], " "), nil
	}
	return "", args[0], strings.Join(args[1:], " "), nil
}

func isLikelyLegacyAskTarget(computerArg string, _ string) bool {
	return resolve.IsResourceId(computerArg)
}

func renderLocalInferenceProgress(w io.Writer, event map[string]interface{}, state *localInferenceProgressState) {
	if w == io.Discard {
		return
	}
	phase, _ := event["phase"].(string)
	status, _ := event["status"].(string)
	if status == "" {
		status = phase
	}
	percent := -1
	if v, ok := event["percent"].(float64); ok {
		percent = int(math.Floor(v))
	}
	if phase == state.lastPhase && percent >= 0 && percent == state.lastPercent {
		return
	}
	state.lastPhase = phase
	state.lastPercent = percent

	if downloaded, ok := event["downloadedBytes"].(float64); ok {
		if total, ok := event["totalBytes"].(float64); ok && total > 0 {
			fmt.Fprintf(w, "%s: %s / %s (%d%%)", status, formatBytes(downloaded), formatBytes(total), percent)
		} else {
			fmt.Fprintf(w, "%s: %s", status, formatBytes(downloaded))
		}
		if speed, ok := event["speedBytesPerSecond"].(float64); ok && speed > 0 {
			fmt.Fprintf(w, " at %s/s", formatBytes(speed))
		}
		if eta, ok := event["etaSeconds"].(float64); ok && eta > 0 {
			fmt.Fprintf(w, ", ETA %s", formatDuration(eta))
		}
		if resumed, _ := event["resumed"].(bool); resumed {
			fmt.Fprint(w, " (resumed)")
		}
		fmt.Fprintln(w)
		return
	}
	if existing, ok := event["existingBytes"].(float64); ok && existing > 0 {
		fmt.Fprintf(w, "%s: resuming from %s\n", status, formatBytes(existing))
		return
	}
	if total, ok := event["totalBytes"].(float64); ok && total > 0 {
		fmt.Fprintf(w, "%s: %s\n", status, formatBytes(total))
		return
	}
	if status != "" {
		fmt.Fprintln(w, status)
	}
}

func formatBytes(value float64) string {
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%.0f B", value)
	}
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	v := value / unit
	for _, suffix := range units {
		if v < unit {
			return fmt.Sprintf("%.1f %s", v, suffix)
		}
		v /= unit
	}
	return fmt.Sprintf("%.1f PiB", v)
}

func formatDuration(seconds float64) string {
	d := time.Duration(seconds * float64(time.Second)).Round(time.Second)
	if d < time.Minute {
		return d.String()
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
}

var localInferenceStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Ollama status through the computer daemon",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		item, err := localInferenceComputerAction(cmd, optionalComputerArg(args), map[string]interface{}{"action": "status"})
		if err != nil {
			return err
		}
		return f.Formatter().WriteItem(item, localInferenceColumns())
	},
}

var localInferenceInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the managed Ollama runtime into idapt app data",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		body := map[string]interface{}{"action": "install", "managed": true}
		if cmd.Flags().Changed("version") {
			version, _ := cmd.Flags().GetString("version")
			body["version"] = version
		}
		item, err := localInferenceComputerActionStream(cmd, optionalComputerArg(args), body)
		if err != nil {
			return err
		}
		return f.Formatter().WriteItem(item, localInferenceColumns())
	},
}

var localInferenceStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start Ollama through the computer daemon",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		managed, _ := cmd.Flags().GetBool("managed")
		item, err := localInferenceComputerAction(cmd, optionalComputerArg(args), map[string]interface{}{
			"action":  "start",
			"managed": managed,
		})
		if err != nil {
			return err
		}
		return f.Formatter().WriteItem(item, localInferenceColumns())
	},
}

var localInferenceStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the managed Ollama runtime",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		item, err := localInferenceComputerAction(cmd, optionalComputerArg(args), map[string]interface{}{"action": "stop"})
		if err != nil {
			return err
		}
		return f.Formatter().WriteItem(item, localInferenceColumns())
	},
}

var localInferenceLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Show managed Ollama logs",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		item, err := localInferenceComputerAction(cmd, optionalComputerArg(args), map[string]interface{}{"action": "logs"})
		if err != nil {
			return err
		}
		if f.Format == output.FormatTable {
			fmt.Fprint(f.Out, item["logs"])
			return nil
		}
		return f.Formatter().WriteItem(item, []output.Column{{Header: "LOGS", Field: "logs", Width: 120}})
	},
}

var localInferenceModelsCmd = &cobra.Command{
	Use:   "models",
	Short: "List Ollama models",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		item, err := localInferenceComputerAction(cmd, optionalComputerArg(args), map[string]interface{}{"action": "list_models"})
		if err != nil {
			return err
		}
		rows, _ := item["models"].([]interface{})
		models := make([]map[string]interface{}, 0, len(rows))
		for _, row := range rows {
			if m, ok := row.(map[string]interface{}); ok {
				models = append(models, m)
			}
		}
		return f.Formatter().WriteList(models, []output.Column{
			{Header: "NAME", Field: "name"},
			{Header: "SIZE", Field: "size"},
			{Header: "MODIFIED_AT", Field: "modified_at"},
		})
	},
}

var localInferencePullCmd = &cobra.Command{
	Use:   "pull <model>",
	Short: "Pull an Ollama model",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		computerArg, model, err := parseModelTargetArgs(cmd, args, "pull")
		if err != nil {
			return err
		}
		item, err := localInferenceComputerAction(cmd, computerArg, map[string]interface{}{
			"action": "pull_model",
			"model":  model,
		})
		if err != nil {
			return err
		}
		return f.Formatter().WriteItem(item, []output.Column{
			{Header: "OK", Field: "ok"},
			{Header: "PROGRESS", Field: "progress", Width: 120},
		})
	},
}

var localInferenceRemoveCmd = &cobra.Command{
	Use:   "remove <model>",
	Short: "Remove an Ollama model",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		computerArg, model, err := parseModelTargetArgs(cmd, args, "remove")
		if err != nil {
			return err
		}
		item, err := localInferenceComputerAction(cmd, computerArg, map[string]interface{}{
			"action": "remove_model",
			"model":  model,
		})
		if err != nil {
			return err
		}
		return f.Formatter().WriteItem(item, []output.Column{{Header: "OK", Field: "ok"}})
	},
}

var localInferenceAskCmd = &cobra.Command{
	Use:   "ask <model> <prompt>",
	Short: "Prompt an Ollama model through the daemon",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		computerArg, model, prompt, err := parseAskTargetArgs(cmd, args)
		if err != nil {
			return err
		}
		item, err := localInferenceComputerAction(cmd, computerArg, map[string]interface{}{
			"action": "chat",
			"model":  model,
			"prompt": prompt,
		})
		if err != nil {
			return err
		}
		if f.Format == output.FormatTable {
			fmt.Fprintln(f.Out, item["content"])
			return nil
		}
		return f.Formatter().WriteItem(item, []output.Column{
			{Header: "CONTENT", Field: "content", Width: 120},
			{Header: "RAW", Field: "raw", Width: 120},
		})
	},
}

var localInferenceSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Install, start, pull a model, and create a daemon provider endpoint",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		computerArg := optionalComputerArg(args)
		computerID, err := resolveLocalInferenceComputer(cmd, f, computerArg)
		if err != nil {
			return err
		}
		model, _ := cmd.Flags().GetString("model")
		if model == "" {
			return fmt.Errorf("--model is required")
		}
		idaptModel, _ := cmd.Flags().GetString("idapt-model")
		if idaptModel == "" {
			idaptModel = "ollama/" + model
		}
		baseURL, _ := cmd.Flags().GetString("base-url")
		if baseURL == "" {
			baseURL = defaultLocalInferenceBaseURL
		}
		name, _ := cmd.Flags().GetString("name")
		if name == "" {
			name = "Local Ollama"
		}
		skipInstall, _ := cmd.Flags().GetBool("skip-install")
		skipPull, _ := cmd.Flags().GetBool("skip-pull")
		if !skipInstall {
			if _, err := localInferenceComputerActionStream(cmd, computerArg, map[string]interface{}{
				"action":   "install",
				"base_url": baseURL,
				"managed":  true,
			}); err != nil {
				return err
			}
		}
		if _, err := localInferenceComputerAction(cmd, computerArg, map[string]interface{}{
			"action":   "start",
			"base_url": baseURL,
			"managed":  true,
		}); err != nil {
			return err
		}
		if !skipPull {
			if _, err := localInferenceComputerAction(cmd, computerArg, map[string]interface{}{
				"action":   "pull_model",
				"base_url": baseURL,
				"model":    model,
			}); err != nil {
				return err
			}
		}
		var resp api.V1ItemResponse
		err = client.Post(cmd.Context(), "/api/v1/provider-endpoints", map[string]interface{}{
			"kind":             "openai_compatible",
			"display_name":     name,
			"transport":        "daemon",
			"runtime":          "ollama",
			"protocol":         "openai_compatible",
			"computer_id":       computerID,
			"local_base_url":   baseURL,
			"default_for_kind": true,
			"model_mappings": []map[string]interface{}{
				{"model_id": idaptModel, "api_model_id": model},
			},
		}, &resp)
		if err != nil {
			return err
		}
		return f.Formatter().WriteItem(resp.Data, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "DISPLAY_NAME", Field: "display_name"},
			{Header: "TRANSPORT", Field: "transport"},
			{Header: "COMPUTER_ID", Field: "computer_id"},
			{Header: "LOCAL_BASE_URL", Field: "local_base_url"},
			{Header: "MODEL_MAPPINGS", Field: "model_mappings", Width: 80},
		})
	},
}

func init() {
	localInferenceCmd.PersistentFlags().String("computer", "", "Target a specific computer instead of the local paired daemon")
	localInferenceCmd.PersistentFlags().String("daemon-config", "", "Path to local daemon config for no-arg local inference commands (default /etc/idapt/config.json or IDAPT_DAEMON_CONFIG)")
	for _, cmd := range []*cobra.Command{
		localInferenceStatusCmd,
		localInferenceInstallCmd,
		localInferenceStartCmd,
		localInferenceStopCmd,
		localInferenceLogsCmd,
		localInferenceModelsCmd,
		localInferencePullCmd,
		localInferenceRemoveCmd,
		localInferenceAskCmd,
		localInferenceSetupCmd,
	} {
		cmd.Flags().String("base-url", defaultLocalInferenceBaseURL, "Local OpenAI-compatible Ollama base URL")
		localInferenceCmd.AddCommand(cmd)
	}
	localInferenceInstallCmd.Flags().String("version", "", "Managed Ollama version (reserved; omit for current bundle)")
	localInferenceStartCmd.Flags().Bool("managed", true, "Use idapt-managed Ollama binary and model directory")
	localInferenceSetupCmd.Flags().String("model", "", "Ollama model to pull and map, e.g. llama3.2:1b")
	localInferenceSetupCmd.Flags().String("idapt-model", "", "Idapt model id to map to the Ollama model")
	localInferenceSetupCmd.Flags().String("name", "", "Provider endpoint display name")
	localInferenceSetupCmd.Flags().Bool("skip-install", false, "Skip managed Ollama install")
	localInferenceSetupCmd.Flags().Bool("skip-pull", false, "Skip model pull")

	rootCmd.AddCommand(localInferenceCmd)
}
