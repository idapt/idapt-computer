package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var posixUserRegex = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

type RunuserConfig struct {
	AllowRoot bool
	RestrictRunAs bool
	AllowedRunAs []string
}

var daemonSelfUsername = func() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	return u.Username, nil
}

func runAsAllowed(runAs string, cfg RunuserConfig) bool {
	if !cfg.RestrictRunAs {
		return true
	}
	if self, err := daemonSelfUsername(); err == nil && runAs == self {
		return true
	}
	for _, u := range cfg.AllowedRunAs {
		if u == runAs {
			return true
		}
	}
	return false
}

var lookupRunAsUID = func(name string) (int, error) {
	u, err := user.Lookup(name)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(u.Uid)
}

func ValidateRunAs(runAs string, cfg RunuserConfig) error {
	if !posixUserRegex.MatchString(runAs) {
		return fmt.Errorf("%s: malformed runAs", ErrRunAsForbidden)
	}
	if runAs == DaemonSelfUser {
		return fmt.Errorf("%s: _daemon is reserved", ErrRunAsForbidden)
	}
	if !runAsAllowed(runAs, cfg) {
		return fmt.Errorf("%s: runAs %q is not in the daemon's allowed-users policy", ErrRunAsForbidden, runAs)
	}
	if runtime.GOOS == "linux" {
		uid, err := lookupRunAsUID(runAs)
		if err != nil {
			return fmt.Errorf("%s: %v", ErrRunAsNotFound, err)
		}
		if uid == 0 && !cfg.AllowRoot {
			return fmt.Errorf("%s: uid 0 (root) requires explicit policy opt-in", ErrRunAsForbidden)
		}
		return nil
	}
	if runAs == "root" && !cfg.AllowRoot {
		return fmt.Errorf("%s: root requires explicit policy opt-in", ErrRunAsForbidden)
	}
	return nil
}

func isSelfIssuedDaemon(env *Envelope) bool {
	return env != nil && env.SelfIssued && env.RunAs == DaemonSelfUser
}

func BuildShellCommand(ctx context.Context, runAs, inner string, env map[string]string) *exec.Cmd {
	var cmd *exec.Cmd
	if runtime.GOOS == "linux" && runAs != "" {
		cmd = exec.CommandContext(ctx, "runuser", "-u", runAs, "--", "/bin/bash", "-c", inner)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/bash", "-c", inner)
	}
	cmd.Env = mergeEnv(os.Environ(), env)
	configureProcessGroup(cmd)
	return cmd
}

func BuildPrlimitCommand(ctx context.Context, runAs, inner string, env map[string]string, kind string) *exec.Cmd {
	if runtime.GOOS != "linux" {
		return BuildShellCommand(ctx, runAs, inner, env)
	}

	asLimit := "536870912" // 512 MB
	cpuLimit := "300"      // 5 minutes
	nofile := "1024"
	fsize := "1073741824" // 1 GB

	switch kind {
	case KindFileUpload, KindFileDownload:
		fsize = "10737418240" // 10 GB
	case KindPortDiscover, KindHealth:
		asLimit = "134217728" // 128 MB
	}

	args := []string{
		"--as=" + asLimit,
		"--cpu=" + cpuLimit,
		"--nofile=" + nofile,
		"--fsize=" + fsize,
		"--",
	}
	if runAs != "" {
		args = append(args, "runuser", "-u", runAs, "--", "/bin/bash", "-c", inner)
	} else {
		args = append(args, "/bin/bash", "-c", inner)
	}
	cmd := exec.CommandContext(ctx, "prlimit", args...)
	cmd.Env = mergeEnv(os.Environ(), env)
	configureProcessGroup(cmd)
	return cmd
}

func isSecretDaemonEnvKey(key string) bool {
	if !strings.HasPrefix(key, "IDAPT_") {
		return false
	}
	return strings.Contains(key, "TOKEN") ||
		strings.Contains(key, "SECRET") ||
		strings.Contains(key, "KEY") ||
		strings.Contains(key, "PASSWORD")
}

func isDangerousEnvKey(key string) bool {
	switch key {
	case "BASH_ENV", "ENV", "IFS", "SHELLOPTS", "BASHOPTS", "GLOBIGNORE", "PS4", "PROMPT_COMMAND", "BASH_XTRACEFD":
		return true
	}
	return strings.HasPrefix(key, "LD_") ||
		strings.HasPrefix(key, "DYLD_") ||
		strings.HasPrefix(key, "BASH_FUNC_")
}

func mergeEnv(base []string, extra map[string]string) []string {
	out := make([]string, 0, len(base)+len(extra))
	for _, kv := range base {
		i := strings.IndexByte(kv, '=')
		if i < 0 {
			continue // malformed; drop
		}
		key := kv[:i]
		if _, override := extra[key]; override {
			continue // replaced below
		}
		if isSecretDaemonEnvKey(key) || isDangerousEnvKey(key) {
			continue
		}
		out = append(out, kv)
	}
	for k, v := range extra {
		if isDangerousEnvKey(k) {
			continue // a hostile envelope cannot inject loader/shell-source keys
		}
		out = append(out, k+"="+v)
	}
	return out
}

func SafeTimeout(ttlMs int) time.Duration {
	if ttlMs <= 0 {
		return 60 * time.Second
	}
	if ttlMs > 30*60_000 {
		return 30 * time.Minute
	}
	return time.Duration(ttlMs) * time.Millisecond
}

var ErrAlreadyRunning = errors.New("already-running")
