package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/creack/pty"
)

type PTYSession struct {
	ptmx *os.File
	cmd  *exec.Cmd
}

func ptyShellInner(mode, runAs string) string {
	if mode == "tmux" {
		return "exec tmux new-session -A -s " + shellQuote(sessionName(runAs))
	}
	return "exec /bin/bash -l"
}

func ptyShellCmd(ctx context.Context, runAs, inner string, env map[string]string) *exec.Cmd {
	var cmd *exec.Cmd
	if runtime.GOOS == "linux" && runAs != "" {
		cmd = exec.CommandContext(ctx, "runuser", "-u", runAs, "--", "/bin/bash", "-c", inner)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/bash", "-c", inner)
	}
	cmd.Env = mergeEnv(os.Environ(), env)
	ptyConfigureCancel(cmd)
	return cmd
}

func StartPTYShell(ctx context.Context, runAs, mode string, cols, rows uint16, cfg RunuserConfig) (*PTYSession, error) {
	if err := ValidateRunAs(runAs, cfg); err != nil {
		return nil, err
	}
	if mode != "" && mode != "shell" && mode != "tmux" {
		return nil, fmt.Errorf("invalid terminal mode %q", mode)
	}
	if cols == 0 {
		cols = 80
	}
	if rows == 0 {
		rows = 24
	}
	cmd := ptyShellCmd(ctx, runAs, ptyShellInner(mode, runAs), map[string]string{
		"TERM": "xterm-256color",
	})
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		return nil, fmt.Errorf("pty start: %w", err)
	}
	return &PTYSession{ptmx: ptmx, cmd: cmd}, nil
}

func (s *PTYSession) Read(p []byte) (int, error) { return s.ptmx.Read(p) }

func (s *PTYSession) Write(p []byte) (int, error) { return s.ptmx.Write(p) }

func (s *PTYSession) Resize(cols, rows uint16) error {
	return pty.Setsize(s.ptmx, &pty.Winsize{Cols: cols, Rows: rows})
}

func (s *PTYSession) Wait() int {
	err := s.cmd.Wait()
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

func (s *PTYSession) Close() error { return s.ptmx.Close() }
