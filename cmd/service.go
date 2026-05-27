package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage the local idapt daemon (start, stop, status, logs)",
	Long: `Manage the local idapt daemon — the long-running process behind chat,
file sync, tunnels, and local inference.

Verbs:
  idapt service up         Start the daemon (installs autostart unit on first run)
  idapt service down       Stop the daemon (autostart unit stays in place)
  idapt service restart    Restart the daemon
  idapt service status     Show running state, PID, version, recent errors
  idapt service logs       Tail recent daemon logs (-f to follow)
  idapt service uninstall  Remove the autostart unit entirely

For foreground debugging, use ` + "`idapt serve`" + ` directly.`,
}

var serviceUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Start the daemon (installs autostart unit on first run)",
	Long: `Idempotent. Writes the per-OS autostart unit if missing
(systemd user service on Linux, LaunchAgent on macOS) and starts the
daemon. Safe to re-run — already-installed units are reused.

Pass --reinstall to force-rewrite the unit (useful after the idapt
binary has moved or been upgraded to a different path).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		reinstall, _ := cmd.Flags().GetBool("reinstall")
		return serviceUp(cmd, reinstall)
	},
}

var serviceDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop the daemon (autostart unit stays in place)",
	Long: `Stops the running daemon. The autostart unit is left
installed so the next ` + "`idapt service up`" + ` is instant. To remove the
unit entirely, use ` + "`idapt service uninstall`" + `.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return serviceDown(cmd)
	},
}

var serviceRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		return serviceRestart(cmd)
	},
}

var serviceStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon running state, PID, recent errors",
	RunE: func(cmd *cobra.Command, args []string) error {
		return serviceStatus(cmd)
	},
}

var serviceLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Tail recent daemon logs",
	Long: `Tails the daemon logs from the platform's native journal
(journalctl on Linux, Unified Logging files on macOS, Event Log on
Windows). Use -f to follow new entries.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		follow, _ := cmd.Flags().GetBool("follow")
		lines, _ := cmd.Flags().GetInt("lines")
		since, _ := cmd.Flags().GetString("since")
		return serviceLogs(cmd, follow, lines, since)
	},
}

var serviceUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the autostart unit entirely (rare — usually you want `down`)",
	Long: `Stops the daemon and removes the autostart unit. After running
this, the daemon will not start on login until you run ` + "`idapt service up`" + `
again. Most users want ` + "`idapt service down`" + ` instead, which stops the
daemon but keeps the unit so the next ` + "`up`" + ` is instant.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return serviceUninstall(cmd)
	},
}

func init() {
	serviceUpCmd.Flags().Bool("reinstall", false, "Force-rewrite the autostart unit before starting (after binary move/upgrade)")
	serviceLogsCmd.Flags().BoolP("follow", "f", false, "Stream new log lines as they arrive")
	serviceLogsCmd.Flags().Int("lines", 100, "Number of recent lines to show before following")
	serviceLogsCmd.Flags().String("since", "", "Only show entries newer than the given duration (e.g. 10m, 2h)")

	serviceCmd.AddCommand(serviceUpCmd)
	serviceCmd.AddCommand(serviceDownCmd)
	serviceCmd.AddCommand(serviceRestartCmd)
	serviceCmd.AddCommand(serviceStatusCmd)
	serviceCmd.AddCommand(serviceLogsCmd)
	serviceCmd.AddCommand(serviceUninstallCmd)
	rootCmd.AddCommand(serviceCmd)

	registerServiceTopLevelAlias("logs", serviceLogsCmd)
	registerServiceTopLevelAlias("status", serviceStatusCmd)
}

func registerServiceTopLevelAlias(name string, target *cobra.Command) {
	proxy := &cobra.Command{
		Use:    name,
		Short:  fmt.Sprintf("Shortcut for `idapt service %s`", name),
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return target.RunE(cmd, args)
		},
	}
	target.Flags().VisitAll(func(f *pflag.Flag) {
		proxy.Flags().AddFlag(f)
	})
	rootCmd.AddCommand(proxy)
}

func notImplementedForOS(verb string) error {
	return fmt.Errorf("service %s is not yet implemented for this OS", verb)
}
