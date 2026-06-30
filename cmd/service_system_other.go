//go:build !linux

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)
func installSystemService(_ *cobra.Command, _ bool) error {
	return fmt.Errorf("system mode is Linux-only — use the per-user service on this OS")
}

func uninstallSystemService(_ *cobra.Command) error {
	return fmt.Errorf("system mode is Linux-only — there is no system service to remove on this OS")
}

func enableLingerForCurrentUser(_ *cobra.Command) {}
