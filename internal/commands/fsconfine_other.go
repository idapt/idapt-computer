//go:build !linux

package commands

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var errConfinedEscape = errors.New("path escapes the allowed root")
func relUnderHome(homeDir, target string) (string, error) {
	clean := filepath.Clean(target)
	rel, err := filepath.Rel(homeDir, clean)
	if err != nil {
		return "", errConfinedEscape
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", errConfinedEscape
	}
	return rel, nil
}

func confineResolved(owner *runAsOwner, target string) (string, error) {
	resolved, err := resolvePathForPolicy(target)
	if err != nil {
		return "", err
	}
	if !pathInside(owner.HomeDir, resolved) {
		return "", errConfinedEscape
	}
	return resolved, nil
}

func openConfined(owner *runAsOwner, target string, flags int, mode os.FileMode) (*os.File, error) {
	if _, err := confineResolved(owner, target); err != nil {
		return nil, err
	}
	return os.OpenFile(target, flags|oNoFollow, mode)
}

func chownConfinedNoFollow(owner *runAsOwner, target string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	if _, err := confineResolved(owner, target); err != nil {
		return err
	}
	return os.Lchown(target, owner.UID, owner.GID)
}

func mkdirConfined(owner *runAsOwner, target string, mode os.FileMode) error {
	if _, err := confineResolved(owner, filepath.Dir(target)); err != nil {
		return err
	}
	return os.Mkdir(target, mode)
}

func mkdirAllConfined(owner *runAsOwner, target string, mode os.FileMode) error {
	if _, err := confineResolved(owner, target); err != nil {
		return err
	}
	return os.MkdirAll(target, mode)
}

func removeConfined(owner *runAsOwner, target string, recursive bool) error {
	if _, err := confineResolved(owner, target); err != nil {
		return err
	}
	if recursive {
		return os.RemoveAll(target)
	}
	return os.Remove(target)
}

func renameConfined(owner *runAsOwner, from, to string) error {
	if _, err := confineResolved(owner, from); err != nil {
		return err
	}
	if _, err := confineResolved(owner, to); err != nil {
		return err
	}
	return os.Rename(from, to)
}
