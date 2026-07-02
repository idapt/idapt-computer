//go:build windows

package cmd

import (
	"os"

	"golang.org/x/sys/windows"
)

var procFreeConsole = windows.NewLazySystemDLL("kernel32.dll").NewProc("FreeConsole")

func hideDaemonConsole(f *os.File) {
	h := windows.Handle(f.Fd())
	_ = windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, h)
	_ = windows.SetStdHandle(windows.STD_ERROR_HANDLE, h)
	_, _, _ = procFreeConsole.Call()
}
