//go:build darwin

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

const macPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>ai.idapt.computer</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>serve</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key>
  <dict>
    <key>SuccessfulExit</key><false/>
  </dict>
  <key>StandardOutPath</key>
  <string>%s/Library/Logs/Idapt/daemon.log</string>
  <key>StandardErrorPath</key>
  <string>%s/Library/Logs/Idapt/daemon.err</string>
</dict>
</plist>
`

const darwinLabel = "ai.idapt.computer"

func darwinPlistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", darwinLabel+".plist"), nil
}

func darwinUID() string {
	return fmt.Sprintf("%d", os.Getuid())
}

func writePlist() (string, error) {
	binPath, err := os.Executable()
	if err != nil {
		return "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	plistPath, err := darwinPlistPath()
	if err != nil {
		return "", err
	}
	logsDir := filepath.Join(home, "Library", "Logs", "Idapt")
	_ = os.MkdirAll(logsDir, 0o755)
	if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(plistPath, []byte(fmt.Sprintf(macPlist, binPath, home, home)), 0o644); err != nil {
		return "", err
	}
	return plistPath, nil
}

func serviceUp(cmd *cobra.Command, reinstall bool) error {
	plistPath, err := darwinPlistPath()
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(plistPath); reinstall || os.IsNotExist(statErr) {
		if _, err := writePlist(); err != nil {
			return fmt.Errorf("write plist: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Autostart LaunchAgent installed at %s\n", plistPath)
	}
	uid := darwinUID()
	_ = exec.Command("launchctl", "bootout", "gui/"+uid, plistPath).Run()
	if err := exec.Command("launchctl", "bootstrap", "gui/"+uid, plistPath).Run(); err != nil {
		return fmt.Errorf("launchctl bootstrap: %w", err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "idapt daemon is up.")
	return nil
}

func serviceDown(cmd *cobra.Command) error {
	plistPath, err := darwinPlistPath()
	if err != nil {
		return err
	}
	uid := darwinUID()
	if err := exec.Command("launchctl", "bootout", "gui/"+uid, plistPath).Run(); err != nil {
		return fmt.Errorf("launchctl bootout: %w", err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "idapt daemon stopped.")
	return nil
}

func serviceRestart(cmd *cobra.Command) error {
	uid := darwinUID()
	target := "gui/" + uid + "/" + darwinLabel
	if err := exec.Command("launchctl", "kickstart", "-k", target).Run(); err != nil {
		return fmt.Errorf("launchctl kickstart: %w", err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "idapt daemon restarted.")
	return nil
}

func serviceStatus(cmd *cobra.Command) error {
	uid := darwinUID()
	target := "gui/" + uid + "/" + darwinLabel
	c := exec.Command("launchctl", "print", target)
	c.Stdout = cmd.OutOrStdout()
	c.Stderr = cmd.ErrOrStderr()
	_ = c.Run()
	return nil
}

func serviceLogs(cmd *cobra.Command, follow bool, lines int, _since string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	logFile := filepath.Join(home, "Library", "Logs", "Idapt", "daemon.log")
	errFile := filepath.Join(home, "Library", "Logs", "Idapt", "daemon.err")
	args := []string{"-n", fmt.Sprintf("%d", lines)}
	if follow {
		args = append(args, "-F")
	}
	args = append(args, logFile, errFile)
	c := exec.Command("tail", args...)
	c.Stdout = cmd.OutOrStdout()
	c.Stderr = cmd.ErrOrStderr()
	return c.Run()
}

func serviceUninstall(cmd *cobra.Command) error {
	plistPath, err := darwinPlistPath()
	if err != nil {
		return err
	}
	uid := darwinUID()
	_ = exec.Command("launchctl", "bootout", "gui/"+uid, plistPath).Run()
	_ = os.Remove(plistPath)
	fmt.Fprintf(cmd.OutOrStdout(), "Autostart LaunchAgent removed (%s).\n", plistPath)
	return nil
}
