package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/idapt/idapt-computer/internal/cliconfig"
	"github.com/spf13/cobra"
)

var configCliCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage CLI configuration",
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Set a CLI configuration value.

Valid keys:
  apiUrl              API base URL (default: https://idapt.app)
  defaultWorkspace    Workspace slug used when --workspace is omitted
  outputFormat        table | json | jsonl | quiet
  noColor             true | false — disable ANSI colors

API keys are NOT stored here — use ` + "`idapt-computer auth login --api-key <key>`" + `
or set the IDAPT_API_KEY env var.`,
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: completeConfigKeys,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := cliconfig.DefaultPath()
		if err != nil {
			return fmt.Errorf("cannot determine config path: %w", err)
		}
		cfg, err := cliconfig.Load(path)
		if err != nil {
			cfg = cliconfig.Defaults()
		}

		if err := cfg.Set(args[0], args[1]); err != nil {
			return err
		}

		if err := cliconfig.Save(path, cfg); err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Set %s = %s\n", args[0], args[1])
		return nil
	},
}

var configGetCmd = &cobra.Command{
	Use:               "get [key]",
	Short:             "Get a configuration value",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeConfigKeys,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath, err := cliconfig.DefaultPath()
		if err != nil {
			return fmt.Errorf("cannot determine config path: %w", err)
		}
		cfg, err := cliconfig.Load(cfgPath)
		if err != nil {
			return err
		}

		if len(args) == 0 {
			for _, key := range cliconfig.Keys() {
				val, _ := cfg.Get(key)
				fmt.Fprintf(cmd.OutOrStdout(), "%s = %s\n", key, val)
			}
			return nil
		}

		val, ok := cfg.Get(args[0])
		if !ok {
			return fmt.Errorf("unknown config key %q; valid keys: %s", args[0], strings.Join(cliconfig.Keys(), ", "))
		}
		fmt.Fprintln(cmd.OutOrStdout(), val)
		return nil
	},
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configuration values",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath, err := cliconfig.DefaultPath()
		if err != nil {
			return fmt.Errorf("cannot determine config path: %w", err)
		}
		cfg, err := cliconfig.Load(cfgPath)
		if err != nil {
			return err
		}
		for _, key := range cliconfig.Keys() {
			val, _ := cfg.Get(key)
			fmt.Fprintf(cmd.OutOrStdout(), "%s = %s\n", key, val)
		}
		return nil
	},
}

var configUnsetCmd = &cobra.Command{
	Use:   "unset <key>",
	Short: "Clear a configuration value",
	Long: `Reset a CLI configuration value to its default (removes it from the file).

Use this to undo a ` + "`config set`" + ` — e.g. ` + "`idapt-computer config unset defaultWorkspace`" + `
restores the personal-workspace fallback.`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeConfigKeys,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := cliconfig.DefaultPath()
		if err != nil {
			return fmt.Errorf("cannot determine config path: %w", err)
		}
		cfg, err := cliconfig.Load(path)
		if err != nil {
			cfg = cliconfig.Defaults()
		}
		if err := cfg.Unset(args[0]); err != nil {
			return err
		}
		if err := cliconfig.Save(path, cfg); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Unset %s\n", args[0])
		return nil
	},
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the path to the CLI config file",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := cliconfig.DefaultPath()
		if err != nil {
			return fmt.Errorf("cannot determine config path: %w", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), path)
		return nil
	},
}

var configEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Open the CLI config file in your editor",
	Long: `Open the CLI config file in your editor.

Picks the editor from $VISUAL, then $EDITOR, then a sensible platform default
(nano/vi on Unix, notepad on Windows). The file is created with defaults first
if it does not exist yet.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := cliconfig.DefaultPath()
		if err != nil {
			return fmt.Errorf("cannot determine config path: %w", err)
		}
		if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
			cfg, _ := cliconfig.Load(path)
			if err := cliconfig.Save(path, cfg); err != nil {
				return fmt.Errorf("create config file: %w", err)
			}
		}

		bin, editorArgs, err := buildEditorCommand(os.Getenv("VISUAL"), os.Getenv("EDITOR"), path)
		if err != nil {
			return err
		}
		editor := exec.Command(bin, editorArgs...)
		editor.Stdin = os.Stdin
		editor.Stdout = os.Stdout
		editor.Stderr = os.Stderr
		return editor.Run()
	},
}

func buildEditorCommand(visual, editor, path string) (bin string, args []string, err error) {
	candidate := visual
	if candidate == "" {
		candidate = editor
	}
	if candidate == "" {
		candidate = defaultEditorCandidate()
	}
	fields := strings.Fields(candidate)
	if len(fields) == 0 {
		return "", nil, fmt.Errorf("no editor found — set $EDITOR or $VISUAL")
	}
	if _, lookErr := exec.LookPath(fields[0]); lookErr != nil {
		return "", nil, fmt.Errorf("editor %q not found — set $EDITOR or $VISUAL to an installed editor", fields[0])
	}
	args = append(fields[1:], path)
	return fields[0], args, nil
}

func defaultEditorCandidate() string {
	var fallbacks []string
	if runtime.GOOS == "windows" {
		fallbacks = []string{"notepad"}
	} else {
		fallbacks = []string{"nano", "vim", "vi"}
	}
	for _, e := range fallbacks {
		if _, err := exec.LookPath(e); err == nil {
			return e
		}
	}
	return ""
}

func init() {
	configCliCmd.AddCommand(configSetCmd)
	configCliCmd.AddCommand(configGetCmd)
	configCliCmd.AddCommand(configListCmd)
	configCliCmd.AddCommand(configUnsetCmd)
	configCliCmd.AddCommand(configPathCmd)
	configCliCmd.AddCommand(configEditCmd)
}
