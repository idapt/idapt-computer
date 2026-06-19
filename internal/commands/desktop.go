package commands
import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const maxDesktopDurationMs = 100_000

func runDesktop(ctx context.Context, env *Envelope, cfg RunuserConfig) Result {
	start := time.Now()

	if err := ValidateRunAs(env.RunAs, cfg); err != nil {
		return errResult(env.ID, ErrRunAsForbidden, err.Error(), start)
	}

	var p DesktopPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	if p.Action == "" {
		return errResult(env.ID, ErrInvalidPayload, "action required", start)
	}

	if p.Action == DesktopActionWait {
		d := clampDesktopDuration(p.DurationMs)
		if d > 0 {
			select {
			case <-time.After(d):
			case <-ctx.Done():
				return errResult(env.ID, ErrCancelled, "wait cancelled", start)
			}
		}
		return desktopAck(env.ID, p.Action, start)
	}

	backend := selectDesktopBackend(cfg, env.RunAs, env.Env)
	if ok, hint := backend.Probe(); !ok {
		return errResult(env.ID, ErrRuntimeUnavailable, hint, start)
	}

	switch p.Action {
	case DesktopActionScreenshot:
		raw, width, height, err := backend.Capture(ctx)
		if err != nil {
			return errResult(env.ID, ErrIO, err.Error(), start)
		}
		if len(raw) == 0 {
			return errResult(env.ID, ErrIO, "screenshot produced no bytes", start)
		}
		data, _ := json.Marshal(DesktopScreenshotResult{
			ImageB64: base64.StdEncoding.EncodeToString(raw),
			Width:    width,
			Height:   height,
		})
		return desktopOK(env.ID, data, start)

	case DesktopActionCursorPosition:
		x, y, err := backend.CursorPosition(ctx)
		if err != nil {
			return errResult(env.ID, ErrIO, err.Error(), start)
		}
		data, _ := json.Marshal(DesktopCursorPositionResult{X: x, Y: y})
		return desktopOK(env.ID, data, start)

	case DesktopActionMouseMove, DesktopActionLeftClick, DesktopActionRightClick,
		DesktopActionMiddleClick, DesktopActionDoubleClick, DesktopActionTripleClick,
		DesktopActionLeftClickDrag, DesktopActionLeftMouseDown, DesktopActionLeftMouseUp,
		DesktopActionScroll, DesktopActionKey, DesktopActionType, DesktopActionHoldKey:
		if err := backend.Input(ctx, p); err != nil {
			return errResult(env.ID, ErrIO, err.Error(), start)
		}
		return desktopAck(env.ID, p.Action, start)
	}

	return errResult(env.ID, ErrInvalidPayload, "unknown desktop action: "+p.Action, start)
}

func clampDesktopDuration(ms int) time.Duration {
	if ms <= 0 {
		return 0
	}
	if ms > maxDesktopDurationMs {
		ms = maxDesktopDurationMs
	}
	return time.Duration(ms) * time.Millisecond
}

func desktopAck(id, action string, start time.Time) Result {
	data, _ := json.Marshal(DesktopActionAck{OK: true, Action: action})
	return desktopOK(id, data, start)
}

func desktopOK(id string, data []byte, start time.Time) Result {
	return Result{
		ID:         id,
		OK:         true,
		DurationMs: time.Since(start).Milliseconds(),
		Data:       data,
	}
}

func runDesktopShell(
	ctx context.Context,
	cfg RunuserConfig,
	runAs, inner string,
	env map[string]string,
) (string, error) {
	_ = cfg
	cmd := BuildPrlimitCommand(ctx, runAs, inner, env, KindDesktop)
	outBuf := newCapped(MaxOutputBytes)
	errBuf := newCapped(16 * 1024)
	cmd.Stdout = outBuf
	cmd.Stderr = errBuf
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(string(errBuf.Bytes()))
		if msg == "" {
			msg = err.Error()
		}
		return string(outBuf.Bytes()), errors.New(msg)
	}
	return string(outBuf.Bytes()), nil
}

func parseTwoInts(s string) (int, int, error) {
	fields := strings.FieldsFunc(strings.TrimSpace(s), func(r rune) bool {
		return r == ' ' || r == ',' || r == '\t' || r == '\n'
	})
	if len(fields) < 2 {
		return 0, 0, fmt.Errorf("cursor position: cannot parse %q", s)
	}
	x, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, 0, fmt.Errorf("cursor position x: %w", err)
	}
	y, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, 0, fmt.Errorf("cursor position y: %w", err)
	}
	return x, y, nil
}

func coord(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}
