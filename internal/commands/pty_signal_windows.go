//go:build windows

package commands

import "os/exec"

func ptyConfigureCancel(_ *exec.Cmd) {}
