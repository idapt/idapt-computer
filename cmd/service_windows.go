//go:build windows

package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)
const windowsTaskName = "idapt-computer"
const windowsRunKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`

func windowsLogPath() (string, error) {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("LOCALAPPDATA + HOME both unset: %w", err)
		}
		localAppData = filepath.Join(home, "AppData", "Local")
	}
	return filepath.Join(localAppData, "idapt", "Logs", "daemon.log"), nil
}

func serviceUp(cmd *cobra.Command, reinstall bool) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate idapt binary: %w", err)
	}
	logPath, err := windowsLogPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	taskCmd := windowsDaemonCommand(exePath, logPath)

	args := []string{
		"/Create",
		"/SC", "ONLOGON",
		"/TN", windowsTaskName,
		"/TR", taskCmd,
		"/F",
	}
	if err := runSchtasks(args...); err != nil {
		if !isWindowsAutostartPolicyError(err) {
			return fmt.Errorf("schtasks create: %w", err)
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "WARN: Task Scheduler autostart failed: %v\n", err)
		fmt.Fprintln(cmd.ErrOrStderr(), "Falling back to the per-user Windows Run key.")
		if err := installWindowsRunKey(taskCmd); err != nil {
			return fmt.Errorf("install HKCU Run fallback: %w", err)
		}
		if err := startWindowsDaemonDetached(exePath, logPath); err != nil {
			return fmt.Errorf("start daemon: %w", err)
		}
		fmt.Fprintf(
			cmd.OutOrStdout(),
			"Autostart registry entry installed at user scope (HKCU Run\\%s). Logs: %s\n",
			windowsTaskName, logPath,
		)
		if reinstall {
			fmt.Fprintln(cmd.OutOrStdout(), "(reinstalled to point at the current binary)")
		}
		return nil
	}
	if err := runSchtasks("/Run", "/TN", windowsTaskName); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "WARN: schtasks /Run: %v\n", err)
	}
	fmt.Fprintf(
		cmd.OutOrStdout(),
		"Autostart task installed at user scope (TN=%s). Logs: %s\n",
		windowsTaskName, logPath,
	)
	if reinstall {
		fmt.Fprintln(cmd.OutOrStdout(), "(reinstalled to point at the current binary)")
	}
	return nil
}

func serviceDown(cmd *cobra.Command) error {
	if err := runSchtasks("/End", "/TN", windowsTaskName); err != nil {
		lower := strings.ToLower(err.Error())
		if !strings.Contains(lower, "not currently running") &&
			!strings.Contains(lower, "no running instance") &&
			!strings.Contains(lower, "task does not exist") &&
			!strings.Contains(lower, "the system cannot find") {
			return fmt.Errorf("schtasks end: %w", err)
		}
	}
	_ = stopWindowsServeProcesses()
	fmt.Fprintln(cmd.OutOrStdout(), "idapt daemon stopped.")
	return nil
}

func serviceRestart(cmd *cobra.Command) error {
	_ = runSchtasks("/End", "/TN", windowsTaskName)
	if err := runSchtasks("/Run", "/TN", windowsTaskName); err != nil {
		if !windowsRunKeyInstalled() {
			return fmt.Errorf("schtasks run: %w", err)
		}
		exePath, exeErr := os.Executable()
		if exeErr != nil {
			return fmt.Errorf("locate idapt binary: %w", exeErr)
		}
		logPath, logErr := windowsLogPath()
		if logErr != nil {
			return logErr
		}
		_ = stopWindowsServeProcesses()
		if startErr := startWindowsDaemonDetached(exePath, logPath); startErr != nil {
			return fmt.Errorf("start daemon: %w", startErr)
		}
	}
	fmt.Fprintln(cmd.OutOrStdout(), "idapt daemon restarted.")
	return nil
}

func serviceStatus(cmd *cobra.Command) error {
	c := exec.Command("schtasks", "/Query", "/TN", windowsTaskName, "/FO", "LIST", "/V")
	c.Stdout = cmd.OutOrStdout()
	c.Stderr = cmd.ErrOrStderr()
	if err := c.Run(); err != nil {
		fmt.Fprintln(cmd.OutOrStdout(), "Task Scheduler autostart: not installed")
	}
	if out, err := queryWindowsRunKey(); err == nil {
		fmt.Fprintln(cmd.OutOrStdout(), "")
		fmt.Fprintln(cmd.OutOrStdout(), "HKCU Run autostart:")
		fmt.Fprint(cmd.OutOrStdout(), out)
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "")
		fmt.Fprintln(cmd.OutOrStdout(), "HKCU Run autostart: not installed")
	}
	if out, err := queryWindowsServeProcesses(); err == nil && strings.TrimSpace(out) != "" {
		fmt.Fprintln(cmd.OutOrStdout(), "")
		fmt.Fprintln(cmd.OutOrStdout(), "Running daemon processes:")
		fmt.Fprint(cmd.OutOrStdout(), out)
	}
	return nil
}

func serviceLogs(cmd *cobra.Command, follow bool, lines int, since string) error {
	if since != "" {
		fmt.Fprintln(cmd.ErrOrStderr(),
			"NOTE: --since is ignored on Windows (file-based log sink). Use --lines to bound the tail length.",
		)
	}
	logPath, err := windowsLogPath()
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(logPath); os.IsNotExist(statErr) {
		fmt.Fprintf(cmd.OutOrStdout(),
			"No log file yet at %s (daemon may not have written anything).\n",
			logPath,
		)
		return nil
	}
	if err := tailFile(logPath, lines, cmd.OutOrStdout()); err != nil {
		return err
	}
	if !follow {
		return nil
	}
	return followFile(logPath, cmd.OutOrStdout())
}

func serviceUninstall(cmd *cobra.Command) error {
	_ = runSchtasks("/End", "/TN", windowsTaskName)
	_ = stopWindowsServeProcesses()
	if err := runSchtasks("/Delete", "/TN", windowsTaskName, "/F"); err != nil {
		lower := strings.ToLower(err.Error())
		if !strings.Contains(lower, "task does not exist") &&
			!strings.Contains(lower, "the system cannot find") {
			return fmt.Errorf("schtasks delete: %w", err)
		}
	}
	if err := uninstallWindowsRunKey(); err != nil {
		lower := strings.ToLower(err.Error())
		if !strings.Contains(lower, "unable to find") &&
			!strings.Contains(lower, "the system was unable to find") {
			return fmt.Errorf("delete HKCU Run fallback: %w", err)
		}
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Autostart task removed.")
	return nil
}

func windowsDaemonCommand(exePath, logPath string) string {
	return fmt.Sprintf(`cmd /c ""%s" serve >> "%s" 2>&1"`, exePath, logPath)
}

func isWindowsAutostartPolicyError(err error) bool {
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "access is denied") ||
		strings.Contains(lower, "disabled by your administrator") ||
		strings.Contains(lower, "policy")
}

func installWindowsRunKey(command string) error {
	return runReg("add", windowsRunKey, "/v", windowsTaskName, "/t", "REG_SZ", "/d", command, "/f")
}

func uninstallWindowsRunKey() error {
	return runReg("delete", windowsRunKey, "/v", windowsTaskName, "/f")
}

func windowsRunKeyInstalled() bool {
	_, err := queryWindowsRunKey()
	return err == nil
}

func queryWindowsRunKey() (string, error) {
	return runRegOutput("query", windowsRunKey, "/v", windowsTaskName)
}

func startWindowsDaemonDetached(exePath, logPath string) error {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log: %w", err)
	}
	defer logFile.Close()

	c := exec.Command(exePath, "serve")
	c.Stdin = nil
	c.Stdout = logFile
	c.Stderr = logFile
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return c.Start()
}

func queryWindowsServeProcesses() (string, error) {
	script := `Get-CimInstance Win32_Process -Filter "name = 'idapt-computer.exe'" | Where-Object { $_.CommandLine -match '(^| )serve( |$)' } | Select-Object ProcessId,CommandLine | Format-List`
	return runPowerShellOutput(script)
}

func stopWindowsServeProcesses() error {
	script := `$ErrorActionPreference = 'SilentlyContinue'; Get-CimInstance Win32_Process -Filter "name = 'idapt-computer.exe'" | Where-Object { $_.CommandLine -match '(^| )serve( |$)' } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force }`
	_, err := runPowerShellOutput(script)
	return err
}

func runSchtasks(args ...string) error {
	c := exec.Command("schtasks", args...)
	out, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf(
			"schtasks %s: %w (%s)",
			strings.Join(args, " "), err, strings.TrimSpace(string(out)),
		)
	}
	return nil
}

func runReg(args ...string) error {
	_, err := runRegOutput(args...)
	return err
}

func runRegOutput(args ...string) (string, error) {
	c := exec.Command("reg", args...)
	out, err := c.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf(
			"reg %s: %w (%s)",
			strings.Join(args, " "), err, strings.TrimSpace(string(out)),
		)
	}
	return string(out), nil
}

func runPowerShellOutput(script string) (string, error) {
	c := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	out, err := c.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("powershell: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func tailFile(path string, n int, w io.Writer) error {
	if n <= 0 {
		n = 100
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	buf := make([]string, 0, n)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		buf = append(buf, scanner.Text())
		if len(buf) > n {
			buf = buf[1:]
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	for _, line := range buf {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}

func followFile(path string, w io.Writer) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	for {
		time.Sleep(500 * time.Millisecond)
		if _, err := io.Copy(w, f); err != nil {
			return err
		}
	}
}
