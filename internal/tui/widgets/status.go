package widgets

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

type StatusKind int

const (
	StatusIdle StatusKind = iota
	StatusStreaming
	StatusReconnecting
	StatusWarn
	StatusError
)

type StatusState struct {
	Tokens  int
	Budget  int
	Cost    float64
	Message string
	Kind    StatusKind
	Hint    string
}

type StatusStyles struct {
	Bar         lipgloss.Style
	OK          lipgloss.Style
	Streaming   lipgloss.Style
	Warn        lipgloss.Style
	Error       lipgloss.Style
	HintStyle   lipgloss.Style
	MutedStyle  lipgloss.Style
}

type Status struct {
	state  StatusState
	width  int
	styles StatusStyles
}

func NewStatus(s StatusStyles) Status { return Status{styles: s} }

func (s *Status) SetStyles(st StatusStyles) { s.styles = st }

func (s *Status) SetState(st StatusState) { s.state = st }

func (s *Status) SetSize(w int) { s.width = w }

func (s Status) View() string {
	left := s.left()
	right := s.right()
	pad := s.width - runewidth.StringWidth(left) - runewidth.StringWidth(right)
	if pad < 1 {
		pad = 1
	}
	return s.styles.Bar.Render(left + strings.Repeat(" ", pad) + right)
}

func (s Status) left() string {
	var parts []string
	if s.state.Tokens > 0 {
		if s.state.Budget > 0 {
			parts = append(parts, fmt.Sprintf("%d/%d tok", s.state.Tokens, s.state.Budget))
		} else {
			parts = append(parts, fmt.Sprintf("%d tok", s.state.Tokens))
		}
	}
	if s.state.Cost > 0 {
		parts = append(parts, fmt.Sprintf("$%.4f", s.state.Cost))
	}
	if len(parts) == 0 {
		return ""
	}
	return s.styles.MutedStyle.Render(strings.Join(parts, " · "))
}

func (s Status) right() string {
	msg := s.state.Message
	switch s.state.Kind {
	case StatusStreaming:
		msg = s.styles.Streaming.Render(msg)
	case StatusReconnecting:
		msg = s.styles.Warn.Render(msg)
	case StatusWarn:
		msg = s.styles.Warn.Render(msg)
	case StatusError:
		msg = s.styles.Error.Render(msg)
	case StatusIdle:
		msg = s.styles.OK.Render(msg)
	}
	hint := s.state.Hint
	if hint != "" {
		hint = s.styles.HintStyle.Render(hint)
	}
	if msg == "" && hint == "" {
		return ""
	}
	if msg == "" {
		return hint
	}
	if hint == "" {
		return msg
	}
	return msg + s.styles.MutedStyle.Render(" · ") + hint
}
