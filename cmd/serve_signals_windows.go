//go:build windows

package cmd

import (
	"fmt"
	"os"
	"syscall"
)

func daemonSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}

func isReloadSignal(_ os.Signal) bool  { return false }
func isRestartSignal(_ os.Signal) bool { return false }

func testRestartSignal() os.Signal { return os.Interrupt }

func reexecDaemon() error {
	return fmt.Errorf("in-place restart is not supported on Windows; restart the idapt-computer service to apply the update")
}
