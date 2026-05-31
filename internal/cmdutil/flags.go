package cmdutil

import (
	"time"

	"github.com/spf13/cobra"
)

type GlobalFlags struct {
	APIKey       string
	Workspace    string
	APIURL       string
	Output       string
	Quiet        bool
	NoColor      bool
	Verbose      bool
	Confirm      bool
	Timeout      time.Duration
	Instructions bool
	JSONInput    string
}

func RegisterGlobalFlags(root *cobra.Command) *GlobalFlags {
	f := &GlobalFlags{}
	root.PersistentFlags().StringVar(&f.APIKey, "api-key", "", "API key for authentication")
	root.PersistentFlags().StringVarP(&f.Workspace, "workspace", "w", "", "Workspace slug or ID")
	root.PersistentFlags().StringVar(&f.APIURL, "api-url", "", "API base URL (default https://idapt.ai)")
	root.PersistentFlags().StringVarP(&f.Output, "output", "o", "", "Output format: table|json|jsonl|quiet")
	root.PersistentFlags().BoolVarP(&f.Quiet, "quiet", "q", false, "Quiet output (alias for --output quiet)")
	root.PersistentFlags().BoolVar(&f.NoColor, "no-color", false, "Disable color output")
	root.PersistentFlags().BoolVarP(&f.Verbose, "verbose", "v", false, "Verbose output")
	root.PersistentFlags().DurationVar(&f.Timeout, "timeout", 0, "Per-request HTTP timeout (e.g. 30s, 2m); 0 = default 60s")
	root.PersistentFlags().BoolVarP(&f.Confirm, "yes", "y", false, "Skip confirmation prompts (assume yes)")
	root.PersistentFlags().BoolVar(&f.Confirm, "confirm", false, "Skip confirmation prompts (deprecated alias for --yes)")
	_ = root.PersistentFlags().MarkDeprecated("confirm", "use --yes/-y instead")
	root.PersistentFlags().BoolVar(&f.Instructions, "instructions", false, "Show the resource instructions (when/why/anti-patterns) instead of running")
	return f
}

func AddListFlags(cmd *cobra.Command) {
	cmd.Flags().Int("limit", 50, "Maximum items to return (server max 100)")
	cmd.Flags().String("cursor", "", "Opaque pagination cursor (pass the previous response's next_cursor)")
	if cmd.Flags().Lookup("columns") == nil {
		cmd.Flags().String("columns", "", "Comma-separated field paths to display (table only), e.g. id,state")
	}
	if cmd.Flags().Lookup("filter") == nil {
		cmd.Flags().StringArray("filter", nil, "Keep rows matching field=value (exact) or field~value (contains); repeatable")
	}
	if cmd.Flags().Lookup("sort") == nil {
		cmd.Flags().String("sort", "", "Sort rows by a field path; prefix with - for descending (e.g. -created_at)")
	}
}

func AddAllFlag(cmd *cobra.Command) {
	cmd.Flags().Bool("all", false, "Fetch every page (walk pagination cursors); overrides --limit/--cursor")
}

func AddJSONInput(cmd *cobra.Command) {
	cmd.Flags().String("json", "", "JSON input (inline or - for stdin)")
}
