//go:build !windows

package cmd

import (
	"fmt"
	"os"
	"syscall"
)

func daemonSignals() []os.Signal {
	return []os.Signal{syscall.SIGTERM, syscall.SIGINT, syscall.SIGUSR1, syscall.SIGHUP}
}

func isReloadSignal(sig os.Signal) bool  { return sig == syscall.SIGHUP }
func isRestartSignal(sig os.Signal) bool { return sig == syscall.SIGUSR1 }

func testRestartSignal() os.Signal { return syscall.SIGUSR1 }

func reexecDaemon() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path for restart: %w", err)
	}
	return syscall.Exec(exe, os.Args, os.Environ())
}
