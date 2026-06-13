package cmd

import (
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/spf13/cobra"
)

var Version = "dev"

var (
	Commit = ""
	Date   = ""
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version",
	Run: func(cmd *cobra.Command, args []string) {
		out := cmd.OutOrStdout()
		commit, date := buildMetadata()
		fmt.Fprintf(out, "idapt %s\n", Version)
		fmt.Fprintf(out, "commit:   %s\n", commit)
		fmt.Fprintf(out, "built:    %s\n", date)
		fmt.Fprintf(out, "go:       %s\n", runtime.Version())
		fmt.Fprintf(out, "platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	},
}

func buildMetadata() (commit, date string) {
	commit, date = Commit, Date
	if bi, ok := debug.ReadBuildInfo(); ok {
		var rev, vt string
		var modified bool
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				rev = s.Value
			case "vcs.time":
				vt = s.Value
			case "vcs.modified":
				modified = s.Value == "true"
			}
		}
		if commit == "" && rev != "" {
			if len(rev) > 12 {
				rev = rev[:12]
			}
			commit = rev
			if modified {
				commit += "-dirty"
			}
		}
		if date == "" && vt != "" {
			date = vt
		}
	}
	if commit == "" {
		commit = "unknown"
	}
	if date == "" {
		date = "unknown"
	}
	return commit, date
}
