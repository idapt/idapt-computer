//go:build !windows

package cmd

import "os"

func hideDaemonConsole(_ *os.File) {}
