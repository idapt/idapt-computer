package commands

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"time"
)

type Executor struct {
	cfg          RunuserConfig
	deduper      *Deduper
	semaphore    chan struct{}
	queue        chan task
	stop         chan struct{}
	stopped      atomic.Bool
	healthState  *HealthState
	resultPoster ResultPoster
}

type ResultPoster interface {
	Post(ctx context.Context, r Result) error
}

type task struct {
	ctx context.Context
	env *Envelope
}

func NewExecutor(cfg RunuserConfig, deduper *Deduper, capacity int, queue int, health *HealthState, poster ResultPoster) *Executor {
	if capacity <= 0 {
		capacity = 8
	}
	if queue <= 0 {
		queue = 32
	}
	e := &Executor{
		cfg:          cfg,
		deduper:      deduper,
		semaphore:    make(chan struct{}, capacity),
		queue:        make(chan task, queue),
		stop:         make(chan struct{}),
		healthState:  health,
		resultPoster: poster,
	}
	for i := 0; i < capacity; i++ {
		go e.worker()
	}
	return e
}

func (e *Executor) Submit(ctx context.Context, env *Envelope) error {
	if e.stopped.Load() {
		_ = e.resultPoster.Post(context.Background(), Result{
			ID:    env.ID,
			OK:    false,
			Error: &ResultError{Code: ErrShuttingDown, Message: "daemon is shutting down"},
		})
		return errors.New("shutting-down")
	}
	if cached, ok := e.deduper.Lookup(env.ID); ok {
		cached.Error = &ResultError{Code: ErrDuplicate, Message: "cached result"}
		_ = e.resultPoster.Post(context.Background(), cached)
		return nil
	}

	t := task{ctx: ctx, env: env}
	select {
	case e.queue <- t:
		e.healthState.SetQueued(len(e.queue))
		return nil
	default:
		if env.Priority == "low" {
			_ = e.resultPoster.Post(context.Background(), Result{
				ID:    env.ID,
				OK:    false,
				Error: &ResultError{Code: ErrRateLimited, Message: "queue full, low priority shed"},
			})
			return errors.New("rate-limited")
		}
		_ = e.resultPoster.Post(context.Background(), Result{
			ID:    env.ID,
			OK:    false,
			Error: &ResultError{Code: ErrRateLimited, Message: "queue full"},
		})
		return errors.New("rate-limited")
	}
}

func (e *Executor) Stop() {
	if !e.stopped.CompareAndSwap(false, true) {
		return
	}
	close(e.stop)
	close(e.queue)
}

func (e *Executor) worker() {
	for {
		select {
		case <-e.stop:
			return
		case t, ok := <-e.queue:
			if !ok {
				return
			}
			e.semaphore <- struct{}{}
			e.healthState.IncInflight()
			e.healthState.SetQueued(len(e.queue))
			res := e.runOne(t.ctx, t.env)
			e.healthState.DecInflight()
			<-e.semaphore
			e.healthState.IncCommand()
			if !res.OK {
				e.healthState.IncError()
			}
			res.Inflight = int(e.healthState.inflight.Load())
			res.Queued = int(e.healthState.queued.Load())
			if res.OK {
				e.deduper.Remember(t.env.ID, res)
			}
			postCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_ = e.resultPoster.Post(postCtx, res)
			cancel()
		}
	}
}

func (e *Executor) runOne(ctx context.Context, env *Envelope) Result {
	cctx, cancel := context.WithTimeout(ctx, SafeTimeout(env.TTLMs))
	defer cancel()

	switch env.Kind {
	case KindExec:
		return runShellExec(cctx, env, e.cfg)
	case KindExecStream:
		return runShellExec(cctx, env, e.cfg)
	case KindFileRead:
		return runFileRead(cctx, env, e.cfg)
	case KindFileWrite:
		return runFileWrite(cctx, env, e.cfg)
	case KindFileDelete:
		return runFileDelete(cctx, env, e.cfg)
	case KindFileList:
		return runFileList(cctx, env, e.cfg)
	case KindFileStat:
		return runFileStat(cctx, env, e.cfg)
	case KindFileMkdir:
		return runFileMkdir(cctx, env, e.cfg)
	case KindFileMove:
		return runFileMove(cctx, env, e.cfg)
	case KindFileGrep:
		return runFileGrep(cctx, env, e.cfg)
	case KindFileFind:
		return runFileFind(cctx, env, e.cfg)
	case KindTmuxRun:
		return runTmuxRun(cctx, env, e.cfg)
	case KindTmuxCapture:
		return runTmuxCapture(cctx, env, e.cfg)
	case KindTmuxSendKeys:
		return runTmuxSendKeys(cctx, env, e.cfg)
	case KindTmuxList:
		return runTmuxList(cctx, env, e.cfg)
	case KindTmuxKill:
		return runTmuxKill(cctx, env, e.cfg)
	case KindUserList:
		return runUserList(cctx, env, e.cfg)
	case KindUserCreate:
		return runUserCreate(cctx, env, e.cfg)
	case KindUserDelete:
		return runUserDelete(cctx, env, e.cfg)
	case KindUserEditGroups:
		return runUserEditGroups(cctx, env, e.cfg)
	case KindEnvList:
		return runEnvList(cctx, env, e.cfg)
	case KindEnvSet:
		return runEnvSet(cctx, env, e.cfg)
	case KindEnvDelete:
		return runEnvDelete(cctx, env, e.cfg)
	case KindPortDiscover:
		return runPortDiscover(cctx, env, e.cfg)
	case KindHealth:
		return runHealth(cctx, env, e.cfg, e.healthState)
	case KindShutdown:
		return runShutdown(cctx, env)
	}

	dataBytes, _ := json.Marshal(map[string]any{})
	return Result{
		ID:         env.ID,
		OK:         false,
		DurationMs: 0,
		Error:      &ResultError{Code: ErrUnsupportedKind, Message: "unsupported kind: " + env.Kind},
		Data:       dataBytes,
	}
}

var shutdownSignal = make(chan struct{}, 1)

func runShutdown(ctx context.Context, env *Envelope) Result {
	select {
	case shutdownSignal <- struct{}{}:
	default:
	}
	return Result{
		ID:         env.ID,
		OK:         true,
		DurationMs: 0,
		Data:       []byte("{}"),
	}
}

func ShutdownChan() <-chan struct{} { return shutdownSignal }
