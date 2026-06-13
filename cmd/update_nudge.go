package cmd

import (
	"fmt"
	"os"

	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/idapt/idapt-cli/internal/update"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

func updateNudgeEligible(cmd *cobra.Command) bool {
	if Version == "dev" {
		return false
	}
	if !isatty.IsTerminal(os.Stdout.Fd()) {
		return false
	}
	if isDaemonCommand(cmd) {
		return false
	}
	switch cmd.Name() {
	case "update", "version", "idapt":
		return false
	}
	return true
}

func kickUpdateCheck(cmd *cobra.Command, appURL string) {
	if !updateNudgeEligible(cmd) {
		return
	}
	update.MaybeRefreshInBackground(appURL, Version, update.DefaultNudgeTTL)
}

func printUpdateNudge(cmd *cobra.Command) {
	if !updateNudgeEligible(cmd) {
		return
	}
	f := cmdutil.FactoryFromCmd(cmd)
	if f == nil || f.Format != output.FormatTable {
		return
	}
	st := update.LoadCheckState()
	banner, ok := update.Nudge(Version, st.LatestVersion)
	if !ok {
		return
	}
	if !st.ShouldNudge(update.NudgeDisplayInterval) {
		return
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "\n%s\n", banner)
	_ = update.MarkNudged()
}
