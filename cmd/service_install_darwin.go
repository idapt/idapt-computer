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
  <string>ai.idapt.daemon</string>
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

func installPlatformService(cmd *cobra.Command) error {
	binPath, err := os.Executable()
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	plistPath := filepath.Join(home, "Library", "LaunchAgents", "ai.idapt.daemon.plist")
	logsDir := filepath.Join(home, "Library", "Logs", "Idapt")
	_ = os.MkdirAll(logsDir, 0o755)
	if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(plistPath, []byte(fmt.Sprintf(macPlist, binPath, home, home)), 0o644); err != nil {
		return err
	}
	uid := fmt.Sprintf("%d", os.Getuid())
	_ = exec.Command("launchctl", "bootout", "gui/"+uid, plistPath).Run() // ok if not loaded
	if err := exec.Command("launchctl", "bootstrap", "gui/"+uid, plistPath).Run(); err != nil {
		return fmt.Errorf("launchctl bootstrap: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "macOS LaunchAgent installed at %s\n", plistPath)
	return nil
}

func uninstallPlatformService(cmd *cobra.Command) error {
	home, _ := os.UserHomeDir()
	plistPath := filepath.Join(home, "Library", "LaunchAgents", "ai.idapt.daemon.plist")
	uid := fmt.Sprintf("%d", os.Getuid())
	_ = exec.Command("launchctl", "bootout", "gui/"+uid, plistPath).Run()
	_ = os.Remove(plistPath)
	fmt.Fprintln(cmd.OutOrStdout(), "macOS LaunchAgent uninstalled")
	return nil
}
