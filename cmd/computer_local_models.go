package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/spf13/cobra"
)

var computerLocalModelsCmd = &cobra.Command{
	Use:     "local-models",
	Aliases: []string{"local-inference"},
	Short:   "Install and manage Ollama models on this computer",
	Annotations: map[string]string{
		"instructions": `# computer local-models — instructions

Manage Ollama models installed on a paired computer. The computer
must have a paired Idapt daemon and Local Inference enabled (FF60).

## Verbs

- ` + "`list <computer>`" + ` — show installed models with current state
  (downloading, installed, loaded, …).
- ` + "`install <computer> <ollama-id>`" + ` — install or re-install. The
  pull runs on the daemon side; this command returns once the
  install is queued and prints the model id you can use with ` +
			"`watch`" + `.
- ` + "`remove <computer> <ollama-id>`" + ` — uninstall (frees disk).
- ` + "`watch <computer>`" + ` — subscribe to the user's event stream and
  print state transitions in real time.

## Notes

- Idempotent: re-installing an existing model is a no-op.
- The model becomes routable for chat as soon as state reaches
  ` + "`installed`" + ` — Ollama lazy-loads into memory on first request.`,
	},
}

func init() {
	computerRemoteCmd.AddCommand(computerLocalModelsCmd)
	computerLocalModelsCmd.AddCommand(computerLocalModelsListCmd)
	computerLocalModelsCmd.AddCommand(computerLocalModelsInstallCmd)
	computerLocalModelsCmd.AddCommand(computerLocalModelsRemoveCmd)
	computerLocalModelsCmd.AddCommand(computerLocalModelsWatchCmd)
}

var computerLocalModelsListCmd = &cobra.Command{
	Use:   "list <computer>",
	Short: "List installed Ollama models on a computer",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		id, err := resolveComputer(cmd, f, args[0])
		if err != nil {
			return err
		}

		path := "/api/v1/computers/" + id + "/local-inference/models"
		var resp struct {
			Data struct {
				Models []map[string]interface{} `json:"models"`
			} `json:"data"`
		}
		if err := client.Get(cmd.Context(), path, nil, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteList(resp.Data.Models, []output.Column{
			{Header: "OLLAMA_ID", Field: "ollamaId"},
			{Header: "STATE", Field: "currentState"},
			{Header: "QUANT", Field: "quantization"},
			{Header: "SIZE_BYTES", Field: "sizeBytes"},
			{Header: "LAST_LOADED", Field: "lastLoadedAt"},
		})
	},
}

var computerLocalModelsInstallCmd = &cobra.Command{
	Use:   "install <computer> <ollama-id>",
	Short: "Install an Ollama model on a computer",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		id, err := resolveComputer(cmd, f, args[0])
		if err != nil {
			return err
		}
		body := map[string]interface{}{"ollama_id": args[1]}
		path := "/api/v1/computers/" + id + "/local-inference/models"
		var resp struct {
			Data struct {
				ModelID          string `json:"model_id"`
				Created          bool   `json:"created"`
				AlreadyInstalled bool   `json:"already_installed"`
			} `json:"data"`
		}
		if err := client.Post(cmd.Context(), path, body, &resp); err != nil {
			return err
		}
		if resp.Data.AlreadyInstalled {
			fmt.Fprintf(f.Out, "Already installed: %s\n", args[1])
			return nil
		}
		fmt.Fprintf(
			f.Out,
			"Install queued: %s (model_id=%s). Run `idapt computer local-models watch %s` to follow.\n",
			args[1],
			resp.Data.ModelID,
			args[0],
		)
		return nil
	},
}

var computerLocalModelsRemoveCmd = &cobra.Command{
	Use:   "remove <computer> <ollama-id>",
	Short: "Remove an installed Ollama model from a computer",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		id, err := resolveComputer(cmd, f, args[0])
		if err != nil {
			return err
		}
		path := "/api/v1/computers/" + id + "/local-inference/models/" +
			url.PathEscape(args[1])
		if err := client.Delete(cmd.Context(), path); err != nil {
			return err
		}
		fmt.Fprintf(f.Out, "Removed: %s\n", args[1])
		return nil
	},
}

var computerLocalModelsWatchCmd = &cobra.Command{
	Use:   "watch <computer>",
	Short: "Stream local-inference model state changes in real time",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		publicID, err := resolveComputer(cmd, f, args[0])
		if err != nil {
			return err
		}
		fmt.Fprintf(
			f.Out,
			"Watching %s for local-inference state changes (Ctrl+C to stop)…\n",
			args[0],
		)

		reader, err := client.StreamSSEGet(
			cmd.Context(),
			"/api/user/subscribe",
			api.WithHeartbeat(45*time.Second),
		)
		if err != nil {
			return err
		}
		defer reader.Close()

		for {
			ev, rerr := reader.Next()
			if errors.Is(rerr, io.EOF) {
				return nil
			}
			if rerr != nil {
				return rerr
			}
			if ev.Event != "computers" {
				continue
			}
			var payload struct {
				Type       string                 `json:"type"`
				ComputerID string                 `json:"computerId"`
				OllamaID   string                 `json:"ollamaId"`
				State      string                 `json:"state"`
				Detail     map[string]interface{} `json:"detail"`
			}
			if err := json.Unmarshal([]byte(ev.Data), &payload); err != nil {
				continue
			}
			if payload.Type != "computers:local-inference-state-changed" {
				continue
			}
			if payload.ComputerID != publicID {
				continue
			}
			ts := time.Now().Format("15:04:05")
			detail := ""
			if len(payload.Detail) > 0 {
				b, _ := json.Marshal(payload.Detail)
				detail = " " + string(b)
			}
			fmt.Fprintf(
				f.Out,
				"[%s] %s %s%s\n",
				ts,
				payload.OllamaID,
				strings.ReplaceAll(payload.State, "_", " "),
				detail,
			)
		}
	},
}
