//go:build windows

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func installPlatformService(cmd *cobra.Command) error {
	return fmt.Errorf("Windows Service install not yet implemented; expected in PR12 hardening pass")
}

func uninstallPlatformService(cmd *cobra.Command) error {
	return fmt.Errorf("Windows Service uninstall not yet implemented; expected in PR12 hardening pass")
}
