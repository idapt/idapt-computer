package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var tmuxWindowRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,32}$`)

func sessionName(runAs string) string {
	if runAs == "" {
		runAs = "idapt"
	}
	return fmt.Sprintf("idapt-%s", runAs)
}

func runTmuxRun(ctx context.Context, env *Envelope, cfg RunuserConfig) Result {
	start := time.Now()
	if err := ValidateRunAs(env.RunAs, cfg); err != nil {
		return errResult(env.ID, ErrRunAsForbidden, err.Error(), start)
	}
	var p TmuxRunPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	if !tmuxWindowRegex.MatchString(p.Window) {
		return errResult(env.ID, ErrInvalidPayload, "bad window name", start)
	}
	if p.Cmd == "" {
		return errResult(env.ID, ErrInvalidPayload, "cmd required", start)
	}

	session := sessionName(env.RunAs)

	if err := tmuxExec(ctx, env.RunAs, "new-session", "-d", "-s", session); err != nil && !isTmuxAlreadyExists(err) {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}

	_ = tmuxExec(ctx, env.RunAs, "new-window", "-t", session+":", "-n", p.Window)

	target := session + ":" + p.Window
	innerCmd := p.Cmd
	if env.CWD != "" {
		innerCmd = "cd " + shellQuote(env.CWD) + " && " + innerCmd
	}
	if err := tmuxExec(ctx, env.RunAs, "send-keys", "-t", target, innerCmd, "Enter"); err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}

	out := ""
	if p.Capture {
		delay := 500 * time.Millisecond
		if p.CaptureDelayMs > 0 {
			delay = time.Duration(p.CaptureDelayMs) * time.Millisecond
		}
		select {
		case <-time.After(delay):
		case <-ctx.Done():
		}
		captured, err := tmuxOutput(ctx, env.RunAs, "capture-pane", "-t", target, "-p")
		if err == nil {
			out = captured
		}
	}

	dataBytes, _ := json.Marshal(map[string]any{"output": out})
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
}

func runTmuxCapture(ctx context.Context, env *Envelope, cfg RunuserConfig) Result {
	start := time.Now()
	if err := ValidateRunAs(env.RunAs, cfg); err != nil {
		return errResult(env.ID, ErrRunAsForbidden, err.Error(), start)
	}
	var p TmuxCapturePayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	if !tmuxWindowRegex.MatchString(p.Window) {
		return errResult(env.ID, ErrInvalidPayload, "bad window name", start)
	}
	target := sessionName(env.RunAs) + ":" + p.Window
	args := []string{"capture-pane", "-t", target, "-p"}
	if p.ScrollbackLines > 0 {
		args = append(args, "-S", fmt.Sprintf("-%d", p.ScrollbackLines))
	}
	out, err := tmuxOutput(ctx, env.RunAs, args...)
	if err != nil {
		if isTmuxNoSession(err) {
			return errResult(env.ID, ErrPathNotFound, "no such window", start)
		}
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	dataBytes, _ := json.Marshal(map[string]any{"output": out})
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
}

func runTmuxSendKeys(ctx context.Context, env *Envelope, cfg RunuserConfig) Result {
	start := time.Now()
	if err := ValidateRunAs(env.RunAs, cfg); err != nil {
		return errResult(env.ID, ErrRunAsForbidden, err.Error(), start)
	}
	var p TmuxSendKeysPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	if !tmuxWindowRegex.MatchString(p.Window) {
		return errResult(env.ID, ErrInvalidPayload, "bad window name", start)
	}
	target := sessionName(env.RunAs) + ":" + p.Window

	var keysArr []string
	var single string
	if err := json.Unmarshal(p.Keys, &single); err == nil {
		keysArr = []string{single}
	} else if err := json.Unmarshal(p.Keys, &keysArr); err != nil {
		return errResult(env.ID, ErrInvalidPayload, "keys must be string or [string]", start)
	}

	delay := 100 * time.Millisecond
	if p.DelayMs > 0 {
		delay = time.Duration(p.DelayMs) * time.Millisecond
	}
	for i, k := range keysArr {
		if err := tmuxExec(ctx, env.RunAs, "send-keys", "-t", target, k); err != nil {
			return errResult(env.ID, ErrIO, err.Error(), start)
		}
		if i < len(keysArr)-1 {
			select {
			case <-time.After(delay):
			case <-ctx.Done():
			}
		}
	}

	out := ""
	if p.Capture {
		cd := 300 * time.Millisecond
		if p.CaptureDelayMs > 0 {
			cd = time.Duration(p.CaptureDelayMs) * time.Millisecond
		}
		select {
		case <-time.After(cd):
		case <-ctx.Done():
		}
		captured, _ := tmuxOutput(ctx, env.RunAs, "capture-pane", "-t", target, "-p")
		out = captured
	}
	dataBytes, _ := json.Marshal(map[string]any{"output": out})
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
}

func runTmuxList(ctx context.Context, env *Envelope, cfg RunuserConfig) Result {
	start := time.Now()
	if err := ValidateRunAs(env.RunAs, cfg); err != nil {
		return errResult(env.ID, ErrRunAsForbidden, err.Error(), start)
	}
	out, err := tmuxOutput(ctx, env.RunAs, "list-windows", "-t", sessionName(env.RunAs), "-F", "#{window_name}|#{window_panes}|#{window_active}")
	if err != nil {
		dataBytes, _ := json.Marshal(map[string]any{"windows": []any{}})
		return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
	}
	type win struct {
		Name      string `json:"name"`
		PaneCount int    `json:"paneCount"`
		Active    bool   `json:"active"`
	}
	wins := []win{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			continue
		}
		var paneCount int
		fmt.Sscanf(parts[1], "%d", &paneCount)
		wins = append(wins, win{
			Name:      parts[0],
			PaneCount: paneCount,
			Active:    parts[2] == "1",
		})
	}
	dataBytes, _ := json.Marshal(map[string]any{"windows": wins})
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
}

func runTmuxKill(ctx context.Context, env *Envelope, cfg RunuserConfig) Result {
	start := time.Now()
	if err := ValidateRunAs(env.RunAs, cfg); err != nil {
		return errResult(env.ID, ErrRunAsForbidden, err.Error(), start)
	}
	var p struct {
		Window string `json:"window"`
	}
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	if !tmuxWindowRegex.MatchString(p.Window) {
		return errResult(env.ID, ErrInvalidPayload, "bad window name", start)
	}
	target := sessionName(env.RunAs) + ":" + p.Window
	if err := tmuxExec(ctx, env.RunAs, "kill-window", "-t", target); err != nil {
		if isTmuxNoSession(err) {
			return errResult(env.ID, ErrPathNotFound, "no such window", start)
		}
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: json.RawMessage("{}")}
}

func tmuxExec(ctx context.Context, runAs string, args ...string) error {
	cmd := buildTmuxCmd(ctx, runAs, args)
	return cmd.Run()
}

func tmuxOutput(ctx context.Context, runAs string, args ...string) (string, error) {
	cmd := buildTmuxCmd(ctx, runAs, args)
	out, err := cmd.Output()
	return string(out), err
}

func buildTmuxCmd(ctx context.Context, runAs string, args []string) *exec.Cmd {
	if runAs == "" {
		return exec.CommandContext(ctx, "tmux", args...)
	}
	innerArgs := append([]string{"-u", runAs, "--", "tmux"}, args...)
	return exec.CommandContext(ctx, "runuser", innerArgs...)
}

func isTmuxAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return strings.Contains(string(ee.Stderr), "duplicate session")
	}
	return false
}

func isTmuxNoSession(err error) bool {
	if err == nil {
		return false
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		s := string(ee.Stderr)
		return strings.Contains(s, "no session") ||
			strings.Contains(s, "can't find") ||
			strings.Contains(s, "no server")
	}
	return false
}
