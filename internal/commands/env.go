package commands

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type envFileTarget struct {
	path string
	uid  int
	gid  int
}

func envFileTargetForUser(username string) (envFileTarget, error) {
	if username == "" {
		return envFileTarget{}, errors.New("username required")
	}
	u, err := user.Lookup(username)
	if err != nil {
		return envFileTarget{}, err
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return envFileTarget{}, err
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return envFileTarget{}, err
	}
	dir := filepath.Join(u.HomeDir, ".bashrc.d")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return envFileTarget{}, err
	}
	if err := os.Chown(dir, uid, gid); err != nil {
		return envFileTarget{}, err
	}
	return envFileTarget{
		path: filepath.Join(dir, "idapt-env"),
		uid:  uid,
		gid:  gid,
	}, nil
}

func readEnvMap(path string) map[string]string {
	out := map[string]string{}
	body, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return out
		}
		return out
	}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, "export ") {
			continue
		}
		body := strings.TrimPrefix(line, "export ")
		eq := strings.IndexByte(body, '=')
		if eq < 0 {
			continue
		}
		name := body[:eq]
		val := body[eq+1:]
		if len(val) >= 2 && val[0] == '\'' && val[len(val)-1] == '\'' {
			val = val[1 : len(val)-1]
			val = strings.ReplaceAll(val, "'\\''", "'")
		}
		out[name] = val
	}
	return out
}

func writeEnvMap(target envFileTarget, m map[string]string) error {
	var sb strings.Builder
	sb.WriteString("# Idapt-managed env vars. Don't edit by hand.\n")
	for k, v := range m {
		sb.WriteString("export " + k + "=" + shellQuote(v) + "\n")
	}
	tmp := target.path + ".idapt-tmp"
	if err := os.WriteFile(tmp, []byte(sb.String()), 0o600); err != nil {
		return err
	}
	if err := os.Chown(tmp, target.uid, target.gid); err != nil {
		return err
	}
	if err := os.Rename(tmp, target.path); err != nil {
		return err
	}
	return os.Chown(target.path, target.uid, target.gid)
}

func runEnvList(ctx context.Context, env *Envelope, cfg RunuserConfig) Result {
	start := time.Now()
	if err := ValidateRunAs(env.RunAs, cfg); err != nil {
		return errResult(env.ID, ErrRunAsForbidden, err.Error(), start)
	}
	var p UsernamePayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	target, err := envFileTargetForUser(p.Username)
	if err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	m := readEnvMap(target.path)
	dataBytes, _ := json.Marshal(map[string]any{"vars": m})
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
}

func runEnvSet(ctx context.Context, env *Envelope, cfg RunuserConfig) Result {
	start := time.Now()
	if err := ValidateRunAs(env.RunAs, cfg); err != nil {
		return errResult(env.ID, ErrRunAsForbidden, err.Error(), start)
	}
	var p EnvSetPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	target, err := envFileTargetForUser(p.Username)
	if err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	m := readEnvMap(target.path)
	m[p.Name] = p.Value
	if err := writeEnvMap(target, m); err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: json.RawMessage("{}")}
}

func runEnvDelete(ctx context.Context, env *Envelope, cfg RunuserConfig) Result {
	start := time.Now()
	if err := ValidateRunAs(env.RunAs, cfg); err != nil {
		return errResult(env.ID, ErrRunAsForbidden, err.Error(), start)
	}
	var p EnvDeletePayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	target, err := envFileTargetForUser(p.Username)
	if err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	m := readEnvMap(target.path)
	delete(m, p.Name)
	if err := writeEnvMap(target, m); err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: json.RawMessage("{}")}
}
