package stream

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const MaxAttempts = 5

func backoff(n int) time.Duration {
	steps := []time.Duration{500 * time.Millisecond, 1 * time.Second, 2 * time.Second, 5 * time.Second, 5 * time.Second}
	if n < 1 {
		n = 1
	}
	if n > len(steps) {
		n = len(steps)
	}
	return steps[n-1]
}

func Reconnect(ctx context.Context, p Params, attempt int) tea.Cmd {
	if attempt > MaxAttempts {
		return func() tea.Msg { return ErrMsg{Err: ErrMaxRetries} }
	}
	return func() tea.Msg {
		select {
		case <-time.After(backoff(attempt)):
		case <-ctx.Done():
			return ErrMsg{Err: ctx.Err()}
		}
		return ReconnectingMsg{Attempt: attempt}
	}
}

var ErrMaxRetries = ErrSentinel("sse reconnect: max retries exceeded")

type ErrSentinel string

func (e ErrSentinel) Error() string { return string(e) }
