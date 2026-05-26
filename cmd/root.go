package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cliconfig"
	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/credential"
	"github.com/idapt/idapt-cli/internal/features"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/idapt/idapt-cli/internal/tui"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

var globalFlags *cmdutil.GlobalFlags

var rootCmd = &cobra.Command{
	Use:   "idapt",
	Short: "idapt CLI — AI workspace from the terminal",
	Long:  "idapt is a CLI tool and per-computer daemon for the idapt platform.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if err := MaybePrintInstructions(cmd); err != nil {
			return err
		}

		if isDaemonCommand(cmd) {
			return nil
		}

		var cfg cliconfig.Config
		if cfgPath, err := cliconfig.DefaultPath(); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %v (config file unavailable)\n", err)
			cfg = cliconfig.Defaults()
		} else if loaded, err := cliconfig.Load(cfgPath); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not load config: %v\n", err)
			cfg = cliconfig.Defaults()
		} else {
			cfg = loaded
		}

		var creds credential.Credentials
		var credPath string
		if cp, err := credential.DefaultPath(); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %v (credentials file unavailable)\n", err)
		} else {
			credPath = cp
			if loaded, err := credential.Load(credPath); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not load credentials: %v\n", err)
			} else {
				creds = loaded
			}
		}

		apiKey := globalFlags.APIKey
		apiKeySource := ""
		if apiKey != "" {
			apiKeySource = "flag"
		}
		if apiKey == "" {
			envKey := os.Getenv("IDAPT_API_KEY")
			if envKey != "" && !strings.HasPrefix(envKey, "mk_") {
				apiKey = envKey
				apiKeySource = "env"
			}
		}
		if apiKey == "" {
			apiKey = creds.APIKey
			if apiKey != "" {
				apiKeySource = "file"
			}
		}

		if globalFlags.Verbose && apiKey != "" {
			masked := apiKey[:min(len(apiKey), 6)] + "..."
			fmt.Fprintf(cmd.ErrOrStderr(), "Auth: using %s from %s\n", masked, apiKeySource)
		}
		if globalFlags.Verbose && apiKey == "" {
			fmt.Fprintf(cmd.ErrOrStderr(), "Auth: no API key found (flag=%v, env=%v, file=%q, fileKey=%v)\n",
				globalFlags.APIKey != "", os.Getenv("IDAPT_API_KEY") != "", credPath, creds.APIKey != "")
		}
		apiURL := globalFlags.APIURL
		if apiURL == "" {
			apiURL = cfg.APIURL
		}
		if globalFlags.Verbose {
			fmt.Fprintf(cmd.ErrOrStderr(), "API URL: %s\n", apiURL)
		}

		format := output.Format(globalFlags.Output)
		if format == "" {
			format = output.Format(cfg.OutputFormat)
		}
		if format == "" {
			format = output.Detect()
		}

		noColor := globalFlags.NoColor || cfg.NoColor

		f := &cmdutil.Factory{
			Config:      cfg,
			Credentials: creds,
			Format:      format,
			NoColor:     noColor,
			Out:         cmd.OutOrStdout(),
			ErrOut:      cmd.ErrOrStderr(),
			In:          cmd.InOrStdin(),
		}
		f.SetClientFn(func() (*api.Client, error) {
			c, err := api.NewClient(api.ClientConfig{
				BaseURL:    apiURL,
				APIKey:     apiKey,
				Verbose:    globalFlags.Verbose,
				CLIVersion: Version,
			})
			if err != nil {
				return nil, err
			}
			if globalFlags.Verbose {
				c.SetErrOut(cmd.ErrOrStderr())
			}
			return c, nil
		})

		f.SetFlagsFn(func() (*features.Flags, error) {
			client, err := f.APIClient()
			if err != nil {
				return &features.Flags{}, nil
			}
			loader := features.NewLoader(client)
			flags, err := loader.Load(cmd.Context())
			if err != nil {
				return &features.Flags{}, nil
			}
			if globalFlags.Verbose {
				fmt.Fprintf(cmd.ErrOrStderr(), "Feature flags: source=%s\n", flags.Source())
			}
			return flags, nil
		})

		cmdutil.SetFactory(cmd, f)

		if isAuthFreeCommand(cmd) {
			return nil
		}
		return cmdutil.RequireAuth(f)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if p, _ := cmd.Flags().GetString("prompt"); p != "" {
			if err := requireTUIFlag(cmd, nil); err != nil {
				return err
			}
			return runChatAskFromRootFlag(cmd, p)
		}
		if !shouldBootTUI(cmd, args) {
			return cmd.Help()
		}
		if err := requireTUIFlag(cmd, nil); err != nil {
			return cmd.Help()
		}
		f := cmdutil.FactoryFromCmd(cmd)
		return tui.Run(cmd.Context(), f)
	},
	SilenceErrors: true,
	SilenceUsage:  true,
}

func shouldBootTUI(cmd *cobra.Command, args []string) bool {
	if os.Getenv("IDAPT_NO_TUI") == "1" {
		return false
	}
	if len(args) > 0 {
		return false
	}
	for _, flag := range []string{"output", "json", "instructions"} {
		if cmd.Flags().Changed(flag) {
			return false
		}
	}
	if !isatty.IsTerminal(os.Stdin.Fd()) {
		return false
	}
	if !isatty.IsTerminal(os.Stdout.Fd()) {
		return false
	}
	return true
}
func NewRootCmd() *cobra.Command {
	root := rootCmd
	root.SetContext(context.Background())
	return root
}

func Execute() error {
	rootCmd.SetContext(context.Background())
	if err := rootCmd.Execute(); err != nil {
		if errors.Is(err, ErrInstructionsShortCircuit) {
			return nil
		}
		return err
	}
	return nil
}

func init() {
	globalFlags = cmdutil.RegisterGlobalFlags(rootCmd)

	rootCmd.PersistentFlags().StringP("prompt", "p", "", "one-shot prompt; bypasses the TUI and streams the response to stdout")

	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(tuiCmd)

	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(configCliCmd)
	rootCmd.AddCommand(workspaceCmd)
	rootCmd.AddCommand(agentCmd)
	rootCmd.AddCommand(chatCmd)
	rootCmd.AddCommand(fileCmd)
	rootCmd.AddCommand(computerRemoteCmd)
	rootCmd.AddCommand(scriptCmd)
	rootCmd.AddCommand(secretCmd)
	rootCmd.AddCommand(hubCmd)
	rootCmd.AddCommand(modelCmd)
	rootCmd.AddCommand(execCmd)
	rootCmd.AddCommand(webCmd)
	rootCmd.AddCommand(mediaCmd)
	rootCmd.AddCommand(settingsCmd)
	rootCmd.AddCommand(profileCmd)
	rootCmd.AddCommand(subscriptionCmd)
	rootCmd.AddCommand(apikeyCmd)
	rootCmd.AddCommand(shareCmd)
	rootCmd.AddCommand(notificationCmd)
	rootCmd.AddCommand(subagentCmd)
	rootCmd.AddCommand(triggerCmd)
	rootCmd.AddCommand(instructionsCmd)
}

func isDaemonCommand(cmd *cobra.Command) bool {
	name := cmd.Name()
	if name == "version" || name == "serve" || name == "help" {
		return true
	}
	if name == "expose" || name == "unexpose" {
		return true
	}
	for p := cmd.Parent(); p != nil; p = p.Parent() {
		pn := p.Name()
		if pn == "firewall" || pn == "tunnel" {
			return true
		}
	}
	if name == "firewall" || name == "tunnel" {
		return true
	}
	return false
}

func isAuthFreeCommand(cmd *cobra.Command) bool {
	if isDaemonCommand(cmd) {
		return true
	}
	name := cmd.Name()
	switch name {
	case "tui",
		"pair",
		"selftest",
		"update",
		"uninstall",
		"instructions",
		"completion", // cobra-generated shell completion command
		"logs",       // top-level alias for `service logs`
		"status":     // top-level alias for `service status`
		return true
	}
	for c := cmd; c != nil; c = c.Parent() {
		switch c.Name() {
		case "auth",
			"config",
			"service":
			return true
		}
	}
	return false
}
