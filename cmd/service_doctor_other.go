//go:build !windows && !linux && !darwin

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)
func collectAutostartFindings() []autostartFinding {
	return []autostartFinding{{
		Severity: sevWarn,
		Title:    "Autostart diagnostics are not implemented for this OS",
	}}
}

func repairAutostart(_ *cobra.Command) error {
	return fmt.Errorf("service doctor --fix is not supported on this OS")
}
