//go:build windows

package cmd

import "fmt"

func signalDaemonRestart(version string) string {
	return fmt.Sprintf(
		"Updated to %s. Restart the idapt-computer service to apply the new version (e.g. `idapt-computer service restart`, or restart it from Services).",
		version,
	)
}
