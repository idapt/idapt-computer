//go:build darwin && !cgo

package commands

func darwinNativeBackend(_ RunuserConfig, _ string, _ map[string]string) DesktopBackend {
	return nil
}
