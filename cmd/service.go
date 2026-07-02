package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/idapt/idapt-computer/internal/config"
	"github.com/idapt/idapt-computer/internal/update"
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
  idapt-computer service doctor     Diagnose + --fix autostart problems (duplicate launchers, ...)
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
		requested, _ := cmd.Flags().GetString("autostart")
		serviceAutostart = resolveServiceAutostart(requested)
		if err := serviceUp(cmd, reinstall); err != nil {
			return err
		}
		persistAutostartPolicy(serviceAutostart)
		return nil
	},
}

var serviceAutostart = config.AutostartAlways

func resolveServiceAutostart(requested string) string {
	policy := config.NormalizeAutostart(requested)
	if requested == "" {
		policy = "" // no explicit request — fall back to recorded/default below
	}
	recorded := ""
	if path, err := config.ResolveConfigPath(configPath); err == nil {
		if cfg, err := config.Load(path); err == nil {
			recorded = cfg.Autostart
		}
	}
	if policy == "" {
		if recorded != "" {
			return config.NormalizeAutostart(recorded)
		}
		return config.AutostartAlways
	}
	if recorded != "" && config.AutostartRank(recorded) > config.AutostartRank(policy) {
		return config.NormalizeAutostart(recorded)
	}
	return policy
}

func persistAutostartPolicy(policy string) {
	path, err := config.ResolveConfigPath(configPath)
	if err != nil {
		return
	}
	cfg, err := config.Load(path)
	if err != nil || cfg.IsLocalMode() {
		return
	}
	normalized := config.NormalizeAutostart(policy)
	if config.NormalizeAutostart(cfg.Autostart) == normalized && cfg.Autostart != "" {
		return
	}
	cfg.Autostart = normalized
	_ = writeStrictJSONFile(path, cfg)
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
		if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
			return printServiceStatusJSON(cmd)
		}
		return serviceStatus(cmd)
	},
}

type serviceStatusView struct {
	Running   bool   `json:"running"`
	Healthy   bool   `json:"healthy"`
	Version   string `json:"version,omitempty"`
	Autostart string `json:"autostart"`
	AutostartMechanism string `json:"autostartMechanism,omitempty"`
	Supervised         *bool  `json:"supervised,omitempty"`
	Paired             bool   `json:"paired"`
	ComputerResourceID string `json:"computerResourceId,omitempty"`
	Domain             string `json:"domain,omitempty"`
	ActiveResourceID   string `json:"activeResourceId,omitempty"`
	ActiveDomain       string `json:"activeDomain,omitempty"`
	Cloud              bool   `json:"cloud"`
	HealthURL          string `json:"healthUrl"`
	HealthStatus       string `json:"healthStatus,omitempty"`
	CommandsEnabled    *bool  `json:"commandsEnabled,omitempty"`
	CommandsConnected  *bool  `json:"commandsConnected,omitempty"`
	CommandsLastError  string `json:"commandsLastError,omitempty"`
	TunnelConnected    *bool  `json:"tunnelConnected,omitempty"`
	TunnelLastError    string `json:"tunnelLastError,omitempty"`
}

func printServiceStatusJSON(cmd *cobra.Command) error {
	view := serviceStatusView{Autostart: config.AutostartAlways}
	mechanism, supervised := daemonSupervision()
	view.AutostartMechanism = mechanism
	view.Supervised = boolPtr(supervised)
	if path, err := config.ResolveConfigPath(configPath); err == nil {
		if cfg, err := config.Load(path); err == nil {
			view.Autostart = config.NormalizeAutostart(cfg.Autostart)
			view.Cloud = cfg.Cloud
			if !cfg.IsLocalMode() {
				view.Paired = true
				view.ComputerResourceID = cfg.ComputerResourceID
				if view.ComputerResourceID == "" {
					view.ComputerResourceID = cfg.ComputerID
				}
				view.Domain = cfg.Domain
			}
		}
	}
	view.HealthURL = update.LocalHealthURL(view.Cloud)
	snapshot, reachable := update.ProbeHealthSnapshot(view.HealthURL, 2*time.Second)
	view.Running = reachable
	view.Healthy = reachable && snapshot.Status == "ok"
	view.Version = snapshot.Version
	view.HealthStatus = snapshot.Status
	if reachable {
		view.ActiveResourceID = snapshot.ComputerResourceID
		if view.ActiveResourceID == "" {
			view.ActiveResourceID = snapshot.ComputerID
		}
		view.ActiveDomain = snapshot.Domain
		view.CommandsEnabled = boolPtr(snapshot.CommandsEnabled)
		view.CommandsConnected = boolPtr(snapshot.CommandsConnected)
		view.CommandsLastError = snapshot.CommandsLastError
		view.TunnelConnected = boolPtr(snapshot.TunnelConnected)
		view.TunnelLastError = snapshot.TunnelLastError
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(view)
}

func boolPtr(v bool) *bool {
	return &v
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

These families are the local security boundary — the cloud can request a
command but this daemon only runs it if the family is enabled HERE. They group
by real blast radius:

  Band A — full control (arbitrary code execution; mutually escalating):
    remote-shell, remote-files, admin-ops, computer-use, remote-terminal
    Use ` + "`full-control`" + ` to toggle all five at once.
  Band B — privileged / exposure: computer-apps, tunnels (HTTP port exposure)
  Band C — bounded: local-inference

Capabilities: full-control, remote-shell, remote-files, admin-ops, computer-use,
remote-terminal, computer-apps, tunnels, local-inference.`,
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

var servicePolicyGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Show the local daemon capability policy (add --json for the desktop app)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return getServicePolicy(cmd)
	},
}

func setServicePolicy(cmd *cobra.Command, capability string, enabled bool) error {
	path, err := config.ResolveConfigPath(configPath)
	if err != nil {
		return fmt.Errorf("resolve daemon config path: %w", err)
	}
	cfg, err := config.LoadRaw(path)
	if err != nil {
		return err
	}
	if cfg.IsLocalMode() {
		return fmt.Errorf("no paired daemon config at %s; run `idapt-computer up` first", path)
	}
	config.Migrate(cfg)
	canonical, ok := setCommandPolicyCapability(&cfg.CommandPolicy, capability, enabled)
	if !ok {
		return fmt.Errorf("unknown capability %q; valid capabilities: full-control, remote-shell, remote-files, admin-ops, computer-use, remote-terminal, computer-apps, tunnels, local-inference", capability)
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
	case "full-control", "full-access":
		policy.RemoteShell = enabled
		policy.RemoteFiles = enabled
		policy.AdminOps = enabled
		policy.ComputerUse = enabled
		policy.RemoteTerminal = enabled
		return "full-control", true
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
	case "remote-terminal", "terminal":
		policy.RemoteTerminal = enabled
		return "remote-terminal", true
	case "tunnels", "tunnel":
		policy.Tunnels = enabled
		return "tunnels", true
	default:
		return "", false
	}
}

type commandPolicyView struct {
	FullControl    bool `json:"fullControl"`
	RemoteShell    bool `json:"remoteShell"`
	RemoteFiles    bool `json:"remoteFiles"`
	AdminOps       bool `json:"adminOps"`
	ComputerUse    bool `json:"computerUse"`
	RemoteTerminal bool `json:"remoteTerminal"`
	ComputerApps   bool `json:"computerApps"`
	Tunnels        bool `json:"tunnels"`
	LocalInference bool `json:"localInference"`
}

func policyView(p config.CommandPolicy) commandPolicyView {
	return commandPolicyView{
		FullControl:    p.RemoteShell && p.RemoteFiles && p.AdminOps && p.ComputerUse && p.RemoteTerminal,
		RemoteShell:    p.RemoteShell,
		RemoteFiles:    p.RemoteFiles,
		AdminOps:       p.AdminOps,
		ComputerUse:    p.ComputerUse,
		RemoteTerminal: p.RemoteTerminal,
		ComputerApps:   p.ComputerApps,
		Tunnels:        p.Tunnels,
		LocalInference: p.LocalInference,
	}
}

func getServicePolicy(cmd *cobra.Command) error {
	path, err := config.ResolveConfigPath(configPath)
	if err != nil {
		return fmt.Errorf("resolve daemon config path: %w", err)
	}
	cfg, err := config.LoadRaw(path)
	if err != nil {
		return err
	}
	config.Migrate(cfg)
	view := policyView(cfg.CommandPolicy)
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(view)
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Local daemon capability policy (%s):\n", path)
	fmt.Fprintf(out, "  full-control:    %s  (band A group)\n", enabledLabel(view.FullControl))
	fmt.Fprintf(out, "    remote-shell:    %s\n", enabledLabel(view.RemoteShell))
	fmt.Fprintf(out, "    remote-files:    %s\n", enabledLabel(view.RemoteFiles))
	fmt.Fprintf(out, "    admin-ops:       %s\n", enabledLabel(view.AdminOps))
	fmt.Fprintf(out, "    computer-use:    %s\n", enabledLabel(view.ComputerUse))
	fmt.Fprintf(out, "    remote-terminal: %s\n", enabledLabel(view.RemoteTerminal))
	fmt.Fprintf(out, "  computer-apps:   %s  (band B)\n", enabledLabel(view.ComputerApps))
	fmt.Fprintf(out, "  tunnels:         %s  (band B, HTTP exposure)\n", enabledLabel(view.Tunnels))
	fmt.Fprintf(out, "  local-inference: %s  (band C)\n", enabledLabel(view.LocalInference))
	return nil
}

func enabledLabel(v bool) string {
	if v {
		return "enabled"
	}
	return "disabled"
}

func init() {
	serviceUpCmd.Flags().Bool("reinstall", false, "Force-rewrite the autostart unit before starting (after binary move/upgrade)")
	serviceUpCmd.Flags().String("autostart", "", "Autostart policy: always (boot/login, default), app (desktop-controlled), or manual (installed, not auto-started)")
	serviceStatusCmd.Flags().Bool("json", false, "Emit machine-readable JSON status (running, version, autostart, identity) for the desktop controller")
	serviceLogsCmd.Flags().BoolP("follow", "f", false, "Stream new log lines as they arrive")
	serviceLogsCmd.Flags().Int("lines", 100, "Number of recent lines to show before following")
	serviceLogsCmd.Flags().String("since", "", "Only show entries newer than the given duration (e.g. 10m, 2h)")
	serviceDoctorCmd.Flags().Bool("fix", false, "Repair fixable problems: collapse the autostart back to a single supervised launcher")

	serviceCmd.AddCommand(serviceUpCmd)
	serviceCmd.AddCommand(serviceDownCmd)
	serviceCmd.AddCommand(serviceRestartCmd)
	serviceCmd.AddCommand(serviceStatusCmd)
	serviceCmd.AddCommand(serviceLogsCmd)
	serviceCmd.AddCommand(serviceDoctorCmd)
	servicePolicyGetCmd.Flags().Bool("json", false, "Emit machine-readable JSON (per-capability + derived fullControl) for the desktop controller")
	servicePolicyCmd.AddCommand(servicePolicyEnableCmd)
	servicePolicyCmd.AddCommand(servicePolicyDisableCmd)
	servicePolicyCmd.AddCommand(servicePolicyGetCmd)
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
