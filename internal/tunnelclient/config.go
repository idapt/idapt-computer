package tunnelclient

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
)

const DefaultConfigPath = "/etc/idapt/tunnels.json"

const (
	AuthModePrivate = "private" // only the computer owner
	AuthModeWorkspace = "workspace" // any member of the computer's workspace
	AuthModeIdapt   = "idapt"   // any signed-in idapt user
)

type ExposedPort struct {
	Port     int    `json:"port"`
	AuthMode string `json:"authMode"`
}

type Config struct {
	Ports []ExposedPort `json:"ports"`
}

type ConfigManager struct {
	mu       sync.RWMutex
	config   Config
	path     string
	onChange func()
}

func NewConfigManager(path string) *ConfigManager {
	cm := &ConfigManager{path: path, config: Config{Ports: []ExposedPort{}}}
	if data, err := os.ReadFile(path); err == nil {
		var cfg Config
		if err := json.Unmarshal(data, &cfg); err != nil {
			log.Printf("WARN: corrupt tunnels config at %s, starting empty: %v", path, err)
		} else if err := validate(&cfg); err != nil {
			log.Printf("WARN: invalid tunnels config at %s, starting empty: %v", path, err)
		} else {
			cm.config = cfg
		}
	}
	return cm
}

func (cm *ConfigManager) SetOnChange(fn func()) {
	cm.mu.Lock()
	cm.onChange = fn
	cm.mu.Unlock()
}

func (cm *ConfigManager) GetConfig() Config {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	out := Config{Ports: make([]ExposedPort, len(cm.config.Ports))}
	copy(out.Ports, cm.config.Ports)
	return out
}

func (cm *ConfigManager) SetConfig(cfg Config) error {
	if err := validate(&cfg); err != nil {
		return err
	}
	cm.mu.Lock()
	cm.config = cfg
	cb := cm.onChange
	cm.mu.Unlock()

	if err := save(cm.path, &cfg); err != nil {
		log.Printf("WARN: failed to persist tunnels config: %v", err)
	}
	if cb != nil {
		cb()
	}
	return nil
}

func (cm *ConfigManager) AddPort(p ExposedPort) (Config, error) {
	cm.mu.RLock()
	next := Config{Ports: make([]ExposedPort, 0, len(cm.config.Ports)+1)}
	for _, existing := range cm.config.Ports {
		if existing.Port != p.Port {
			next.Ports = append(next.Ports, existing)
		}
	}
	cm.mu.RUnlock()
	next.Ports = append(next.Ports, p)
	if err := cm.SetConfig(next); err != nil {
		return Config{}, err
	}
	return cm.GetConfig(), nil
}

func (cm *ConfigManager) RemovePort(port int) Config {
	cm.mu.RLock()
	next := Config{Ports: make([]ExposedPort, 0, len(cm.config.Ports))}
	for _, existing := range cm.config.Ports {
		if existing.Port != port {
			next.Ports = append(next.Ports, existing)
		}
	}
	cm.mu.RUnlock()
	_ = cm.SetConfig(next) // next is a subset of an already-valid config
	return cm.GetConfig()
}

func (cm *ConfigManager) ReloadFromDisk() error {
	data, err := os.ReadFile(cm.path)
	if err != nil {
		if os.IsNotExist(err) {
			return cm.SetConfig(Config{Ports: []ExposedPort{}})
		}
		return fmt.Errorf("read tunnels config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse tunnels config: %w", err)
	}
	return cm.SetConfig(cfg)
}

func (cm *ConfigManager) IsExposed(port int) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	for _, p := range cm.config.Ports {
		if p.Port == port {
			return true
		}
	}
	return false
}

func (cm *ConfigManager) Count() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return len(cm.config.Ports)
}

func validate(cfg *Config) error {
	seen := make(map[int]bool)
	for i, p := range cfg.Ports {
		if p.Port < 1 || p.Port > 65535 {
			return fmt.Errorf("port %d at index %d: must be 1-65535", p.Port, i)
		}
		switch p.AuthMode {
		case AuthModePrivate, AuthModeWorkspace, AuthModeIdapt:
		default:
			return fmt.Errorf("port %d: authMode must be private, workspace, or idapt", p.Port)
		}
		if seen[p.Port] {
			return fmt.Errorf("duplicate port %d", p.Port)
		}
		seen[p.Port] = true
	}
	return nil
}

func save(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
