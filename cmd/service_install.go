package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage the per-OS daemon service installation",
}

var serviceInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the daemon as a service for the current OS",
	RunE: func(cmd *cobra.Command, args []string) error {
		return installPlatformService(cmd)
	},
}

var serviceUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the daemon service",
	RunE: func(cmd *cobra.Command, args []string) error {
		return uninstallPlatformService(cmd)
	},
}

func init() {
	serviceCmd.AddCommand(serviceInstallCmd)
	serviceCmd.AddCommand(serviceUninstallCmd)
	rootCmd.AddCommand(serviceCmd)
}

func defaultServiceErr() error {
	return fmt.Errorf("service install not yet implemented for this OS")
}
