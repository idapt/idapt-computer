package cmdutil

import (
	"github.com/spf13/cobra"
)

type GlobalFlags struct {
	APIKey       string
	Workspace      string
	APIURL       string
	Output       string
	NoColor      bool
	Verbose      bool
	Confirm      bool
	Instructions bool
	JSONInput    string
}

func RegisterGlobalFlags(root *cobra.Command) *GlobalFlags {
	f := &GlobalFlags{}
	root.PersistentFlags().StringVar(&f.APIKey, "api-key", "", "API key for authentication")
	root.PersistentFlags().StringVar(&f.Workspace, "workspace", "", "Workspace slug or ID")
	root.PersistentFlags().StringVar(&f.APIURL, "api-url", "", "API base URL (default https://idapt.ai)")
	root.PersistentFlags().StringVarP(&f.Output, "output", "o", "", "Output format: table|json|jsonl|quiet")
	root.PersistentFlags().BoolVar(&f.NoColor, "no-color", false, "Disable color output")
	root.PersistentFlags().BoolVar(&f.Verbose, "verbose", false, "Verbose output")
	root.PersistentFlags().BoolVar(&f.Confirm, "confirm", false, "Skip confirmation prompts")
	root.PersistentFlags().BoolVar(&f.Instructions, "instructions", false, "Show the resource instructions (when/why/anti-patterns) instead of running")
	return f
}

func AddListFlags(cmd *cobra.Command) {
	cmd.Flags().Int("limit", 50, "Maximum items to return (server max 100)")
	cmd.Flags().String("cursor", "", "Opaque pagination cursor (pass the previous response's next_cursor)")
}

func AddJSONInput(cmd *cobra.Command) {
	cmd.Flags().String("json", "", "JSON input (inline or - for stdin)")
}
