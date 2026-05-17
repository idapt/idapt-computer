package widgets

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

type HeaderState struct {
	Project string
	Agent   string
	Model   string
}

type HeaderStyles struct {
	Title lipgloss.Style
	Meta  lipgloss.Style
	Bar   lipgloss.Style
}

type Header struct {
	state  HeaderState
	width  int
	styles HeaderStyles
}

func NewHeader(s HeaderStyles) Header { return Header{styles: s} }

func (h *Header) SetStyles(s HeaderStyles) { h.styles = s }

func (h *Header) SetState(s HeaderState) { h.state = s }

func (h *Header) SetSize(w int) { h.width = w }

func (h Header) View() string {
	project := h.state.Project
	if project == "" {
		project = "(no project)"
	}
	model := h.state.Model
	if model == "" {
		model = "(no model)"
	}
	agent := h.state.Agent
	if agent == "" {
		agent = "(no agent)"
	}
	sep := h.styles.Meta.Render(" · ")
	parts := []string{
		h.styles.Title.Render("idapt"),
		h.styles.Meta.Render(project),
		h.styles.Meta.Render(model),
		h.styles.Meta.Render("agent: " + agent),
	}
	line := strings.Join(parts, sep)
	if h.width > 0 && runewidth.StringWidth(line) > h.width {
		line = runewidth.Truncate(line, h.width, "…")
	}
	return h.styles.Bar.Render(line)
}
