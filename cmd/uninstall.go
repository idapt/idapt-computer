package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/idaptpaths"
	"github.com/spf13/cobra"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove idapt from this machine (binary, autostart, optionally user data)",
	Long: `Removes idapt from this machine.

By default:
  - Stops the daemon and removes the autostart unit
  - Unmounts any active FUSE drives
  - Deletes the binary
  - KEEPS your config, cache, and data (credentials, chat history cache,
    local-inference models) so a future reinstall picks up where you left off

With --purge, also removes:
  - Per-user config dir   (credentials, CLI preferences)
  - Per-user cache dir    (model list, flag cache, FUSE blob cache)
  - Per-user data dir     (local-inference models — can be many GB)

Interactive by default. The exact file list is shown before anything is
deleted. Use --confirm (or the global --confirm flag) to skip the prompt
for CI / scripted use.`,
	RunE: runUninstall,
}

func init() {
	uninstallCmd.Flags().Bool("purge", false, "Also remove config, cache, and data directories (irreversible — wipes credentials and local models)")
	rootCmd.AddCommand(uninstallCmd)
}

type uninstallEntry struct {
	Path  string
	Label string
}

type uninstallPlan struct {
	Binary       string           // resolved binary path, or "" if undeterminable
	Remove       []uninstallEntry // dirs / files we'll delete
	Keep         []uninstallEntry // dirs we'll leave in place (rendered as "Kept:")
	Purge        bool
	RunningAsRoot bool // for messaging the user about sudo requirements
}

func runUninstall(cmd *cobra.Command, _ []string) error {
	purge, _ := cmd.Flags().GetBool("purge")
	plan, err := buildUninstallPlan(purge)
	if err != nil {
		return fmt.Errorf("build uninstall plan: %w", err)
	}
	renderUninstallPlan(cmd, plan)
	if !confirmUninstall(cmd) {
		fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
		return nil
	}
	return executeUninstall(cmd, plan)
}

func buildUninstallPlan(purge bool) (uninstallPlan, error) {
	plan := uninstallPlan{Purge: purge, RunningAsRoot: os.Geteuid() == 0}

	if exe, err := os.Executable(); err == nil {
		plan.Binary = exe
		plan.Remove = append(plan.Remove, uninstallEntry{
			Path:  exe,
			Label: "idapt binary",
		})
	}

	plan.Remove = append(plan.Remove, uninstallEntry{
		Path:  "(per-OS autostart unit)",
		Label: "autostart unit (via `service uninstall`)",
	})

	cfg, cfgErr := idaptpaths.ConfigDir()
	cache, cacheErr := idaptpaths.CacheDir()
	data, dataErr := idaptpaths.DataDir()

	dirs := []struct {
		path  string
		err   error
		label string
	}{
		{cfg, cfgErr, "config dir (credentials, settings)"},
		{cache, cacheErr, "cache dir (model list, flag cache, FUSE blobs)"},
		{data, dataErr, "data dir (local-inference models)"},
	}
	for _, d := range dirs {
		if d.err != nil || d.path == "" {
			continue
		}
		if _, statErr := os.Stat(d.path); statErr != nil {
			continue
		}
		entry := uninstallEntry{Path: d.path, Label: d.label}
		if purge {
			plan.Remove = append(plan.Remove, entry)
		} else {
			plan.Keep = append(plan.Keep, entry)
		}
	}
	return plan, nil
}

func renderUninstallPlan(cmd *cobra.Command, plan uninstallPlan) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "The following will be removed:")
	for _, e := range plan.Remove {
		fmt.Fprintf(out, "  - %s — %s\n", e.Path, e.Label)
	}
	if len(plan.Keep) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Kept (use --purge to remove these too):")
		for _, e := range plan.Keep {
			fmt.Fprintf(out, "  - %s — %s\n", e.Path, e.Label)
		}
	}
	if plan.Binary != "" && !plan.RunningAsRoot && !isWritable(plan.Binary) {
		fmt.Fprintln(out)
		fmt.Fprintf(out, "Note: removing %s requires sudo — you may be prompted.\n", plan.Binary)
	}
}

func confirmUninstall(cmd *cobra.Command) bool {
	if globalFlags != nil && globalFlags.Confirm {
		return true
	}
	return cmdutil.ConfirmAction(cmdutil.FactoryFromCmd(cmd), "Proceed with uninstall?")
}

func executeUninstall(cmd *cobra.Command, plan uninstallPlan) error {
	out := cmd.OutOrStdout()
	var errs []string

	if err := serviceDown(silenced(cmd)); err != nil {
		fmt.Fprintf(out, "  daemon: already stopped\n")
	} else {
		fmt.Fprintln(out, "  daemon: stopped")
	}

	if err := serviceUninstall(silenced(cmd)); err != nil {
		errs = append(errs, fmt.Sprintf("remove autostart unit: %v", err))
	} else {
		fmt.Fprintln(out, "  autostart unit: removed")
	}

	for _, e := range plan.Remove {
		if e.Path == "" || e.Path == "(per-OS autostart unit)" {
			continue
		}
		if e.Path == plan.Binary {
			continue // binary is handled last
		}
		if err := os.RemoveAll(e.Path); err != nil {
			errs = append(errs, fmt.Sprintf("remove %s: %v", e.Path, err))
		} else {
			fmt.Fprintf(out, "  removed: %s\n", e.Path)
		}
	}

	if plan.Binary != "" {
		if err := removeBinary(plan.Binary); err != nil {
			errs = append(errs, fmt.Sprintf("remove binary %s: %v", plan.Binary, err))
		} else {
			fmt.Fprintf(out, "  removed: %s\n", plan.Binary)
		}
	}

	if len(errs) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Uninstall completed with errors:")
		for _, e := range errs {
			fmt.Fprintf(out, "  - %s\n", e)
		}
		return fmt.Errorf("uninstall: %d step(s) failed", len(errs))
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "idapt uninstalled.")
	return nil
}

func removeBinary(path string) error {
	if err := os.Remove(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrPermission) {
		return err
	}
	if os.Geteuid() == 0 {
		return os.Remove(path)
	}
	sudo, err := exec.LookPath("sudo")
	if err != nil {
		return fmt.Errorf("removing %s requires elevated permissions and sudo is not available; re-run as root", path)
	}
	c := exec.Command(sudo, "rm", "-f", path)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func isWritable(path string) bool {
	parent := dir(path)
	f, err := os.CreateTemp(parent, ".idapt-uninstall-probe-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}

func dir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}

func silenced(cmd *cobra.Command) *cobra.Command {
	clone := *cmd
	clone.SetOut(io_discard{})
	clone.SetErr(io_discard{})
	return &clone
}

type io_discard struct{}

func (io_discard) Write(p []byte) (int, error) { return len(p), nil }
