package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
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
	activeMu     sync.Mutex
	active       map[string]context.CancelFunc
}

type ResultPoster interface {
	Post(ctx context.Context, r Result) error
}

type ChunkPoster interface {
	PostChunk(ctx context.Context, c Chunk) error
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
		active:       make(map[string]context.CancelFunc),
	}
	for i := 0; i < capacity; i++ {
		go e.worker()
	}
	return e
}

func (e *Executor) Submit(ctx context.Context, env *Envelope) error {
	if env.Kind == KindCancel {
		res := e.runCancel(env)
		postCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = e.resultPoster.Post(postCtx, res)
		cancel()
		return nil
	}
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

	commandCtx, cancel := context.WithCancel(ctx)
	e.track(env.ID, cancel)
	t := task{ctx: commandCtx, env: env}
	select {
	case e.queue <- t:
		e.healthState.SetQueued(len(e.queue))
		return nil
	default:
		cancel()
		e.untrack(env.ID)
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

func (e *Executor) track(commandID string, cancel context.CancelFunc) {
	e.activeMu.Lock()
	defer e.activeMu.Unlock()
	e.active[commandID] = cancel
}

func (e *Executor) untrack(commandID string) {
	e.activeMu.Lock()
	defer e.activeMu.Unlock()
	delete(e.active, commandID)
}

func (e *Executor) cancelCommand(commandID string) bool {
	e.activeMu.Lock()
	cancel := e.active[commandID]
	e.activeMu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

func (e *Executor) Stop() {
	if !e.stopped.CompareAndSwap(false, true) {
		return
	}
	e.activeMu.Lock()
	for _, cancel := range e.active {
		cancel()
	}
	e.activeMu.Unlock()
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
			var res Result
			if t.ctx.Err() != nil {
				res = cancelledResult(t.env.ID)
			} else {
				res = e.runOne(t.ctx, t.env)
			}
			e.untrack(t.env.ID)
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

func (e *Executor) runCancel(env *Envelope) Result {
	start := time.Now()
	var payload CancelPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	if payload.TargetCommandID == "" {
		return errResult(env.ID, ErrInvalidPayload, "targetCommandId required", start)
	}
	cancelled := e.cancelCommand(payload.TargetCommandID)
	dataBytes, _ := json.Marshal(map[string]any{
		"targetCommandId": payload.TargetCommandID,
		"cancelled":       cancelled,
	})
	return Result{
		ID:         env.ID,
		OK:         true,
		DurationMs: time.Since(start).Milliseconds(),
		Data:       dataBytes,
	}
}

func cancelledResult(commandID string) Result {
	dataBytes, _ := json.Marshal(ExecResult{
		ExitCode: nil,
		Stdout:   "",
		Stderr:   "",
		TimedOut: false,
	})
	return Result{
		ID:         commandID,
		OK:         false,
		DurationMs: 0,
		Data:       dataBytes,
		Error:      &ResultError{Code: ErrCancelled, Message: "command cancelled"},
	}
}

func (e *Executor) runOne(ctx context.Context, env *Envelope) Result {
	cctx, cancel := context.WithTimeout(ctx, SafeTimeout(env.TTLMs))
	defer cancel()

	switch env.Kind {
	case KindExec:
		return runShellExec(cctx, env, e.cfg)
	case KindExecStream:
		if poster, ok := e.resultPoster.(ChunkPoster); ok {
			return runShellExecStream(cctx, env, e.cfg, poster)
		}
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
	case KindLocalStatus:
		return runLocalInferenceStatus(cctx, env)
	case KindLocalInstall:
		if poster, ok := e.resultPoster.(ChunkPoster); ok {
			return runLocalInferenceRuntimeInstall(cctx, env, poster)
		}
		return runLocalInferenceRuntimeInstall(cctx, env, nil)
	case KindLocalUpdate:
		if poster, ok := e.resultPoster.(ChunkPoster); ok {
			return runLocalInferenceRuntimeUpdate(cctx, env, poster)
		}
		return runLocalInferenceRuntimeUpdate(cctx, env, nil)
	case KindLocalStart:
		return runLocalInferenceRuntimeStart(cctx, env)
	case KindLocalStop:
		return runLocalInferenceRuntimeStop(cctx, env)
	case KindLocalLogs:
		return runLocalInferenceRuntimeLogs(cctx, env)
	case KindLocalModelList:
		return runLocalInferenceModelList(cctx, env)
	case KindLocalModelPull:
		if poster, ok := e.resultPoster.(ChunkPoster); ok {
			return runLocalInferenceModelPull(cctx, env, poster)
		}
		return errResult(env.ID, ErrInternal, "chunk poster unavailable", time.Now())
	case KindLocalModelRm:
		return runLocalInferenceModelRemove(cctx, env)
	case KindLocalModelCreate:
		if poster, ok := e.resultPoster.(ChunkPoster); ok {
			return runLocalInferenceModelCreate(cctx, env, poster)
		}
		return errResult(env.ID, ErrInternal, "chunk poster unavailable", time.Now())
	case KindLocalChat:
		if poster, ok := e.resultPoster.(ChunkPoster); ok {
			return runLocalInferenceChat(cctx, env, poster)
		}
		return errResult(env.ID, ErrInternal, "chunk poster unavailable", time.Now())
	case KindAppRuntimeStatus:
		return runComputerAppRuntimeStatus(cctx, env)
	case KindAppRuntimeSetup:
		return runComputerAppRuntimeSetup(cctx, env)
	case KindAppList:
		return runComputerAppList(cctx, env)
	case KindAppExternalList:
		return runComputerAppExternalList(cctx, env)
	case KindAppInspect:
		return runComputerAppInspect(cctx, env)
	case KindAppCreate:
		return runComputerAppCreate(cctx, env)
	case KindAppRun:
		return runComputerAppRun(cctx, env)
	case KindAppBuild:
		return runComputerAppRun(cctx, env)
	case KindAppComposeUp:
		return runComputerAppComposeUp(cctx, env)
	case KindAppStart:
		return runComputerAppLifecycle(cctx, env, "start")
	case KindAppStop:
		return runComputerAppLifecycle(cctx, env, "stop")
	case KindAppRestart:
		return runComputerAppLifecycle(cctx, env, "restart")
	case KindAppDelete:
		return runComputerAppLifecycle(cctx, env, "delete")
	case KindAppReset:
		return runComputerAppLifecycle(cctx, env, "reset")
	case KindAppLogs:
		return runComputerAppLogs(cctx, env)
	case KindAppExec:
		return runComputerAppExec(cctx, env)
	case KindAppPorts:
		return runComputerAppInspect(cctx, env)
	case KindAppExpose:
		return runComputerAppExpose(cctx, env, true)
	case KindAppUnexpose:
		return runComputerAppExpose(cctx, env, false)
	case KindDesktop:
		return runDesktop(cctx, env, e.cfg)
	case KindCancel:
		return e.runCancel(env)
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
		Error:      &ResultError{Code: ErrUnsupportedKind, Message: fmt.Sprintf("unsupported kind: %s", env.Kind)},
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
