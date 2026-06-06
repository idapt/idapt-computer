package cmd

import (
	"fmt"
	"os"

	"github.com/idapt/idapt-cli/internal/config"
	"github.com/spf13/cobra"
)

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop the local idapt daemon (autostart unit stays installed)",
	Long: `Shortcut for ` + "`idapt service down`" + `. Stops the running daemon
without removing the autostart unit, so the next ` + "`idapt up`" + ` is
instant.

To clear the pairing entirely use ` + "`idapt logout`" + `;
to remove the autostart unit too use ` + "`idapt service uninstall`" + `.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return serviceDown(cmd)
	},
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Unpair this computer (clear the daemon config + stop the daemon)",
	Long: `Removes the daemon config that links this computer to your idapt
account. The autostart unit stays installed so the next ` + "`idapt up`" + ` is
fast. The daemon (if running) is stopped — it would otherwise keep
heartbeating with stale credentials until the next restart.

This does NOT remove the computer row from the server side. The
linked computer will appear under "computers" in the web UI with
"unreachable" health until you delete it there or re-pair it.

Use ` + "`idapt auth logout`" + ` to clear the CLI's own API key
(separate from the device pairing — they are unrelated credentials).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		path, err := config.UserConfigPath()
		if err != nil {
			return fmt.Errorf("locate config path: %w", err)
		}
		removed := false
		if _, statErr := os.Stat(path); statErr == nil {
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("remove %s: %w", path, err)
			}
			removed = true
			fmt.Fprintf(out, "Removed daemon config: %s\n", path)
		}
		if _, statErr := os.Stat(config.LegacySystemConfigPath); statErr == nil {
			if err := os.Remove(config.LegacySystemConfigPath); err == nil {
				fmt.Fprintf(out, "Removed legacy daemon config: %s\n", config.LegacySystemConfigPath)
				removed = true
			} else {
				fmt.Fprintf(out, "Legacy daemon config remains at %s (run with sudo to remove)\n", config.LegacySystemConfigPath)
			}
		}
		if !removed {
			fmt.Fprintln(out, "No daemon config was found — nothing to unpair.")
		}
		if err := serviceDown(cmd); err != nil {
			fmt.Fprintf(out, "WARN: could not stop daemon: %v\n", err)
		}
		fmt.Fprintln(out, "Unpaired.")
		return nil
	},
}
func init() {
	rootCmd.AddCommand(downCmd)
	rootCmd.AddCommand(logoutCmd)
}
