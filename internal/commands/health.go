package commands

import (
	"context"
	"encoding/json"
	"runtime"
	"sync/atomic"
	"time"
)

type HealthState struct {
	startTime       time.Time
	totalCommands   atomic.Int64
	totalErrors     atomic.Int64
	inflight        atomic.Int64
	queued          atomic.Int64
	capacityCmds    int
	cliVersion      string
	fuseMountsRefFn func() int
}

func NewHealthState(cliVersion string, capacity int, fuseMountsFn func() int) *HealthState {
	return &HealthState{
		startTime:       time.Now(),
		capacityCmds:    capacity,
		cliVersion:      cliVersion,
		fuseMountsRefFn: fuseMountsFn,
	}
}

func (h *HealthState) IncCommand() { h.totalCommands.Add(1) }
func (h *HealthState) IncError()   { h.totalErrors.Add(1) }
func (h *HealthState) IncInflight() {
	h.inflight.Add(1)
}
func (h *HealthState) DecInflight() {
	h.inflight.Add(-1)
}
func (h *HealthState) SetQueued(n int) {
	h.queued.Store(int64(n))
}

func (h *HealthState) Inflight() int {
	return int(h.inflight.Load())
}

func (h *HealthState) Snapshot() map[string]any {
	mountCount := 0
	if h.fuseMountsRefFn != nil {
		mountCount = h.fuseMountsRefFn()
	}
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	return map[string]any{
		"version":          h.cliVersion,
		"uptimeSec":        int(time.Since(h.startTime).Seconds()),
		"inflight":         int(h.inflight.Load()),
		"queued":           int(h.queued.Load()),
		"capacityCommands": h.capacityCmds,
		"totalCommands":    int(h.totalCommands.Load()),
		"totalErrors":      int(h.totalErrors.Load()),
		"fuseMounts":       mountCount,
		"memoryRssBytes":   int(mem.Sys),
	}
}

func runHealth(ctx context.Context, env *Envelope, cfg RunuserConfig, state *HealthState) Result {
	start := time.Now()
	dataBytes, _ := json.Marshal(state.Snapshot())
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
}
