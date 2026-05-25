//go:build linux

package commands

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func configureProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		pgid, err := syscall.Getpgid(cmd.Process.Pid)
		if err == nil {
			err = syscall.Kill(-pgid, syscall.SIGKILL)
			if err == nil || errors.Is(err, syscall.ESRCH) {
				return nil
			}
		}
		return cmd.Process.Kill()
	}
}
