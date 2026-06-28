package cmd

import (
	"fmt"
	"strings"

	"github.com/idapt/idapt-computer/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage the local idapt daemon (start, stop, status, logs)",
	Long: `Manage the local idapt daemon — the long-running process behind chat,
file sync, tunnels, and local inference.

Verbs:
  idapt-computer service up         Start the daemon (installs autostart unit on first run)
  idapt-computer service down       Stop the daemon (autostart unit stays in place)
  idapt-computer service restart    Restart the daemon
  idapt-computer service status     Show running state, PID, version, recent errors
  idapt-computer service logs       Tail recent daemon logs (-f to follow)
  idapt-computer service policy     Enable or disable local daemon capabilities
  idapt-computer service uninstall  Remove the autostart unit entirely

For foreground debugging, use ` + "`idapt-computer serve`" + ` directly.`,
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
installed so the next ` + "`idapt-computer service up`" + ` is instant. To remove the
unit entirely, use ` + "`idapt-computer service uninstall`" + `.`,
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
this, the daemon will not start on login until you run ` + "`idapt-computer service up`" + `
again. Most users want ` + "`idapt-computer service down`" + ` instead, which stops the
daemon but keeps the unit so the next ` + "`up`" + ` is instant.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return serviceUninstall(cmd)
	},
}

var servicePolicyCmd = &cobra.Command{
	Use:   "policy",
	Short: "Enable or disable local daemon capabilities",
	Long: `Enable or disable local daemon capability families in the paired
daemon config. Changes take effect after ` + "`idapt-computer service restart`" + `.

Capabilities: remote-shell, remote-files, admin-ops, local-inference,
computer-apps, computer-use, tunnels.`,
}

var servicePolicyEnableCmd = &cobra.Command{
	Use:   "enable <capability>",
	Short: "Enable a local daemon capability",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return setServicePolicy(cmd, args[0], true)
	},
}

var servicePolicyDisableCmd = &cobra.Command{
	Use:   "disable <capability>",
	Short: "Disable a local daemon capability",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return setServicePolicy(cmd, args[0], false)
	},
}

func setServicePolicy(cmd *cobra.Command, capability string, enabled bool) error {
	path, err := config.ResolveConfigPath(configPath)
	if err != nil {
		return fmt.Errorf("resolve daemon config path: %w", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	if cfg.IsLocalMode() {
		return fmt.Errorf("no paired daemon config at %s; run `idapt-computer up` first", path)
	}
	canonical, ok := setCommandPolicyCapability(&cfg.CommandPolicy, capability, enabled)
	if !ok {
		return fmt.Errorf("unknown capability %q; valid capabilities: remote-shell, remote-files, admin-ops, local-inference, computer-apps, computer-use, tunnels", capability)
	}
	if err := writeStrictJSONFile(path, cfg); err != nil {
		return err
	}
	state := "disabled"
	if enabled {
		state = "enabled"
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Local daemon policy %s: %s\n", state, canonical)
	fmt.Fprintln(cmd.OutOrStdout(), "Restart the daemon with `idapt-computer service restart` for the change to take effect.")
	return nil
}

func setCommandPolicyCapability(policy *config.CommandPolicy, capability string, enabled bool) (string, bool) {
	canonical := strings.ToLower(strings.TrimSpace(capability))
	canonical = strings.ReplaceAll(canonical, "_", "-")
	switch canonical {
	case "remote-shell", "shell":
		policy.RemoteShell = enabled
		return "remote-shell", true
	case "remote-files", "files":
		policy.RemoteFiles = enabled
		return "remote-files", true
	case "admin-ops", "admin", "users", "user-management":
		policy.AdminOps = enabled
		return "admin-ops", true
	case "local-inference", "inference":
		policy.LocalInference = enabled
		return "local-inference", true
	case "computer-apps", "apps":
		policy.ComputerApps = enabled
		return "computer-apps", true
	case "computer-use", "desktop":
		policy.ComputerUse = enabled
		return "computer-use", true
	case "tunnels", "tunnel":
		policy.Tunnels = enabled
		return "tunnels", true
	default:
		return "", false
	}
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
	servicePolicyCmd.AddCommand(servicePolicyEnableCmd)
	servicePolicyCmd.AddCommand(servicePolicyDisableCmd)
	serviceCmd.AddCommand(servicePolicyCmd)
	serviceCmd.AddCommand(serviceUninstallCmd)
	rootCmd.AddCommand(serviceCmd)

	registerServiceTopLevelAlias("logs", serviceLogsCmd)
	registerServiceTopLevelAlias("status", serviceStatusCmd)
}

func registerServiceTopLevelAlias(name string, target *cobra.Command) {
	proxy := &cobra.Command{
		Use:    name,
		Short:  fmt.Sprintf("Shortcut for `idapt-computer service %s`", name),
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
