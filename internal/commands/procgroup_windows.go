//go:build windows

package commands

import "os/exec"

func setProcessGroup(_ *exec.Cmd) {}

func killProcessGroup(_ int) {}
