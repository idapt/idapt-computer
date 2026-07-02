package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

var daemonLogFilePath string

func resolveDaemonLogPath() string {
	if daemonLogFilePath != "" {
		return daemonLogFilePath
	}
	return os.Getenv("IDAPT_LOG_FILE")
}

func setupDaemonLogging(path string) (*os.File, error) {
	if path == "" {
		return nil, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create daemon log dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open daemon log %s: %w", path, err)
	}
	os.Stdout = f
	os.Stderr = f
	log.SetOutput(f)
	hideDaemonConsole(f)
	return f, nil
}
