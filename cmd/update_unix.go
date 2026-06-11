//go:build !windows

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func signalDaemonRestart(version string) string {
	pid := findServePID()
	if pid <= 0 {
		return fmt.Sprintf("Updated to %s. No running daemon found — restart manually to apply.", version)
	}
	if err := syscall.Kill(pid, syscall.SIGUSR1); err != nil {
		return fmt.Sprintf("Updated to %s. Failed to signal daemon (PID %d): %v — restart manually.", version, pid, err)
	}
	return fmt.Sprintf("Updated to %s. Daemon restarting seamlessly (PID %d).", version, pid)
}

func findServePID() int {
	if pid := findPIDViaSystemctl(); pid > 0 {
		return pid
	}
	if pid := findPIDViaProcScan(); pid > 0 {
		return pid
	}
	if pid := findPIDViaPgrep(); pid > 0 {
		return pid
	}
	return 0
}

func findPIDViaSystemctl() int {
	out, err := exec.Command("systemctl", "show", "idapt.service", "-p", "MainPID", "--value").Output()
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil || pid <= 0 {
		return 0
	}
	return pid
}

func findPIDViaProcScan() int {
	myPID := os.Getpid()
	entries, err := filepath.Glob("/proc/[0-9]*/cmdline")
	if err != nil {
		return 0
	}
	for _, entry := range entries {
		data, err := os.ReadFile(entry)
		if err != nil {
			continue
		}
		cmdline := string(data)
		if !strings.Contains(cmdline, "idapt") || !strings.Contains(cmdline, "serve") {
			continue
		}
		parts := strings.Split(entry, "/")
		if len(parts) < 3 {
			continue
		}
		pid, err := strconv.Atoi(parts[2])
		if err != nil || pid <= 0 || pid == myPID {
			continue
		}
		return pid
	}
	return 0
}

func findPIDViaPgrep() int {
	myPID := os.Getpid()
	out, err := exec.Command("pgrep", "-f", "idapt serve").Output()
	if err != nil {
		return 0
	}
	for _, line := range strings.Fields(string(out)) {
		pid, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil || pid <= 0 || pid == myPID {
			continue
		}
		return pid
	}
	return 0
}
