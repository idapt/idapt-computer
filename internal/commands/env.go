package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const envSourceMarker = "idapt-auto-env"

const (
	envBashrcDir = ".bashrc.d"
	envFileName  = "idapt-env"
)

var envProfileFiles = []string{".bashrc", ".profile"}

func authorizeEnvTarget(username, runAs string) (*runAsOwner, error) {
	if username == "" {
		return nil, errors.New("username required")
	}
	if !posixUserRegex.MatchString(username) {
		return nil, errors.New("malformed username")
	}
	if username != runAs {
		return nil, fmt.Errorf("%s: env target user %q must equal runAs %q", ErrRunAsForbidden, username, runAs)
	}
	owner, err := resolveRunAsOwner(username)
	if err != nil {
		return nil, err
	}
	return owner, nil
}

func ensureBashrcDir(owner *runAsOwner) error {
	dir := filepath.Join(owner.HomeDir, envBashrcDir)
	if err := mkdirAllConfined(owner, dir, 0o755); err != nil {
		return err
	}
	return chownConfinedNoFollow(owner, dir)
}

func ensureShellSourcesEnv(owner *runAsOwner) error {
	snippet := "\n# " + envSourceMarker + "\n[ -f \"$HOME/.bashrc.d/idapt-env\" ] && . \"$HOME/.bashrc.d/idapt-env\"\n"
	for _, name := range envProfileFiles {
		path := filepath.Join(owner.HomeDir, name)

		if rf, err := openConfined(owner, path, os.O_RDONLY, 0); err == nil {
			body, _ := io.ReadAll(rf)
			rf.Close()
			if strings.Contains(string(body), envSourceMarker) {
				continue
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}

		f, err := openConfined(owner, path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		if _, werr := f.WriteString(snippet); werr != nil {
			_ = f.Close()
			return werr
		}
		if os.Geteuid() == 0 {
			_ = f.Chown(owner.UID, owner.GID)
		}
		if cerr := f.Close(); cerr != nil {
			return cerr
		}
	}
	return nil
}

func envFilePath(owner *runAsOwner) (string, error) {
	if err := ensureBashrcDir(owner); err != nil {
		return "", err
	}
	if err := ensureShellSourcesEnv(owner); err != nil {
		return "", err
	}
	return filepath.Join(owner.HomeDir, envBashrcDir, envFileName), nil
}

func readEnvMap(owner *runAsOwner, path string) map[string]string {
	out := map[string]string{}
	f, err := openConfined(owner, path, os.O_RDONLY, 0)
	if err != nil {
		return out
	}
	defer f.Close()
	body, err := io.ReadAll(f)
	if err != nil {
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
		rest := strings.TrimPrefix(line, "export ")
		eq := strings.IndexByte(rest, '=')
		if eq < 0 {
			continue
		}
		name := rest[:eq]
		val := rest[eq+1:]
		if len(val) >= 2 && val[0] == '\'' && val[len(val)-1] == '\'' {
			val = val[1 : len(val)-1]
			val = strings.ReplaceAll(val, "'\\''", "'")
		}
		out[name] = val
	}
	return out
}

func writeEnvMap(owner *runAsOwner, path string, m map[string]string) error {
	var sb strings.Builder
	sb.WriteString("# Idapt-managed env vars. Don't edit by hand.\n")
	for k, v := range m {
		sb.WriteString("export " + k + "=" + shellQuote(v) + "\n")
	}
	tmp := path + ".idapt-tmp-" + randSuffix()
	f, err := openConfined(owner, tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, werr := f.WriteString(sb.String()); werr != nil {
		_ = f.Close()
		_ = removeConfined(owner, tmp, false)
		return werr
	}
	if os.Geteuid() == 0 {
		_ = f.Chown(owner.UID, owner.GID)
	}
	if cerr := f.Close(); cerr != nil {
		_ = removeConfined(owner, tmp, false)
		return cerr
	}
	if err := renameConfined(owner, tmp, path); err != nil {
		_ = removeConfined(owner, tmp, false)
		return err
	}
	return chownConfinedNoFollow(owner, path)
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
	owner, err := authorizeEnvTarget(p.Username, env.RunAs)
	if err != nil {
		return errResult(env.ID, ErrRunAsForbidden, err.Error(), start)
	}
	path, err := envFilePath(owner)
	if err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	m := readEnvMap(owner, path)
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
	owner, err := authorizeEnvTarget(p.Username, env.RunAs)
	if err != nil {
		return errResult(env.ID, ErrRunAsForbidden, err.Error(), start)
	}
	path, err := envFilePath(owner)
	if err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	m := readEnvMap(owner, path)
	m[p.Name] = p.Value
	if err := writeEnvMap(owner, path, m); err != nil {
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
	owner, err := authorizeEnvTarget(p.Username, env.RunAs)
	if err != nil {
		return errResult(env.ID, ErrRunAsForbidden, err.Error(), start)
	}
	path, err := envFilePath(owner)
	if err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	m := readEnvMap(owner, path)
	delete(m, p.Name)
	if err := writeEnvMap(owner, path, m); err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: json.RawMessage("{}")}
}
