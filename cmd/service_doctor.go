package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)
type autostartSeverity string

const (
	sevOK    autostartSeverity = "OK"
	sevWarn  autostartSeverity = "WARN"
	sevError autostartSeverity = "ERROR"
)

type autostartFinding struct {
	Severity autostartSeverity
	Title    string
	Detail   string
	Fixable  bool
}

func findingsHaveError(fs []autostartFinding) bool {
	for _, f := range fs {
		if f.Severity == sevError {
			return true
		}
	}
	return false
}

func findingsHaveFixable(fs []autostartFinding) bool {
	for _, f := range fs {
		if f.Fixable {
			return true
		}
	}
	return false
}

func findingsAllOK(fs []autostartFinding) bool {
	for _, f := range fs {
		if f.Severity != sevOK {
			return false
		}
	}
	return true
}

func countProcessIDRecords(out string) int {
	n := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "ProcessId") {
			n++
		}
	}
	return n
}

func printAutostartFindings(w io.Writer, fs []autostartFinding) {
	for _, f := range fs {
		fmt.Fprintf(w, "  [%s] %s\n", f.Severity, f.Title)
		if f.Detail != "" {
			fmt.Fprintf(w, "         %s\n", f.Detail)
		}
	}
}

func windowsAutostartFindings(taskExists, runKeyExists bool, serveProcs int) []autostartFinding {
	var out []autostartFinding
	switch {
	case taskExists && runKeyExists:
		out = append(out, autostartFinding{
			Severity: sevError,
			Title:    "Duplicate autostart launchers (Scheduled Task + HKCU Run-key)",
			Detail:   "Both are registered — two daemons start at login and collide on the management port. Keep only the supervised Scheduled Task.",
			Fixable:  true,
		})
	case runKeyExists:
		out = append(out, autostartFinding{
			Severity: sevWarn,
			Title:    "Unsupervised autostart (HKCU Run-key fallback)",
			Detail:   "Starts at login only, with no crash-restart. Reinstall the supervised Scheduled Task.",
			Fixable:  true,
		})
	case taskExists:
		out = append(out, autostartFinding{Severity: sevOK, Title: "Single supervised autostart (Scheduled Task)"})
	default:
		out = append(out, autostartFinding{
			Severity: sevWarn,
			Title:    "No autostart installed",
			Detail:   "Run `idapt-computer service up` to install it.",
		})
	}
	if serveProcs > 1 {
		out = append(out, autostartFinding{
			Severity: sevError,
			Title:    fmt.Sprintf("%d daemon processes running", serveProcs),
			Detail:   "Multiple `idapt-computer serve` processes duel for the management port; all but one fail to bind. Collapse to a single instance.",
			Fixable:  true,
		})
	}
	return out
}

func linuxAutostartFindings(userUnitExists, systemUnitExists bool) []autostartFinding {
	switch {
	case userUnitExists && systemUnitExists:
		return []autostartFinding{{
			Severity: sevError,
			Title:    "Duplicate autostart scopes (systemd --user unit AND system unit)",
			Detail:   "Both a per-user and a system-wide idapt-computer.service are installed — two daemons start and collide on the management port. Keep exactly one scope.",
			Fixable:  true,
		}}
	case userUnitExists || systemUnitExists:
		return []autostartFinding{{Severity: sevOK, Title: "Single systemd autostart unit"}}
	default:
		return []autostartFinding{{
			Severity: sevWarn,
			Title:    "No autostart unit installed",
			Detail:   "Run `idapt-computer service up` to install it.",
		}}
	}
}

func darwinAutostartFindings(agentExists bool) []autostartFinding {
	if agentExists {
		return []autostartFinding{{Severity: sevOK, Title: "Single launchd LaunchAgent autostart"}}
	}
	return []autostartFinding{{
		Severity: sevWarn,
		Title:    "No LaunchAgent installed",
		Detail:   "Run `idapt-computer service up` to install it.",
	}}
}

var serviceDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose (and --fix) autostart problems (duplicate launchers, unsupervised fallback, scope conflicts)",
	Long: `Inspect the daemon's autostart wiring and report problems that make it
flap or fail to start after a reboot:

  • duplicate launchers — a Windows Scheduled Task AND an HKCU Run-key, or a
    Linux systemd --user unit AND a system unit — that start two daemons which
    collide on the management port
  • an unsupervised fallback (the Windows Run-key) that never restarts on crash
  • more than one daemon process running at once

Pass --fix to collapse the autostart back to a single supervised mechanism.`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		fix, _ := cmd.Flags().GetBool("fix")
		return runServiceDoctor(cmd, fix)
	},
}

func runServiceDoctor(cmd *cobra.Command, fix bool) error {
	out := cmd.OutOrStdout()
	findings := collectAutostartFindings()
	printAutostartFindings(out, findings)

	if fix && findingsHaveFixable(findings) {
		fmt.Fprintln(out, "\nApplying fixes...")
		if err := repairAutostart(cmd); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "WARN: repair step reported: %v\n", err)
		}
		fmt.Fprintln(out, "\nRe-checking...")
		findings = collectAutostartFindings()
		printAutostartFindings(out, findings)
	} else if findingsHaveFixable(findings) {
		fmt.Fprintln(out, "\nRun `idapt-computer service doctor --fix` to repair the fixable items above.")
	}

	if findingsHaveError(findings) {
		return fmt.Errorf("autostart has unresolved problems (see above)")
	}
	if findingsAllOK(findings) {
		fmt.Fprintln(out, "\nAutostart looks healthy.")
	}
	return nil
}
