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

	"github.com/idapt/idapt-computer/internal/config"
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

const linuxUnitName = "idapt-computer.service"

const linuxUpdateUnitName = "idapt-update.service"
const linuxUpdateTimerName = "idapt-update.timer"

const linuxUpdateService = `[Unit]
Description=Idapt daemon self-update

[Service]
Type=oneshot
ExecStart=%s update --quiet
`

const linuxUpdateTimer = `[Unit]
Description=Idapt daemon update timer

[Timer]
OnBootSec=10min
OnUnitActiveSec=6h
RandomizedDelaySec=600

[Install]
WantedBy=timers.target
`

func linuxUnitPath() (string, error) {
	return linuxUserUnitPath(linuxUnitName)
}

func linuxUserUnitPath(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "systemd", "user", name), nil
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
	warnConflictingLinuxScope(cmd)
	if _, statErr := os.Stat(unitPath); reinstall || os.IsNotExist(statErr) {
		if err := writeUnit(); err != nil {
			return fmt.Errorf("write unit: %w", err)
		}
		if err := runSystemctl("daemon-reload"); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Autostart unit installed at %s\n", unitPath)
	}

	if serviceAutostart != config.AutostartManual {
		if err := installLinuxUpdateTimer(cmd, reinstall); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "WARN: could not install auto-update timer: %v\n", err)
		}
	}

	switch serviceAutostart {
	case config.AutostartManual:
		fmt.Fprintln(cmd.OutOrStdout(), "Autostart policy: manual (unit installed; start with `idapt-computer service up --autostart always` or `service restart`).")
		return nil
	case config.AutostartApp:
		if err := runSystemctl("start", linuxUnitName); err != nil {
			return fmt.Errorf("systemctl start: %w", err)
		}
	default: // always
		if err := runSystemctl("enable", "--now", linuxUnitName); err != nil {
			return fmt.Errorf("systemctl enable: %w", err)
		}
	}
	state := waitForActiveState(linuxUnitName, 3*time.Second)
	switch state {
	case "active":
		fmt.Fprintln(cmd.OutOrStdout(), "idapt daemon is up.")
		return nil
	case "activating":
		fmt.Fprintln(cmd.OutOrStdout(), "idapt daemon is starting (still activating after 3s — check `idapt-computer service status`).")
		return nil
	default:
		fmt.Fprintf(cmd.ErrOrStderr(), "idapt daemon failed to stay running (state=%s).\n\n", state)
		fmt.Fprintln(cmd.ErrOrStderr(), "Last log entries:")
		tailCmd := exec.Command("journalctl", "--user", "-u", linuxUnitName, "--no-pager", "-n", "20")
		tailCmd.Stdout = cmd.ErrOrStderr()
		tailCmd.Stderr = cmd.ErrOrStderr()
		_ = tailCmd.Run()
		fmt.Fprintln(cmd.ErrOrStderr(), "\nFollow live logs with: idapt-computer service logs -f")
		return fmt.Errorf("daemon did not reach active state (state=%s)", state)
	}
}

func installLinuxUpdateTimer(cmd *cobra.Command, reinstall bool) error {
	binPath, err := os.Executable()
	if err != nil {
		return err
	}
	svcPath, err := linuxUserUnitPath(linuxUpdateUnitName)
	if err != nil {
		return err
	}
	timerPath, err := linuxUserUnitPath(linuxUpdateTimerName)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(svcPath), 0o755); err != nil {
		return err
	}
	wrote := false
	if _, statErr := os.Stat(svcPath); reinstall || os.IsNotExist(statErr) {
		if err := os.WriteFile(svcPath, []byte(fmt.Sprintf(linuxUpdateService, binPath)), 0o644); err != nil {
			return err
		}
		wrote = true
	}
	if _, statErr := os.Stat(timerPath); reinstall || os.IsNotExist(statErr) {
		if err := os.WriteFile(timerPath, []byte(linuxUpdateTimer), 0o644); err != nil {
			return err
		}
		wrote = true
	}
	if wrote {
		if err := runSystemctl("daemon-reload"); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Auto-update timer installed (%s).\n", timerPath)
	}
	return runSystemctl("enable", "--now", linuxUpdateTimerName)
}

func removeLinuxUpdateTimer() {
	_ = exec.Command("systemctl", "--user", "disable", "--now", linuxUpdateTimerName).Run()
	if p, err := linuxUserUnitPath(linuxUpdateTimerName); err == nil {
		_ = os.Remove(p)
	}
	if p, err := linuxUserUnitPath(linuxUpdateUnitName); err == nil {
		_ = os.Remove(p)
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
	removeLinuxUpdateTimer()
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

func enableLingerForCurrentUser(cmd *cobra.Command) {
	uname := os.Getenv("USER")
	if uname == "" {
		uname = detectDefaultUser()
	}
	if uname == "" || uname == "user" {
		return
	}
	out, err := exec.Command("loginctl", "enable-linger", uname).CombinedOutput()
	if err == nil {
		fmt.Fprintf(cmd.OutOrStdout(), "Enabled linger for %s — the daemon will keep running after logout and start on boot.\n", uname)
		return
	}
	fmt.Fprintf(cmd.ErrOrStderr(),
		"NOTE: could not enable linger for %s (%s).\n"+
			"      The daemon will stop when you log out and will NOT auto-start after a reboot.\n"+
			"      Enable it once with:  sudo loginctl enable-linger %s\n",
		uname, strings.TrimSpace(string(out)), uname)
}

func userUnitInstalled() bool {
	p, err := linuxUnitPath()
	if err != nil {
		return false
	}
	_, statErr := os.Stat(p)
	return statErr == nil
}

func systemUnitInstalled() bool {
	_, err := os.Stat(linuxSystemUnitPath)
	return err == nil
}

func warnConflictingLinuxScope(cmd *cobra.Command) {
	if os.Geteuid() != 0 && systemUnitInstalled() {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"WARN: a SYSTEM-scope daemon unit is also installed (%s).\n"+
				"      Two units start two daemons that collide on the management port.\n"+
				"      Remove one:  sudo systemctl disable --now %s   (or run `idapt-computer service doctor`)\n",
			linuxSystemUnitPath, linuxUnitName)
	}
}

func collectAutostartFindings() []autostartFinding {
	return linuxAutostartFindings(userUnitInstalled(), systemUnitInstalled())
}

func repairAutostart(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	if !(userUnitInstalled() && systemUnitInstalled()) {
		fmt.Fprintln(out, "  nothing to repair")
		return nil
	}
	if os.Geteuid() == 0 {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"  both scopes are installed. Re-run `idapt-computer service doctor --fix` as your normal user to drop the\n"+
				"  per-user unit, or keep user-scope and drop system with:  systemctl disable --now %s && rm %s\n",
			linuxUnitName, linuxSystemUnitPath)
		return nil
	}
	_ = exec.Command("systemctl", "--user", "disable", "--now", linuxUnitName).Run()
	if p, err := linuxUnitPath(); err == nil {
		_ = os.Remove(p)
	}
	removeLinuxUpdateTimer()
	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	fmt.Fprintf(out, "  removed the per-user unit (kept the system unit at %s)\n", linuxSystemUnitPath)
	return nil
}
