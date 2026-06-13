package cmd

import "github.com/spf13/cobra"

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Chat — the interactive TUI (`chat ask`)",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	},
}

var fileCmd = &cobra.Command{
	Use:   "drive",
	Short: "Drive — local mount (FUSE) and cloud sync",
}
