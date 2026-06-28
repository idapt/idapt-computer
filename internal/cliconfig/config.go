package cliconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/idapt/idapt-computer/internal/idaptpaths"
)

const CurrentSchemaVersion = 1

type Config struct {
	Version          int    `json:"version,omitempty"`
	APIURL           string `json:"apiUrl,omitempty"`
	DefaultWorkspace string `json:"defaultWorkspace,omitempty"`
	OutputFormat     string `json:"outputFormat,omitempty"`
	NoColor          bool   `json:"noColor,omitempty"`
}

func DefaultPath() (string, error) {
	dir, err := idaptpaths.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func Defaults() Config {
	return Config{
		Version: CurrentSchemaVersion,
		APIURL:  "https://idapt.app",
	}
}

func Load(path string) (Config, error) {
	cfg := Defaults()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return applyEnvOverrides(cfg), nil
		}
		return cfg, fmt.Errorf("reading config: %w", err)
	}
	if len(data) == 0 {
		return applyEnvOverrides(cfg), nil
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return applyEnvOverrides(Defaults()), nil
	}

	if cfg.Version > CurrentSchemaVersion {
		return applyEnvOverrides(Defaults()), nil
	}

	cfg.Version = CurrentSchemaVersion

	if cfg.APIURL == "" {
		cfg.APIURL = "https://idapt.app"
	}

	return applyEnvOverrides(cfg), nil
}

func applyEnvOverrides(cfg Config) Config {
	if v := os.Getenv("IDAPT_API_URL"); v != "" {
		cfg.APIURL = v
	}
	if v := os.Getenv("IDAPT_PROJECT"); v != "" {
		cfg.DefaultWorkspace = v
	}
	if v := os.Getenv("IDAPT_WORKSPACE"); v != "" {
		cfg.DefaultWorkspace = v
	}
	if v := os.Getenv("IDAPT_OUTPUT"); v != "" {
		cfg.OutputFormat = v
	}
	if os.Getenv("NO_COLOR") != "" {
		cfg.NoColor = true
	}
	return cfg
}

func Save(path string, cfg Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	if cfg.Version == 0 {
		cfg.Version = CurrentSchemaVersion
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	release, err := lockFileExclusive(path + ".lock")
	if err == nil {
		defer release()
	}

	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return os.WriteFile(path, data, 0600)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func (c *Config) Get(key string) (string, bool) {
	switch key {
	case "apiUrl":
		return c.APIURL, true
	case "defaultWorkspace":
		return c.DefaultWorkspace, true
	case "outputFormat":
		return c.OutputFormat, true
	case "noColor":
		return strconv.FormatBool(c.NoColor), true
	default:
		return "", false
	}
}

func (c *Config) Set(key, value string) error {
	switch key {
	case "apiUrl":
		c.APIURL = value
	case "defaultWorkspace":
		c.DefaultWorkspace = value
	case "outputFormat":
		c.OutputFormat = value
	case "noColor":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("noColor must be true or false, got %q", value)
		}
		c.NoColor = b
	default:
		return fmt.Errorf("unknown config key %q; valid keys: %v", key, Keys())
	}
	return nil
}

func (c *Config) Unset(key string) error {
	switch key {
	case "apiUrl":
		c.APIURL = ""
	case "defaultWorkspace":
		c.DefaultWorkspace = ""
	case "outputFormat":
		c.OutputFormat = ""
	case "noColor":
		c.NoColor = false
	default:
		return fmt.Errorf("unknown config key %q; valid keys: %v", key, Keys())
	}
	return nil
}

func Keys() []string {
	return []string{
		"apiUrl",
		"defaultWorkspace",
		"outputFormat",
		"noColor",
	}
}
