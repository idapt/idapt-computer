//go:build !windows

package update

import (
	"fmt"
	"io"
	"os"
	"os/exec"
)

func replaceBinary(tmpPath, binaryPath string, sameDir bool) error {
	if sameDir {
		return os.Rename(tmpPath, binaryPath)
	}
	if os.Geteuid() == 0 {
		return moveCrossFS(tmpPath, binaryPath)
	}
	sudo, err := exec.LookPath("sudo")
	if err != nil {
		return fmt.Errorf("destination %s requires elevated permissions but sudo is not available; re-run as root", binaryPath)
	}
	cmd := exec.Command(sudo, "install", "-m", "0755", tmpPath, binaryPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sudo install %s: %w", binaryPath, err)
	}
	return nil
}

func moveCrossFS(src, dst string) error {
	staged := dst + ".new"
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(staged, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(staged)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(staged)
		return err
	}
	if err := os.Rename(staged, dst); err != nil {
		os.Remove(staged)
		return err
	}
	_ = os.Remove(src)
	return nil
}
