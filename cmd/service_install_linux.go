//go:build linux

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

const linuxUserUnit = `[Unit]
Description=Idapt daemon (user)
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=%s serve
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
`

func installPlatformService(cmd *cobra.Command) error {
	binPath, err := os.Executable()
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	unitDir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		return err
	}
	unitPath := filepath.Join(unitDir, "idapt.service")
	if err := os.WriteFile(unitPath, []byte(fmt.Sprintf(linuxUserUnit, binPath)), 0o644); err != nil {
		return err
	}
	if err := exec.Command("systemctl", "--user", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w", err)
	}
	if err := exec.Command("systemctl", "--user", "enable", "--now", "idapt.service").Run(); err != nil {
		return fmt.Errorf("systemctl enable: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Linux user service installed at %s\n", unitPath)
	return nil
}

func uninstallPlatformService(cmd *cobra.Command) error {
	home, _ := os.UserHomeDir()
	unitPath := filepath.Join(home, ".config", "systemd", "user", "idapt.service")
	_ = exec.Command("systemctl", "--user", "disable", "--now", "idapt.service").Run()
	_ = os.Remove(unitPath)
	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	fmt.Fprintln(cmd.OutOrStdout(), "Linux user service uninstalled")
	return nil
}
