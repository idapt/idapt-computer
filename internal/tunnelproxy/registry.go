package tunnelproxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const registryKeyPrefix = "tunnel:host:"

const maxCacheEntries = 10000

type TunnelEntry struct {
	MachineID    string `json:"machineId"`
	LocalPort    int    `json:"localPort"`
	AuthMode     string `json:"authMode"`
	OwnerActorID string `json:"ownerActorId"`
	ProjectID    string `json:"projectId,omitempty"`
}

var ErrNoTunnel = errors.New("tunnelproxy: no such tunnel")

type Registry struct {
	rdb      *redis.Client
	cacheTTL time.Duration
	mu       sync.Mutex
	cache    map[string]cachedEntry
}

type cachedEntry struct {
	entry   *TunnelEntry
	missing bool
	expires time.Time
}

func NewRegistry(redisURL string) (*Registry, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	return &Registry{
		rdb:      redis.NewClient(opt),
		cacheTTL: 5 * time.Second,
		cache:    make(map[string]cachedEntry),
	}, nil
}

func (r *Registry) Ping(ctx context.Context) error {
	return r.rdb.Ping(ctx).Err()
}

func (r *Registry) Close() error {
	return r.rdb.Close()
}

func (r *Registry) Lookup(ctx context.Context, host string) (*TunnelEntry, error) {
	host = strings.ToLower(host)

	r.mu.Lock()
	if c, ok := r.cache[host]; ok && time.Now().Before(c.expires) {
		r.mu.Unlock()
		if c.missing {
			return nil, ErrNoTunnel
		}
		return c.entry, nil
	}
	r.mu.Unlock()

	val, err := r.rdb.Get(ctx, registryKeyPrefix+host).Result()
	if errors.Is(err, redis.Nil) {
		r.store(host, cachedEntry{missing: true, expires: time.Now().Add(r.cacheTTL)})
		return nil, ErrNoTunnel
	}
	if err != nil {
		return nil, fmt.Errorf("registry lookup: %w", err)
	}
	var entry TunnelEntry
	if err := json.Unmarshal([]byte(val), &entry); err != nil {
		return nil, fmt.Errorf("registry decode for %q: %w", host, err)
	}
	r.store(host, cachedEntry{entry: &entry, expires: time.Now().Add(r.cacheTTL)})
	return &entry, nil
}

func (r *Registry) store(host string, c cachedEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.cache) >= maxCacheEntries {
		r.cache = make(map[string]cachedEntry)
	}
	r.cache[host] = c
}
