package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/idapt/idapt-cli/internal/tui/widgets"
)

func (m Model) View() string {
	if m.width < 40 {
		return m.tooNarrow()
	}

	base := m.renderBase()

	if m.state == viewPicker && m.picker.Visible() {
		return overlayCenter(dimBackground(base, m.width, m.height), m.picker.View(), m.width, m.height)
	}
	return base
}

func (m Model) renderBase() string {
	sections := []string{
		m.header.View(),
		m.transcript.View(),
	}
	if m.suggest.Visible() {
		sections = append(sections, m.suggest.View())
	}
	sections = append(sections, m.composer.View())
	if tb := m.toolbar.View(); tb != "" {
		sections = append(sections, tb)
	}
	sections = append(sections, m.status.View())
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func defaultToolbarButtons(streaming bool) []widgets.Button {
	if streaming {
		return []widgets.Button{
			{ID: "stop", Label: "■ Stop", Kind: widgets.BtnDanger},
			{ID: "menu", Label: "☰ Menu", Kind: widgets.BtnPrimary},
			{ID: "help", Label: "? Help"},
			{ID: "quit", Label: "✕ Quit"},
		}
	}
	return []widgets.Button{
		{ID: "menu", Label: "☰ Menu", Kind: widgets.BtnPrimary},
		{ID: "new", Label: "+ New"},
		{ID: "help", Label: "? Help"},
		{ID: "model", Label: "✦ Model"},
		{ID: "agent", Label: "👤 Agent"},
		{ID: "project", Label: "📁 Project"},
		{ID: "quit", Label: "✕ Quit"},
	}
}

func (m Model) tooNarrow() string {
	return m.theme.Error.Render("Terminal too narrow — resize to ≥ 40 columns")
}
