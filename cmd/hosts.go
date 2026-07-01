package cmd

import "github.com/spf13/cobra"

var fileCmd = &cobra.Command{
	Use:   "drive",
	Short: "Drive — local mount (FUSE) and cloud sync",
}
