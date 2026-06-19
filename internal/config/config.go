package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	ComputerID string `json:"computerId"`

	ComputerResourceID string `json:"computerResourceId,omitempty"`

	AppURL string `json:"appUrl"`

	Domain string `json:"domain"`

	ComputerToken string `json:"computerToken"`

	DefaultBackendPort int `json:"defaultBackendPort"`

	CLIBinaryURL string `json:"cliBinaryUrl"`

	TunnelProxyURL string `json:"tunnelProxyUrl"`

	Mounts []MountEntry `json:"mounts,omitempty"`
}

type MountEntry struct {
	WorkspaceID       string   `json:"workspaceId"`
	MountPoint      string   `json:"mountPoint"`
	CacheDir        string   `json:"cacheDir,omitempty"`
	MaxCacheSizeGB  int      `json:"maxCacheSizeGB,omitempty"` // default 10
	ExcludePatterns []string `json:"excludePatterns,omitempty"`
}

func Load(path string) (*Config, error) {
	var cfg Config
	cfgPresent := false

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read config file %s: %w", path, err)
		}
	} else {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parse config file %s: %w", path, err)
		}
		cfgPresent = true
	}

	if v := os.Getenv("IDAPT_COMPUTER_ID"); v != "" {
		cfg.ComputerID = v
	}
	if v := os.Getenv("IDAPT_APP_URL"); v != "" {
		cfg.AppURL = v
	}
	if v := os.Getenv("IDAPT_DOMAIN"); v != "" {
		cfg.Domain = v
	}
	if v := os.Getenv("IDAPT_COMPUTER_TOKEN"); v != "" {
		cfg.ComputerToken = v
	}
	if v := os.Getenv("IDAPT_TUNNEL_PROXY_URL"); v != "" {
		cfg.TunnelProxyURL = v
	}

	if cfg.DefaultBackendPort == 0 {
		cfg.DefaultBackendPort = 80
	}
	if cfg.AppURL == "" {
		cfg.AppURL = "https://idapt.app"
	}

	if cfgPresent {
		if cfg.ComputerID == "" {
			return nil, fmt.Errorf("computerId is required")
		}
		if cfg.Domain == "" {
			return nil, fmt.Errorf("domain is required")
		}
		if strings.Contains(cfg.Domain, "*") {
			return nil, fmt.Errorf("domain must be a specific subdomain, not a wildcard: %s", cfg.Domain)
		}
	}

	return &cfg, nil
}

func (c *Config) IsLocalMode() bool {
	return c.ComputerID == ""
}
