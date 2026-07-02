//go:build windows

package update

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/windows"
)

func replaceBinary(tmpPath, binaryPath string, sameDir bool) error {
	src := tmpPath
	if !sameDir {
		staged := binaryPath + ".new"
		if err := copyFile(tmpPath, staged); err != nil {
			return fmt.Errorf("stage new binary next to destination: %w", err)
		}
		_ = os.Remove(tmpPath)
		src = staged
	}

	old := binaryPath + ".old"
	_ = os.Remove(old) // clear a stale .old left by a previous update

	if err := os.Rename(binaryPath, old); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("rename current binary aside: %w", err)
	}

	if err := os.Rename(src, binaryPath); err != nil {
		_ = os.Rename(old, binaryPath)
		return fmt.Errorf("move new binary into place: %w", err)
	}

	if err := os.Remove(old); err != nil {
		scheduleDeleteOnReboot(old)
	}
	return nil
}

func scheduleDeleteOnReboot(path string) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	_ = windows.MoveFileEx(p, nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}
