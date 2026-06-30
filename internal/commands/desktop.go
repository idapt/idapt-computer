package commands
import (
	"context"
	"crypto/sha256"
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
		shot, err := backend.Capture(ctx, p.DisplayID)
		if err != nil {
			return errResult(env.ID, ErrIO, err.Error(), start)
		}
		if len(shot.PNG) == 0 {
			return errResult(env.ID, ErrIO, "screenshot produced no bytes", start)
		}
		return desktopOK(env.ID, marshalScreenshot(shot), start)

	case DesktopActionWaitForStable:
		shot, err := waitForStableCapture(ctx, backend, p.DisplayID, clampDesktopDuration(p.DurationMs))
		if err != nil {
			return errResult(env.ID, ErrIO, err.Error(), start)
		}
		return desktopOK(env.ID, marshalScreenshot(shot), start)

	case DesktopActionCursorPosition:
		x, y, err := backend.CursorPosition(ctx)
		if err != nil {
			return errResult(env.ID, ErrIO, err.Error(), start)
		}
		data, _ := json.Marshal(DesktopCursorPositionResult{X: x, Y: y})
		return desktopOK(env.ID, data, start)

	case DesktopActionListDisplays:
		displays, err := backend.ListDisplays(ctx)
		if err != nil {
			return desktopErrFor(env.ID, err, start)
		}
		data, _ := json.Marshal(DesktopDisplaysResult{Displays: displays})
		return desktopOK(env.ID, data, start)

	case DesktopActionClipboardGet:
		text, err := backend.Clipboard(ctx)
		if err != nil {
			return desktopErrFor(env.ID, err, start)
		}
		data, _ := json.Marshal(DesktopClipboardResult{Text: text})
		return desktopOK(env.ID, data, start)

	case DesktopActionClipboardSet:
		if err := backend.SetClipboard(ctx, p.Text); err != nil {
			return desktopErrFor(env.ID, err, start)
		}
		return desktopAck(env.ID, p.Action, start)

	case DesktopActionWindowList:
		windows, err := backend.ListWindows(ctx)
		if err != nil {
			return desktopErrFor(env.ID, err, start)
		}
		data, _ := json.Marshal(DesktopWindowsResult{Windows: windows})
		return desktopOK(env.ID, data, start)

	case DesktopActionWindowFocus:
		if p.WindowID == "" && strings.TrimSpace(p.Text) == "" {
			return errResult(env.ID, ErrInvalidPayload, "window-focus requires windowId or text (title match)", start)
		}
		if err := backend.FocusWindow(ctx, p.WindowID, p.Text); err != nil {
			return desktopErrFor(env.ID, err, start)
		}
		return desktopAck(env.ID, p.Action, start)

	case DesktopActionMouseMove, DesktopActionHover, DesktopActionLeftClick, DesktopActionRightClick,
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

func marshalScreenshot(shot DesktopCapture) []byte {
	data, _ := json.Marshal(DesktopScreenshotResult{
		ImageB64:          base64.StdEncoding.EncodeToString(shot.PNG),
		Width:             shot.Width,
		Height:            shot.Height,
		DisplayID:         shot.DisplayID,
		OffsetX:           shot.OffsetX,
		OffsetY:           shot.OffsetY,
		ActiveWindowTitle: shot.ActiveWindowTitle,
	})
	return data
}

func desktopErrFor(id string, err error, start time.Time) Result {
	if errors.Is(err, errDesktopActionUnsupported) {
		return errResult(id, ErrRuntimeUnavailable, err.Error(), start)
	}
	return errResult(id, ErrIO, err.Error(), start)
}

func waitForStableCapture(
	ctx context.Context,
	backend DesktopBackend,
	displayID string,
	maxWait time.Duration,
) (DesktopCapture, error) {
	if maxWait <= 0 {
		maxWait = 5 * time.Second
	}
	const pollInterval = 350 * time.Millisecond
	deadline := time.Now().Add(maxWait)
	var last DesktopCapture
	var lastHash [32]byte
	have := false
	for {
		shot, err := backend.Capture(ctx, displayID)
		if err != nil {
			return DesktopCapture{}, err
		}
		h := sha256.Sum256(shot.PNG)
		if have && h == lastHash {
			return shot, nil
		}
		last, lastHash, have = shot, h, true
		if time.Now().After(deadline) {
			return last, nil
		}
		select {
		case <-time.After(pollInterval):
		case <-ctx.Done():
			return last, ctx.Err()
		}
	}
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
