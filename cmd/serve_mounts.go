//go:build linux || darwin

package cmd

import (
	"context"
	"log"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/config"
	ifuse "github.com/idapt/idapt-cli/internal/fuse"
)

type fuseMountSupervisor struct {
	mm *ifuse.MountManager
}

func newMountSupervisor() mountSupervisor {
	return &fuseMountSupervisor{mm: ifuse.NewMountManager()}
}

func (s *fuseMountSupervisor) ActiveMountCount() int {
	if s.mm == nil {
		return 0
	}
	return len(s.mm.ActiveMounts())
}

func (s *fuseMountSupervisor) AutoMount(ctx context.Context, cfg *config.Config) {
	if len(cfg.Mounts) == 0 {
		log.Printf("fuse-mount: no mounts configured")
		return
	}
	apiClient, err := buildFuseAPIClient(cfg)
	if err != nil {
		log.Printf("fuse-mount: disabled (API client error: %v)", err)
		return
	}
	for _, m := range cfg.Mounts {
		maxCache := int64(m.MaxCacheSizeGB) * 1024 * 1024 * 1024
		if maxCache == 0 {
			maxCache = 10 * 1024 * 1024 * 1024 // default 10GB
		}
		mountCfg := ifuse.MountConfig{
			WorkspaceID:     m.WorkspaceID,
			MountPoint:      m.MountPoint,
			CacheDir:        m.CacheDir,
			MaxCacheSize:    maxCache,
			ExcludePatterns: m.ExcludePatterns,
		}
		if err := s.mm.Mount(ctx, mountCfg, apiClient); err != nil {
			log.Printf("fuse-mount: failed to mount %s at %s: %v", m.WorkspaceID, m.MountPoint, err)
		}
	}
}

func (s *fuseMountSupervisor) Shutdown(ctx context.Context) {
	if s.mm != nil {
		s.mm.Shutdown(ctx)
	}
}

func buildFuseAPIClient(cfg *config.Config) (*ifuse.FuseAPIClient, error) {
	apiClient, err := api.NewClient(api.ClientConfig{
		BaseURL: cfg.AppURL,
		APIKey:  cfg.ComputerToken, // uses computer token for auth
	})
	if err != nil {
		return nil, err
	}
	return ifuse.NewFuseAPIClient(apiClient), nil
}
