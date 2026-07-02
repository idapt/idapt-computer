//go:build !windows

package commands

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func ptyConfigureCancel(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		pgid, err := syscall.Getpgid(cmd.Process.Pid)
		if err == nil {
			if kerr := syscall.Kill(-pgid, syscall.SIGKILL); kerr == nil || errors.Is(kerr, syscall.ESRCH) {
				return nil
			}
		}
		return cmd.Process.Kill()
	}
}
