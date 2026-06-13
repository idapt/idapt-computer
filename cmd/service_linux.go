//go:build linux

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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
	state := waitForActiveState(linuxUnitName, 3*time.Second)
	switch state {
	case "active":
		fmt.Fprintln(cmd.OutOrStdout(), "idapt daemon is up.")
		return nil
	case "activating":
		fmt.Fprintln(cmd.OutOrStdout(), "idapt daemon is starting (still activating after 3s — check `idapt service status`).")
		return nil
	default:
		fmt.Fprintf(cmd.ErrOrStderr(), "idapt daemon failed to stay running (state=%s).\n\n", state)
		fmt.Fprintln(cmd.ErrOrStderr(), "Last log entries:")
		tailCmd := exec.Command("journalctl", "--user", "-u", linuxUnitName, "--no-pager", "-n", "20")
		tailCmd.Stdout = cmd.ErrOrStderr()
		tailCmd.Stderr = cmd.ErrOrStderr()
		_ = tailCmd.Run()
		fmt.Fprintln(cmd.ErrOrStderr(), "\nFollow live logs with: idapt service logs -f")
		return fmt.Errorf("daemon did not reach active state (state=%s)", state)
	}
}

func waitForActiveState(unit string, timeout time.Duration) string {
	deadline := time.Now().Add(timeout)
	last := "unknown"
	for time.Now().Before(deadline) {
		out, _ := exec.Command("systemctl", "--user", "is-active", unit).Output()
		last = strings.TrimSpace(string(out))
		switch last {
		case "active", "failed", "inactive":
			return last
		}
		time.Sleep(200 * time.Millisecond)
	}
	return last
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
