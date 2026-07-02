package update

import (
	"fmt"
	"io"
	"os"
)

func lkgPath(binaryPath string) string { return binaryPath + ".last-known-good" }

func SaveLastKnownGood(binaryPath string) error {
	return copyExecutable(binaryPath, lkgPath(binaryPath))
}

func HasLastKnownGood(binaryPath string) bool {
	_, err := os.Stat(lkgPath(binaryPath))
	return err == nil
}

func RestoreLastKnownGood(binaryPath string) error {
	lkg := lkgPath(binaryPath)
	if _, err := os.Stat(lkg); err != nil {
		return fmt.Errorf("no last-known-good binary at %s: %w", lkg, err)
	}
	staged := binaryPath + ".restore"
	if err := copyExecutable(lkg, staged); err != nil {
		return err
	}
	if err := replaceBinary(staged, binaryPath, true); err != nil {
		_ = os.Remove(staged)
		return err
	}
	return nil
}

func copyExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
