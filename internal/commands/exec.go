package commands

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

var secretsBaseDir = "/run/idapt-secrets"

var interactiveSudoRe = regexp.MustCompile(`(?:^|[;&|(\n])\s*sudo\s+[^-\s]`)

func sudoNeedsInteractivePassword(cmd string, isRoot bool) bool {
	if isRoot {
		return false
	}
	return interactiveSudoRe.MatchString(cmd)
}

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

	if sudoNeedsInteractivePassword(payload.Cmd, os.Geteuid() == 0) {
		return errResult(env.ID, ErrRunAsForbidden,
			"interactive sudo isn't supported over the daemon — use `sudo -n` with a NOPASSWD rule, or run the command with runAs=root",
			start)
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
	cancelled := cctx.Err() == context.Canceled
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
	} else if cancelled {
		res.OK = false
		res.Error = &ResultError{Code: ErrCancelled, Message: "command cancelled"}
	}
	return res
}

func runShellExecStream(ctx context.Context, env *Envelope, cfg RunuserConfig, poster ChunkPoster) Result {
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
	_, cleanup, err := materializeSecrets(env.ID, env.Secrets, envMap, env.RunAs)
	if err != nil {
		return errResult(env.ID, ErrInternal, err.Error(), start)
	}
	defer cleanup()

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

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return errResult(env.ID, ErrInternal, err.Error(), start)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return errResult(env.ID, ErrInternal, err.Error(), start)
	}

	stdoutBuf := newCapped(MaxOutputBytes)
	stderrBuf := newCapped(MaxOutputBytes)
	var mu sync.Mutex
	var stdoutOffset int64
	var stderrOffset int64
	var wg sync.WaitGroup

	streamPipe := func(channel string, reader io.Reader, buf *cappedBuffer, offset *int64) {
		defer wg.Done()
		tmp := make([]byte, 8192)
		for {
			n, readErr := reader.Read(tmp)
			if n > 0 {
				chunk := append([]byte(nil), tmp[:n]...)
				mu.Lock()
				_, _ = buf.Write(chunk)
				currentOffset := *offset
				*offset += int64(len(chunk))
				mu.Unlock()

				postCtx, postCancel := context.WithTimeout(context.Background(), 5*time.Second)
				_ = poster.PostChunk(postCtx, Chunk{
					ID:      env.ID,
					Offset:  currentOffset,
					Channel: channel,
					DataB64: base64.StdEncoding.EncodeToString(chunk),
				})
				postCancel()
			}
			if readErr != nil {
				return
			}
		}
	}

	if err := cmd.Start(); err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}

	wg.Add(2)
	go streamPipe("stdout", stdoutPipe, stdoutBuf, &stdoutOffset)
	go streamPipe("stderr", stderrPipe, stderrBuf, &stderrOffset)

	err = cmd.Wait()
	wg.Wait()

	exitCode := -1
	timedOut := false
	if cctx.Err() == context.DeadlineExceeded {
		timedOut = true
	}
	cancelled := cctx.Err() == context.Canceled
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
	} else if cancelled {
		res.OK = false
		res.Error = &ResultError{Code: ErrCancelled, Message: "command cancelled"}
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
		if err := os.Lchown(dir, owner.UID, owner.GID); err != nil {
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
				f, werr := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL|oNoFollow, 0o400)
				if werr != nil {
					cleanup()
					return "", func() {}, werr
				}
				if _, werr := f.Write([]byte(s.Value)); werr != nil {
					_ = f.Close()
					cleanup()
					return "", func() {}, werr
				}
				if os.Geteuid() == 0 {
					if cerr := f.Chown(owner.UID, owner.GID); cerr != nil {
						_ = f.Close()
						cleanup()
						return "", func() {}, cerr
					}
				}
				if cerr := f.Close(); cerr != nil {
					cleanup()
					return "", func() {}, cerr
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
