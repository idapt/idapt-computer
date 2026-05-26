//go:build windows

package cmd

import (
	"github.com/spf13/cobra"
)
func serviceUp(_ *cobra.Command, _ bool) error      { return notImplementedForOS("up") }
func serviceDown(_ *cobra.Command) error            { return notImplementedForOS("down") }
func serviceRestart(_ *cobra.Command) error         { return notImplementedForOS("restart") }
func serviceStatus(_ *cobra.Command) error          { return notImplementedForOS("status") }
func serviceLogs(_ *cobra.Command, _ bool, _ int, _ string) error {
	return notImplementedForOS("logs")
}
func serviceUninstall(_ *cobra.Command) error { return notImplementedForOS("uninstall") }
