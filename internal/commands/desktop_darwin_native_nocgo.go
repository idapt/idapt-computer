//go:build darwin && !cgo

package commands

func darwinNativeBackend(_ map[string]string) DesktopBackend { return nil }
