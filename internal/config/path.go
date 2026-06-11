package config

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/idapt/idapt-cli/internal/idaptpaths"
)

const LegacySystemConfigPath = "/etc/idapt/config.json"

func UserConfigPath() (string, error) {
	dir, err := idaptpaths.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func EnsureUserConfigPath() (string, error) {
	dir, err := idaptpaths.EnsureConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func ResolveConfigPath(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	user, err := UserConfigPath()
	if err != nil {
		return "", err
	}
	if _, statErr := os.Stat(user); statErr == nil {
		return user, nil
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return "", statErr
	}
	if _, statErr := os.Stat(LegacySystemConfigPath); statErr == nil {
		return LegacySystemConfigPath, nil
	}
	return user, nil
}
