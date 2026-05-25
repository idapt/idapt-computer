//go:build linux

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

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

const linuxUnitName = "idapt.service"

func linuxUnitPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "systemd", "user", linuxUnitName), nil
}

func writeUnit() error {
	binPath, err := os.Executable()
	if err != nil {
		return err
	}
	unitPath, err := linuxUnitPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(unitPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(unitPath, []byte(fmt.Sprintf(linuxUserUnit, binPath)), 0o644)
}

func serviceUp(cmd *cobra.Command, reinstall bool) error {
	unitPath, err := linuxUnitPath()
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(unitPath); reinstall || os.IsNotExist(statErr) {
		if err := writeUnit(); err != nil {
			return fmt.Errorf("write unit: %w", err)
		}
		if err := runSystemctl("daemon-reload"); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Autostart unit installed at %s\n", unitPath)
	}
	if err := runSystemctl("enable", "--now", linuxUnitName); err != nil {
		return fmt.Errorf("systemctl enable: %w", err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "idapt daemon is up.")
	return nil
}

func serviceDown(cmd *cobra.Command) error {
	if err := runSystemctl("stop", linuxUnitName); err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), "idapt daemon stopped.")
	return nil
}

func serviceRestart(cmd *cobra.Command) error {
	if err := runSystemctl("restart", linuxUnitName); err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), "idapt daemon restarted.")
	return nil
}

func serviceStatus(cmd *cobra.Command) error {
	c := exec.Command("systemctl", "--user", "status", linuxUnitName, "--no-pager")
	c.Stdout = cmd.OutOrStdout()
	c.Stderr = cmd.ErrOrStderr()
	_ = c.Run()
	return nil
}

func serviceLogs(cmd *cobra.Command, follow bool, lines int, since string) error {
	args := []string{"--user", "-u", linuxUnitName, "--no-pager"}
	if lines > 0 {
		args = append(args, "-n", strconv.Itoa(lines))
	}
	if since != "" {
		args = append(args, "--since", since)
	}
	if follow {
		args = append(args, "-f")
	}
	c := exec.Command("journalctl", args...)
	c.Stdout = cmd.OutOrStdout()
	c.Stderr = cmd.ErrOrStderr()
	return c.Run()
}

func serviceUninstall(cmd *cobra.Command) error {
	unitPath, err := linuxUnitPath()
	if err != nil {
		return err
	}
	_ = exec.Command("systemctl", "--user", "disable", "--now", linuxUnitName).Run()
	_ = os.Remove(unitPath)
	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	fmt.Fprintf(cmd.OutOrStdout(), "Autostart unit removed (%s).\n", unitPath)
	return nil
}

func runSystemctl(args ...string) error {
	full := append([]string{"--user"}, args...)
	c := exec.Command("systemctl", full...)
	out, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %v: %w (%s)", args, err, string(out))
	}
	return nil
}
