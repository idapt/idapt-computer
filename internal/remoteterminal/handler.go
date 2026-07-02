package remoteterminal

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/idapt/idapt-computer/internal/commands"
	"github.com/idapt/idapt-computer/internal/tunnel"
)
func ptyIdleTimeout() time.Duration {
	return envDurationSeconds("IDAPT_PTY_IDLE_TIMEOUT", 30*time.Minute)
}

func ptyMaxLifetime() time.Duration {
	return envDurationSeconds("IDAPT_PTY_MAX_LIFETIME", 8*time.Hour)
}

func maxPTYSessions() int {
	if v := os.Getenv("IDAPT_MAX_PTY_SESSIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 8
}

func envDurationSeconds(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return fallback
}

type Handler struct {
	runuserCfg commands.RunuserConfig
	sem        chan struct{}
}

func New(runuserCfg commands.RunuserConfig) *Handler {
	return &Handler{
		runuserCfg: runuserCfg,
		sem:        make(chan struct{}, maxPTYSessions()),
	}
}

func (h *Handler) HandleStream(stream *tunnel.Stream, header tunnel.StreamHeader) {
	select {
	case h.sem <- struct{}{}:
		defer func() { <-h.sem }()
	default:
		_ = stream.Reject("too many terminal sessions")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), ptyMaxLifetime())
	defer cancel()

	sess, err := commands.StartPTYShell(ctx, header.RunAs, header.Mode, header.Cols, header.Rows, h.runuserCfg)
	if err != nil {
		_ = stream.Reject(err.Error())
		return
	}
	if err := stream.Confirm(); err != nil {
		_ = sess.Close()
		return
	}
	log.Printf("tunnel: pty session opened runAs=%s mode=%q", header.RunAs, header.Mode)

	go func() {
		<-ctx.Done()
		_ = sess.Close()
	}()

	idle := time.AfterFunc(ptyIdleTimeout(), cancel)
	defer idle.Stop()

	go func() {
		for {
			t, payload, rerr := tunnel.ReadPTYFrame(stream)
			if rerr != nil {
				cancel()
				return
			}
			idle.Reset(ptyIdleTimeout())
			switch t {
			case tunnel.PTYFrameData:
				if _, werr := sess.Write(payload); werr != nil {
					cancel()
					return
				}
			case tunnel.PTYFrameResize:
				if cols, rows, ok := tunnel.ParsePTYResize(payload); ok {
					_ = sess.Resize(cols, rows)
				}
			}
		}
	}()

	buf := make([]byte, tunnel.PTYOutputChunk)
	for {
		n, rerr := sess.Read(buf)
		if n > 0 {
			if werr := tunnel.WritePTYData(stream, buf[:n]); werr != nil {
				break
			}
		}
		if rerr != nil {
			break
		}
	}
	cancel()
	exitCode := sess.Wait()
	_ = tunnel.WritePTYExit(stream, int32(exitCode))
	log.Printf("tunnel: pty session closed runAs=%s exit=%d", header.RunAs, exitCode)
}
