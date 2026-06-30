package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/idapt/idapt-computer/internal/config"
	"github.com/spf13/cobra"
)
var serviceElevateCmd = &cobra.Command{
	Use:   "elevate",
	Short: "Upgrade the daemon from a user service to a root system service (Linux, sudo)",
	Long: `Migrates this computer's pairing to a root SYSTEM service so the
daemon starts at boot and can run commands as any user (including root) when a
command explicitly asks for it. Day-to-day commands still run as your user.

The pairing identity (computerId + token) is preserved — the server keeps the
same computer and simply sees it report runsAsRoot=true on the next heartbeat.

Requires sudo (writes /etc/idapt and installs a system unit). Linux only.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runServiceElevate(cmd)
	},
}

var serviceDropCmd = &cobra.Command{
	Use:   "drop",
	Short: "Downgrade the daemon from a root system service back to a user service (sudo)",
	Long: `Reverses ` + "`idapt-computer service elevate`" + `: removes the root system
service and reinstalls the rootless per-user service. The pairing identity is
preserved; the daemon reports runsAsRoot=false on the next heartbeat.

Requires sudo to remove the root system unit. Linux only.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runServiceDrop(cmd)
	},
}

func runServiceElevate(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	if !systemModeSupported() {
		return fmt.Errorf("system mode is Linux-only — `service elevate` is unavailable on this OS")
	}
	if os.Geteuid() != 0 {
		return fmt.Errorf("`service elevate` needs root to install the system service — re-run with sudo")
	}

	userPath, err := config.UserConfigPath()
	if err != nil {
		return fmt.Errorf("locate user config path: %w", err)
	}
	if _, statErr := os.Stat(userPath); statErr != nil {
		return fmt.Errorf("no user-mode config found at %s — pair first with `idapt-computer up`", userPath)
	}

	if err := serviceUninstall(cmd); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "WARN: could not fully remove the user unit: %v\n", err)
	}

	if err := migrateConfig(userPath, config.SystemConfigPath); err != nil {
		return fmt.Errorf("migrate config to %s: %w", config.SystemConfigPath, err)
	}
	fmt.Fprintf(out, "Migrated pairing config to %s.\n", config.SystemConfigPath)

	if err := installSystemService(cmd, true); err != nil {
		return fmt.Errorf("install system service: %w", err)
	}
	fmt.Fprintln(out, "Elevated: the daemon now runs as a root system service.")
	return nil
}

func runServiceDrop(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	if !systemModeSupported() {
		return fmt.Errorf("system mode is Linux-only — `service drop` is unavailable on this OS")
	}
	if os.Geteuid() != 0 {
		return fmt.Errorf("`service drop` needs root to remove the system service — re-run with sudo")
	}

	userPath, err := config.UserConfigPath()
	if err != nil {
		return fmt.Errorf("locate user config path: %w", err)
	}

	if err := uninstallSystemService(cmd); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "WARN: could not fully remove the system unit: %v\n", err)
	}

	if _, statErr := os.Stat(config.SystemConfigPath); statErr == nil {
		if err := migrateConfig(config.SystemConfigPath, userPath); err != nil {
			return fmt.Errorf("migrate config to %s: %w", userPath, err)
		}
		fmt.Fprintf(out, "Migrated pairing config to %s.\n", userPath)
	} else {
		fmt.Fprintf(cmd.ErrOrStderr(), "NOTE: no system config at %s to migrate back; using whatever is already at %s.\n", config.SystemConfigPath, userPath)
	}

	if err := installServiceForMode(cmd, InstallModeUser, true); err != nil {
		return fmt.Errorf("install user service: %w", err)
	}
	fmt.Fprintln(out, "Dropped: the daemon now runs as a rootless per-user service.")
	if os.Getenv("SUDO_USER") != "" {
		fmt.Fprintf(out, "Tip: to bind the user service to %s's login instead of root's, run `idapt-computer service up` as that user.\n", os.Getenv("SUDO_USER"))
	}
	return nil
}

func migrateConfig(src, dst string) error {
	if filepath.Clean(src) == filepath.Clean(dst) {
		return nil
	}
	cfg, err := config.Load(src)
	if err != nil {
		return fmt.Errorf("read source config: %w", err)
	}
	if cfg.IsLocalMode() {
		return fmt.Errorf("source config %s has no pairing (local mode) — nothing to migrate", src)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(dst), err)
	}
	if err := writeMigratedConfig(dst, cfg); err != nil {
		return err
	}
	return nil
}

func writeMigratedConfig(dst string, cfg *config.Config) error {
	out := map[string]any{
		"computerId":         cfg.ComputerID,
		"computerResourceId": cfg.ComputerResourceID,
		"appUrl":             cfg.AppURL,
		"domain":             cfg.Domain,
		"jwksUrl":            cfg.AppURL + "/api/cloud-computers/jwks",
		"computerToken":      cfg.ComputerToken,
		"defaultBackendPort": cfg.DefaultBackendPort,
	}
	if cfg.ComputerResourceID == "" {
		out["computerResourceId"] = cfg.ComputerID
	}
	if cfg.TunnelProxyURL != "" {
		out["tunnelProxyUrl"] = cfg.TunnelProxyURL
	}
	return writeStrictJSONFile(dst, out)
}

func init() {
	serviceCmd.AddCommand(serviceElevateCmd)
	serviceCmd.AddCommand(serviceDropCmd)
}
