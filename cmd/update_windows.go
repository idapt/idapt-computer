//go:build windows

package cmd

import "fmt"

func signalDaemonRestart(version string) string {
	if err := restartWindowsDaemon(); err != nil {
		return fmt.Sprintf(
			"Updated to %s, but the automatic restart failed: %v. Run `idapt-computer service restart` to apply.",
			version, err,
		)
	}
	return fmt.Sprintf("Updated to %s. Daemon service restarted with the new binary.", version)
}
