package tunnelclient

import (
	"context"
	"fmt"
	"sync"
)

type Manager struct {
	cm     *ConfigManager
	syncer *Syncer

	mu    sync.RWMutex
	cache []SyncedTunnel
}

func NewManager(cm *ConfigManager, syncer *Syncer) *Manager {
	return &Manager{cm: cm, syncer: syncer}
}

func (m *Manager) Config() *ConfigManager { return m.cm }

func (m *Manager) Expose(ctx context.Context, port int, authMode string) (SyncedTunnel, error) {
	cfg, err := m.cm.AddPort(ExposedPort{Port: port, AuthMode: authMode})
	if err != nil {
		return SyncedTunnel{}, err
	}
	tunnels, err := m.sync(ctx, cfg)
	if err != nil {
		return SyncedTunnel{}, err
	}
	for _, t := range tunnels {
		if t.Port == port {
			return t, nil
		}
	}
	return SyncedTunnel{}, fmt.Errorf("backend did not return a tunnel for port %d", port)
}

func (m *Manager) Unexpose(ctx context.Context, port int) error {
	cfg := m.cm.RemovePort(port)
	_, err := m.sync(ctx, cfg)
	return err
}

func (m *Manager) Sync(ctx context.Context) ([]SyncedTunnel, error) {
	return m.sync(ctx, m.cm.GetConfig())
}

func (m *Manager) sync(ctx context.Context, cfg Config) ([]SyncedTunnel, error) {
	tunnels, err := m.syncer.Push(ctx, cfg)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.cache = tunnels
	m.mu.Unlock()
	return tunnels, nil
}

func (m *Manager) Cached() []SyncedTunnel {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]SyncedTunnel, len(m.cache))
	copy(out, m.cache)
	return out
}
