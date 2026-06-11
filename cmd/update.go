package cmd

import (
	"fmt"
	"os"

	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/config"
	"github.com/idapt/idapt-cli/internal/progress"
	"github.com/idapt/idapt-cli/internal/update"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check for and apply updates",
	Long:  "Checks for a new version of the idapt daemon, downloads it, and signals the running daemon to seamlessly restart.",
	RunE: func(cmd *cobra.Command, args []string) error {
		appURL := os.Getenv("IDAPT_APP_URL")
		if appURL == "" {
			if cfg, err := config.Load(configPath); err == nil {
				appURL = cfg.AppURL
			}
		}
		if appURL == "" {
			appURL = "https://idapt.ai"
		}

		binaryPath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("get executable path: %w", err)
		}

		updater := update.New(appURL, Version, binaryPath)
		if f := cmdutil.FactoryFromCmd(cmd); f != nil {
			updater.ProgressBar = func(total int64) *progress.Bar {
				return f.NewProgressBar("Downloading update", total)
			}
		}

		info, err := updater.Check()
		if err != nil {
			return fmt.Errorf("check for updates: %w", err)
		}

		if info == nil {
			_ = update.SaveCheckState(Version)
			fmt.Println("Already up to date.")
			return nil
		}

		_ = update.SaveCheckState(info.Version)

		fmt.Printf("Update available: %s → %s\n", Version, info.Version)

		if check, _ := cmd.Flags().GetBool("check"); check {
			fmt.Println("Run `idapt update` to install it.")
			return nil
		}

		if err := updater.Apply(info); err != nil {
			return fmt.Errorf("apply update: %w", err)
		}

		fmt.Println(signalDaemonRestart(info.Version))
		return nil
	},
}

func init() {
	updateCmd.Flags().Bool("check", false, "Check for an update and report it without downloading/applying")
	rootCmd.AddCommand(updateCmd)
}
