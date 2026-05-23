package cmd

import (
	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/tui"
	"github.com/spf13/cobra"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch the interactive TUI",
	Long: `Launch the interactive TUI.

This is equivalent to running ` + "`idapt`" + ` with no subcommand in a terminal.
Use this explicit form in scripts, aliases, or when you want to bypass the
TTY heuristic (which suppresses the TUI when piped to/from another process).

Set IDAPT_NO_TUI=1 to disable the automatic boot from the bare ` + "`idapt`" + ` command.

Availability: gated on the ` + "`tui`" + ` feature flag — see ` + "`idapt instructions tui`" + ` if you get an "unavailable" error.`,
	PreRunE: requireTUIFlag,
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		return tui.Run(cmd.Context(), f)
	},
}

func init() {
	tuiCmd.Annotations = map[string]string{
		InstructionsAnnotationKey: tuiInstructions,
	}

	origRootHelp := rootCmd.HelpFunc()
	rootCmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		applyTUIVisibility()
		origRootHelp(c, args)
	})
}

const tuiInstructions = `Interactive chat TUI.

When:
  - You want to chat conversationally with an idapt agent in your terminal.
  - You want to use the same models / agents / projects you'd use in the web app,
    but with keyboard-driven flow and no browser.

Launch:
  idapt           # auto-boots TUI if stdin+stdout are ttys and no -p/--json
  idapt tui       # explicit; never auto-suppressed

Inside the TUI:
  Enter           send
  Shift+Enter     newline (or Ctrl+J)
  Ctrl+C          cancel current stream / quit if composer empty
  Ctrl+D          quit (composer empty)
  /help           list keybindings + slash commands

For one-shot non-interactive mode:
  idapt -p "explain this regex"
  echo "hello" | idapt -p
  idapt chat ask "what is 2+2?" --stream

Disable auto-boot in scripts: IDAPT_NO_TUI=1.
`
