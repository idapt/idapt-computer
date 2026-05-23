package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	MachineID string `json:"machineId"`

	MachineResourceID string `json:"machineResourceId,omitempty"`

	AppURL string `json:"appUrl"`

	Domain string `json:"domain"`

	MachineToken string `json:"machineToken"`

	DefaultBackendPort int `json:"defaultBackendPort"`

	CLIBinaryURL string `json:"cliBinaryUrl"`

	TunnelProxyURL string `json:"tunnelProxyUrl"`

	Mounts []MountEntry `json:"mounts,omitempty"`
}

type MountEntry struct {
	ProjectID       string   `json:"projectId"`
	MountPoint      string   `json:"mountPoint"`
	CacheDir        string   `json:"cacheDir,omitempty"`
	MaxCacheSizeGB  int      `json:"maxCacheSizeGB,omitempty"` // default 10
	ExcludePatterns []string `json:"excludePatterns,omitempty"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config file %s: %w", path, err)
	}

	if v := os.Getenv("IDAPT_MACHINE_ID"); v != "" {
		cfg.MachineID = v
	}
	if v := os.Getenv("IDAPT_APP_URL"); v != "" {
		cfg.AppURL = v
	}
	if v := os.Getenv("IDAPT_DOMAIN"); v != "" {
		cfg.Domain = v
	}
	if v := os.Getenv("IDAPT_MACHINE_TOKEN"); v != "" {
		cfg.MachineToken = v
	}
	if v := os.Getenv("IDAPT_TUNNEL_PROXY_URL"); v != "" {
		cfg.TunnelProxyURL = v
	}

	if cfg.DefaultBackendPort == 0 {
		cfg.DefaultBackendPort = 80
	}

	if cfg.MachineID == "" {
		return nil, fmt.Errorf("machineId is required")
	}
	if cfg.AppURL == "" {
		return nil, fmt.Errorf("appUrl is required")
	}
	if cfg.Domain == "" {
		return nil, fmt.Errorf("domain is required")
	}
	if strings.Contains(cfg.Domain, "*") {
		return nil, fmt.Errorf("domain must be a specific subdomain, not a wildcard: %s", cfg.Domain)
	}

	return &cfg, nil
}
