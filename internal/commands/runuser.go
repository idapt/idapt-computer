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

func mergeEnv(base []string, extra map[string]string) []string {
	if len(extra) == 0 {
		return base
	}
	out := make([]string, 0, len(base)+len(extra))
	seen := make(map[string]bool, len(extra))
	for _, kv := range base {
		i := strings.IndexByte(kv, '=')
		if i < 0 {
			out = append(out, kv)
			continue
		}
		key := kv[:i]
		if _, ok := extra[key]; ok {
			seen[key] = true
			continue
		}
		out = append(out, kv)
	}
	for k, v := range extra {
		_ = seen
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
