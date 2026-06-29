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

	CommandPolicy CommandPolicy `json:"commandPolicy,omitempty"`
}

type CommandPolicy struct {
	RemoteShell    bool `json:"remoteShell,omitempty"`
	RemoteFiles    bool `json:"remoteFiles,omitempty"`
	AdminOps       bool `json:"adminOps,omitempty"`
	LocalInference bool `json:"localInference,omitempty"`
	ComputerApps   bool `json:"computerApps,omitempty"`
	ComputerUse    bool `json:"computerUse,omitempty"`
	Tunnels        bool `json:"tunnels,omitempty"`
}

type MountEntry struct {
	WorkspaceID     string   `json:"workspaceId"`
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
	applyCommandPolicyEnv(&cfg.CommandPolicy)

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

func applyCommandPolicyEnv(policy *CommandPolicy) {
	if envBool("IDAPT_ALLOW_RISKY_COMPUTER_COMMANDS") {
		*policy = CommandPolicy{
			RemoteShell:    true,
			RemoteFiles:    true,
			AdminOps:       true,
			LocalInference: true,
			ComputerApps:   true,
			ComputerUse:    true,
			Tunnels:        true,
		}
	}
	if envBool("IDAPT_ALLOW_REMOTE_SHELL") {
		policy.RemoteShell = true
	}
	if envBool("IDAPT_ALLOW_REMOTE_FILES") {
		policy.RemoteFiles = true
	}
	if envBool("IDAPT_ALLOW_ADMIN_OPS") {
		policy.AdminOps = true
	}
	if envBool("IDAPT_ALLOW_LOCAL_INFERENCE") {
		policy.LocalInference = true
	}
	if envBool("IDAPT_ALLOW_COMPUTER_APPS") {
		policy.ComputerApps = true
	}
	if envBool("IDAPT_ALLOW_COMPUTER_USE") {
		policy.ComputerUse = true
	}
	if envBool("IDAPT_ALLOW_TUNNELS") {
		policy.Tunnels = true
	}
}

func envBool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (c *Config) IsLocalMode() bool {
	return c.ComputerID == ""
}
