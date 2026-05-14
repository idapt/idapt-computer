package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

var secretsBaseDir = "/run/idapt-secrets"

func runShellExec(ctx context.Context, env *Envelope, cfg RunuserConfig) Result {
	start := time.Now()

	if err := ValidateRunAs(env.RunAs, cfg); err != nil {
		return errResult(env.ID, ErrRunAsForbidden, err.Error(), start)
	}

	var payload ExecPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	if payload.Cmd == "" {
		return errResult(env.ID, ErrInvalidPayload, "cmd required", start)
	}
	if env.Container != "" {
		return errResult(env.ID, ErrContainerNotFound, "container exec not implemented", start)
	}

	timeout := SafeTimeout(env.TTLMs)
	if payload.TimeoutMs > 0 {
		t := time.Duration(payload.TimeoutMs) * time.Millisecond
		if t < timeout {
			timeout = t
		}
	}

	envMap := map[string]string{}
	for k, v := range env.Env {
		envMap[k] = v
	}
	tmpfsDir, cleanup, err := materializeSecrets(env.ID, env.Secrets, envMap, env.RunAs)
	if err != nil {
		return errResult(env.ID, ErrInternal, err.Error(), start)
	}
	defer cleanup()
	_ = tmpfsDir

	inner := payload.Cmd
	if env.CWD != "" {
		inner = "cd " + shellQuote(env.CWD) + " && " + inner
	}
	if payload.ShellMode == "strict" {
		inner = "set -o pipefail; " + inner
	}

	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := BuildPrlimitCommand(cctx, env.RunAs, inner, envMap, env.Kind)

	stdoutBuf := newCapped(MaxOutputBytes)
	stderrBuf := newCapped(MaxOutputBytes)
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf

	err = cmd.Run()
	exitCode := -1
	timedOut := false
	if cctx.Err() == context.DeadlineExceeded {
		timedOut = true
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exitCode = exitErr.ExitCode()
	} else if err == nil {
		exitCode = 0
	}

	durMs := time.Since(start).Milliseconds()
	dataBytes, _ := json.Marshal(ExecResult{
		ExitCode: ptrInt(exitCode),
		Stdout:   string(stdoutBuf.Bytes()),
		Stderr:   string(stderrBuf.Bytes()),
		TimedOut: timedOut,
	})
	res := Result{
		ID:         env.ID,
		OK:         !timedOut && err == nil || exitCode != -1,
		DurationMs: durMs,
		Data:       dataBytes,
		Truncated:  stdoutBuf.Truncated || stderrBuf.Truncated,
	}
	if timedOut {
		res.OK = false
		res.Error = &ResultError{Code: ErrCommandTimeout, Message: "command exceeded timeout"}
	}
	return res
}

func ptrInt(i int) *int { return &i }

type cappedBuffer struct {
	buf       bytes.Buffer
	cap       int
	Truncated bool
}

func newCapped(cap int) *cappedBuffer {
	return &cappedBuffer{cap: cap}
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if c.buf.Len() >= c.cap {
		c.Truncated = true
		return len(p), nil // pretend write succeeded but discard
	}
	avail := c.cap - c.buf.Len()
	if len(p) <= avail {
		return c.buf.Write(p)
	}
	c.Truncated = true
	c.buf.Write(p[:avail])
	return len(p), nil
}

func (c *cappedBuffer) Bytes() []byte { return c.buf.Bytes() }

func errResult(id, code, msg string, start time.Time) Result {
	return Result{
		ID:         id,
		OK:         false,
		DurationMs: time.Since(start).Milliseconds(),
		Error:      &ResultError{Code: code, Message: msg},
	}
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	out := make([]byte, 0, len(s)+2)
	out = append(out, '\'')
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			out = append(out, '\'', '\\', '\'', '\'')
		} else {
			out = append(out, s[i])
		}
	}
	out = append(out, '\'')
	return string(out)
}

func materializeSecrets(commandID string, secrets []SecretInjection, envMap map[string]string, runAs string) (string, func(), error) {
	if len(secrets) == 0 {
		return "", func() {}, nil
	}
	dir := filepath.Join(secretsBaseDir, commandID)
	hasFile := false
	for _, s := range secrets {
		if s.Mode == "file" {
			hasFile = true
			break
		}
	}
	cleanup := func() {}
	if hasFile {
		owner, err := resolveRunAsOwner(runAs)
		if err != nil {
			return "", cleanup, err
		}
		if err := os.MkdirAll(secretsBaseDir, 0o755); err != nil {
			return "", cleanup, err
		}
		if err := os.Chmod(secretsBaseDir, 0o755); err != nil {
			return "", cleanup, err
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", cleanup, err
		}
		cleanup = func() { _ = os.RemoveAll(dir) }
		if err := os.Chown(dir, owner.UID, owner.GID); err != nil {
			cleanup()
			return "", func() {}, err
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			cleanup()
			return "", func() {}, err
		}
		for _, s := range secrets {
			if s.Mode == "file" {
				p := filepath.Join(dir, filepath.Base(s.Name))
				if err := os.WriteFile(p, []byte(s.Value), 0o400); err != nil {
					cleanup()
					return "", func() {}, err
				}
				if err := os.Chown(p, owner.UID, owner.GID); err != nil {
					cleanup()
					return "", func() {}, err
				}
				if err := os.Chmod(p, 0o400); err != nil {
					cleanup()
					return "", func() {}, err
				}
			}
		}
		envMap["IDAPT_SECRETS_DIR"] = dir
	}
	for _, s := range secrets {
		if s.Mode == "env" {
			envMap[s.Name] = s.Value
		}
	}
	return dir, cleanup, nil
}
