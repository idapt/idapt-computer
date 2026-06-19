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
	"time"

	"github.com/spf13/cobra"
)
const windowsTaskName = "idapt-computer"

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

	taskCmd := fmt.Sprintf(`cmd /c ""%s" serve >> "%s" 2>&1"`, exePath, logPath)

	args := []string{
		"/Create",
		"/SC", "ONLOGON",
		"/TN", windowsTaskName,
		"/TR", taskCmd,
		"/F",
	}
	if err := runSchtasks(args...); err != nil {
		return fmt.Errorf("schtasks create: %w", err)
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
			!strings.Contains(lower, "no running instance") {
			return fmt.Errorf("schtasks end: %w", err)
		}
	}
	fmt.Fprintln(cmd.OutOrStdout(), "idapt daemon stopped.")
	return nil
}

func serviceRestart(cmd *cobra.Command) error {
	_ = runSchtasks("/End", "/TN", windowsTaskName)
	if err := runSchtasks("/Run", "/TN", windowsTaskName); err != nil {
		return fmt.Errorf("schtasks run: %w", err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "idapt daemon restarted.")
	return nil
}

func serviceStatus(cmd *cobra.Command) error {
	c := exec.Command("schtasks", "/Query", "/TN", windowsTaskName, "/FO", "LIST", "/V")
	c.Stdout = cmd.OutOrStdout()
	c.Stderr = cmd.ErrOrStderr()
	_ = c.Run()
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
	if err := runSchtasks("/Delete", "/TN", windowsTaskName, "/F"); err != nil {
		lower := strings.ToLower(err.Error())
		if !strings.Contains(lower, "task does not exist") &&
			!strings.Contains(lower, "the system cannot find") {
			return fmt.Errorf("schtasks delete: %w", err)
		}
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Autostart task removed.")
	return nil
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
