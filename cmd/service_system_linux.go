//go:build linux

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)
const linuxSystemUnit = `[Unit]
Description=Idapt daemon (system)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
Environment=IDAPT_ALLOW_RUNAS_ROOT=1
ExecStart=%s serve
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
`

const linuxSystemUnitPath = "/etc/systemd/system/" + linuxUnitName

func renderSystemUnit(binPath string) string {
	return fmt.Sprintf(linuxSystemUnit, binPath)
}

func writeSystemUnit() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("writing the system unit requires root — re-run with sudo")
	}
	binPath, err := os.Executable()
	if err != nil {
		return err
	}
	if err := assertSecureSystemBinaryPath(binPath); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(linuxSystemUnitPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(linuxSystemUnitPath, []byte(renderSystemUnit(binPath)), 0o644)
}

func assertSecureSystemBinaryPath(binPath string) error {
	resolved, err := filepath.EvalSymlinks(binPath)
	if err != nil {
		resolved = binPath
	}
	dir := filepath.Dir(resolved)
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("stat binary dir %s: %w", dir, err)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("cannot determine ownership of %s", dir)
	}
	if st.Uid != 0 {
		return fmt.Errorf(
			"refusing to install a root system service that runs %s: its directory %s is owned by uid %d (not root), so a non-root user could replace the binary and gain root. Install the binary under a root-owned path (e.g. /usr/local/bin) first",
			resolved, dir, st.Uid)
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf(
			"refusing to install a root system service that runs %s: its directory %s is group/world-writable (mode %o), so a non-root user could replace the binary and gain root",
			resolved, dir, info.Mode().Perm())
	}
	return nil
}

func installSystemService(cmd *cobra.Command, reinstall bool) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("installing the system service requires root — re-run `idapt-computer up --system` (or `service elevate`) with sudo")
	}
	warnConflictingUserScopeFromSystem(cmd)
	if _, statErr := os.Stat(linuxSystemUnitPath); reinstall || os.IsNotExist(statErr) {
		if err := writeSystemUnit(); err != nil {
			return fmt.Errorf("write system unit: %w", err)
		}
		if err := runSystemSystemctl("daemon-reload"); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "System unit installed at %s\n", linuxSystemUnitPath)
	}
	if err := runSystemSystemctl("enable", "--now", linuxUnitName); err != nil {
		return fmt.Errorf("systemctl enable: %w", err)
	}
	state := waitForSystemActiveState(linuxUnitName, 3*time.Second)
	switch state {
	case "active":
		fmt.Fprintln(cmd.OutOrStdout(), "idapt daemon (system) is up.")
		return nil
	case "activating":
		fmt.Fprintln(cmd.OutOrStdout(), "idapt daemon (system) is starting (still activating after 3s — check `systemctl status idapt-computer`).")
		return nil
	default:
		fmt.Fprintf(cmd.ErrOrStderr(), "idapt daemon (system) failed to stay running (state=%s).\n\n", state)
		fmt.Fprintln(cmd.ErrOrStderr(), "Last log entries:")
		tailCmd := exec.Command("journalctl", "-u", linuxUnitName, "--no-pager", "-n", "20")
		tailCmd.Stdout = cmd.ErrOrStderr()
		tailCmd.Stderr = cmd.ErrOrStderr()
		_ = tailCmd.Run()
		return fmt.Errorf("system daemon did not reach active state (state=%s)", state)
	}
}

func uninstallSystemService(cmd *cobra.Command) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("removing the system service requires root — re-run with sudo")
	}
	_ = exec.Command("systemctl", "disable", "--now", linuxUnitName).Run()
	_ = os.Remove(linuxSystemUnitPath)
	_ = exec.Command("systemctl", "daemon-reload").Run()
	fmt.Fprintf(cmd.OutOrStdout(), "System unit removed (%s).\n", linuxSystemUnitPath)
	return nil
}

func warnConflictingUserScopeFromSystem(cmd *cobra.Command) {
	sudoUser := os.Getenv("SUDO_USER")
	if sudoUser == "" {
		return
	}
	userUnit := filepath.Join("/home", sudoUser, ".config", "systemd", "user", linuxUnitName)
	if _, err := os.Stat(userUnit); err != nil {
		return
	}
	fmt.Fprintf(cmd.ErrOrStderr(),
		"WARN: a USER-scope daemon unit is also installed for %s (%s).\n"+
			"      Two units start two daemons that collide on the management port.\n"+
			"      Remove it:  sudo -u %s XDG_RUNTIME_DIR=/run/user/$(id -u %s) systemctl --user disable --now %s\n",
		sudoUser, userUnit, sudoUser, sudoUser, linuxUnitName)
}

func runSystemSystemctl(args ...string) error {
	c := exec.Command("systemctl", args...)
	out, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %v: %w (%s)", args, err, string(out))
	}
	return nil
}

func waitForSystemActiveState(unit string, timeout time.Duration) string {
	deadline := time.Now().Add(timeout)
	last := "unknown"
	for time.Now().Before(deadline) {
		out, _ := exec.Command("systemctl", "is-active", unit).Output()
		last = strings.TrimSpace(string(out))
		switch last {
		case "active", "failed", "inactive":
			return last
		}
		time.Sleep(200 * time.Millisecond)
	}
	return last
}
