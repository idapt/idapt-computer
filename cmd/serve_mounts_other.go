//go:build !linux && !darwin

package cmd

import (
	"context"
	"log"

	"github.com/idapt/idapt-cli/internal/config"
)

type noopMountSupervisor struct{}

func newMountSupervisor() mountSupervisor { return noopMountSupervisor{} }

func (noopMountSupervisor) ActiveMountCount() int { return 0 }

func (noopMountSupervisor) AutoMount(_ context.Context, cfg *config.Config) {
	if len(cfg.Mounts) > 0 {
		log.Printf("fuse-mount: %d configured mount(s) skipped — Drive FUSE mounts are supported on Linux and macOS only", len(cfg.Mounts))
		return
	}
	log.Printf("fuse-mount: not supported on this platform")
}

func (noopMountSupervisor) Shutdown(_ context.Context) {}
