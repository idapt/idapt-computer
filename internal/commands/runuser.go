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
	"strings"
	"time"
)

var posixUserRegex = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

type RunuserConfig struct {
	AllowRoot bool
}

func ValidateRunAs(runAs string, cfg RunuserConfig) error {
	if !posixUserRegex.MatchString(runAs) {
		return fmt.Errorf("%s: malformed runAs", ErrRunAsForbidden)
	}
	if runAs == DaemonSelfUser {
		return fmt.Errorf("%s: _daemon is reserved", ErrRunAsForbidden)
	}
	if runAs == "root" && !cfg.AllowRoot {
		return fmt.Errorf("%s: root requires explicit policy opt-in", ErrRunAsForbidden)
	}
	if runtime.GOOS == "linux" {
		if _, err := user.Lookup(runAs); err != nil {
			return fmt.Errorf("%s: %v", ErrRunAsNotFound, err)
		}
	}
	return nil
}

func BuildShellCommand(ctx context.Context, runAs, inner string, env map[string]string) *exec.Cmd {
	var cmd *exec.Cmd
	if runtime.GOOS == "linux" && runAs != "" {
		cmd = exec.CommandContext(ctx, "runuser", "-u", runAs, "--", "/bin/bash", "-c", inner)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/bash", "-c", inner)
	}
	cmd.Env = mergeEnv(os.Environ(), env)
	return cmd
}

func BuildPrlimitCommand(ctx context.Context, runAs, inner string, env map[string]string, kind string) *exec.Cmd {
	if runtime.GOOS != "linux" {
		return BuildShellCommand(ctx, runAs, inner, env)
	}

	asLimit := "536870912" // 512 MB
	cpuLimit := "300"      // 5 minutes
	nofile := "1024"
	nproc := "512"
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
		"--nproc=" + nproc,
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
	if ttlMs > 300_000 {
		return 300 * time.Second
	}
	return time.Duration(ttlMs) * time.Millisecond
}

var ErrAlreadyRunning = errors.New("already-running")
