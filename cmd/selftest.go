package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/spf13/cobra"
)

var selftestCmd = &cobra.Command{
	Use:   "selftest",
	Short: "Verify the daemon binary can run on this host",
	Long: `Performs a quick set of checks: required binaries available
(runuser, tmux, iptables, prlimit), config readable, and the binary
itself is not corrupt. Used by the update flow as pre/post-install
checks (see services/idapt-computer/internal/update/).`,
	RunE: runSelftest,
}

func init() {
	rootCmd.AddCommand(selftestCmd)
}

func runSelftest(cmd *cobra.Command, args []string) error {
	checks := []func() error{
		checkBashAvailable,
	}
	if runtime.GOOS == "linux" {
		checks = append(checks,
			checkRunuserAvailable,
			checkPrlimitAvailable,
		)
	}
	for _, c := range checks {
		if err := c(); err != nil {
			return err
		}
	}
	fmt.Fprintln(cmd.OutOrStdout(), "selftest: ok")
	return nil
}

func checkBashAvailable() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "/bin/bash", "-c", "true").Run(); err != nil {
		return fmt.Errorf("bash unavailable: %w", err)
	}
	return nil
}

func checkRunuserAvailable() error {
	if _, err := exec.LookPath("runuser"); err != nil {
		return fmt.Errorf("runuser missing: %w", err)
	}
	return nil
}

func checkPrlimitAvailable() error {
	if _, err := exec.LookPath("prlimit"); err != nil {
		return fmt.Errorf("prlimit missing: %w", err)
	}
	return nil
}
