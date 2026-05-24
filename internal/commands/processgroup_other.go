//go:build !linux

package commands

import "os/exec"

func configureProcessGroup(cmd *exec.Cmd) {}
