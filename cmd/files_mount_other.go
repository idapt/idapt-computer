//go:build !linux && !darwin

package cmd

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)
func newUnsupportedDriveFSCmd(use, short string) *cobra.Command {
	verb := strings.Fields(use)[0]
	return &cobra.Command{
		Use:   use,
		Short: short + " (Linux/macOS only)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf(
				"drive %s requires a FUSE filesystem, which is not available on %s — "+
					"Drive mount/sync are supported on Linux and macOS only",
				verb, runtime.GOOS,
			)
		},
	}
}

func init() {
	fileCmd.AddCommand(newUnsupportedDriveFSCmd("mount <workspace> <mountpoint>", "Mount your Drive as a local filesystem"))
	fileCmd.AddCommand(newUnsupportedDriveFSCmd("unmount <mountpoint>", "Unmount a Drive FUSE filesystem"))
	fileCmd.AddCommand(newUnsupportedDriveFSCmd("sync <workspace> <local-path>", "Sync Drive with a local directory"))
}
