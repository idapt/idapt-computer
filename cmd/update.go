package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/idapt/idapt-computer/internal/cmdutil"
	"github.com/idapt/idapt-computer/internal/config"
	"github.com/idapt/idapt-computer/internal/progress"
	"github.com/idapt/idapt-computer/internal/update"
	"github.com/spf13/cobra"
)

const healthGateTimeout = 60 * time.Second

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check for and apply updates",
	Long: "Checks for a new version of the idapt daemon, verifies the signed manifest, " +
		"downloads + verifies the binary, swaps it in, restarts the daemon, and rolls back " +
		"to the last-known-good binary if the new version fails its health check.",
	RunE: func(cmd *cobra.Command, args []string) error {
		quiet, _ := cmd.Flags().GetBool("quiet")
		say := func(format string, a ...any) {
			if !quiet {
				fmt.Printf(format+"\n", a...)
			}
		}

		cfg, _ := config.Load(configPath)
		appURL := os.Getenv("IDAPT_APP_URL")
		if appURL == "" && cfg != nil {
			appURL = cfg.AppURL
		}
		if appURL == "" {
			appURL = "https://idapt.app"
		}
		cloud := cfg != nil && cfg.Cloud

		binaryPath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("get executable path: %w", err)
		}

		updater := update.New(appURL, Version, binaryPath)
		if cfg != nil {
			updater.SetComputerID(cfg.ComputerID)
		}
		if f := cmdutil.FactoryFromCmd(cmd); f != nil && !quiet {
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
			say("Already up to date.")
			return nil
		}

		_ = update.SaveCheckState(info.Version)
		say("Update available: %s → %s", Version, info.Version)

		if check, _ := cmd.Flags().GetBool("check"); check {
			say("Run `idapt-computer update` to install it.")
			return nil
		}

		release, err := update.AcquireUpdateLock()
		if err != nil {
			return err
		}
		defer release()

		healthURL := update.LocalHealthURL(cloud)
		_, _, daemonRunning := update.ProbeHealth(healthURL, 3*time.Second)

		if err := update.SaveLastKnownGood(binaryPath); err != nil {
			say("warning: could not save last-known-good (continuing): %v", err)
		}

		if err := updater.Apply(info); err != nil {
			return fmt.Errorf("apply update: %w", err)
		}

		say("%s", signalDaemonRestart(info.Version))

		if daemonRunning {
			if herr := update.WaitHealthy(healthURL, info.Version, healthGateTimeout); herr != nil {
				say("update: %s — rolling back to last-known-good", herr)
				if !update.HasLastKnownGood(binaryPath) {
					return fmt.Errorf("update unhealthy and no last-known-good to roll back to: %w", herr)
				}
				if rbErr := update.RestoreLastKnownGood(binaryPath); rbErr != nil {
					return fmt.Errorf("update unhealthy AND rollback failed: %v (original: %w)", rbErr, herr)
				}
				_ = signalDaemonRestart(Version) // bring the daemon back on the restored binary
				_ = update.WaitHealthy(healthURL, Version, 30*time.Second)
				return fmt.Errorf("update rolled back to last-known-good after health check failed: %w", herr)
			}
			say("update: new version %s is healthy.", info.Version)
		}
		return nil
	},
}

func init() {
	updateCmd.Flags().Bool("check", false, "Check for an update and report it without downloading/applying")
	updateCmd.Flags().Bool("quiet", false, "Suppress informational output (used by the auto-update timer)")
	rootCmd.AddCommand(updateCmd)
}
