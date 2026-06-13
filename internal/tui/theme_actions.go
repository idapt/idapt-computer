package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/idapt/idapt-cli/internal/tui/widgets"
)

func (m Model) handleThemeSlash(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		return m.openPicker(pickerThemeArgs)
	}
	arg := strings.ToLower(strings.TrimSpace(args[0]))
	switch arg {
	case "auto", "light", "dark":
		next := ParseThemeMode(arg)
		m.applyTheme(next)
		m.transcript.Append(widgets.Message{
			Role: widgets.RoleSystem,
			Body: "theme: " + next.String(),
		})
		return m, m.persistContext()
	default:
		m.transcript.AppendError(fmt.Sprintf("theme must be auto|light|dark, got %q", args[0]))
		return m, nil
	}
}

func (m *Model) applyTheme(mode ThemeMode) {
	m.theme = NewTheme(mode, m.noColor)
	m.cfg.Theme = mode.String()

	switch mode {
	case ThemeLight:
		lipgloss.SetHasDarkBackground(false)
	case ThemeDark:
		lipgloss.SetHasDarkBackground(true)
	}

	m.composer = m.restyleComposer(m.composer)
	m.transcript = m.restyleTranscript(m.transcript)
	m.header = m.restyleHeader(m.header)
	m.status = m.restyleStatus(m.status)
	m.picker = m.restylePicker(m.picker)
	m.suggest = m.restyleSuggest(m.suggest)
	m.toolbar = m.restyleToolbar(m.toolbar)
}

func (m Model) restyleSuggest(s widgets.Suggest) widgets.Suggest {
	s.SetStyles(widgets.SuggestStyles{
		Border:   m.theme.SuggestBorder,
		Selected: m.theme.SuggestSelected,
		Item:     m.theme.SuggestItem,
		Hint:     m.theme.SuggestHint,
	})
	return s
}

func (m Model) restyleToolbar(t widgets.Toolbar) widgets.Toolbar {
	t.SetStyles(widgets.ToolbarStyles{
		Idle:    m.theme.ButtonIdle,
		Hover:   m.theme.ButtonHover,
		Primary: m.theme.ButtonPrimary,
		Danger:  m.theme.ButtonDanger,
		Bar:     m.theme.StatusBar,
	})
	return t
}

func (m Model) restyleComposer(c widgets.Composer) widgets.Composer {
	c.SetStyles(widgets.ComposerStyles{
		FileChip: m.theme.FileChip,
		Hint:     m.theme.Hint,
		Border:   m.theme.ComposerBorder,
		Prompt:   m.theme.ComposerPrompt,
		BgSoft:   m.theme.Muted,
	})
	return c
}

func (m Model) restyleTranscript(t widgets.Transcript) widgets.Transcript {
	t.SetStyles(widgets.TranscriptStyles{
		User:      m.theme.UserLabel,
		Assistant: m.theme.AssistantLabel,
		System:    m.theme.SystemLabel,
		Error:     m.theme.Error,
		Muted:     m.theme.Muted,
		NoColor:   m.noColor,
	})
	return t
}

func (m Model) restyleHeader(h widgets.Header) widgets.Header {
	h.SetStyles(widgets.HeaderStyles{
		Title: m.theme.HeaderTitle,
		Meta:  m.theme.HeaderMeta,
		Bar:   m.theme.HeaderBar,
	})
	return h
}

func (m Model) restyleStatus(s widgets.Status) widgets.Status {
	s.SetStyles(widgets.StatusStyles{
		Bar:        m.theme.StatusBar,
		OK:         m.theme.StatusOK,
		Streaming:  m.theme.StatusStreaming,
		Warn:       m.theme.StatusWarn,
		Error:      m.theme.StatusError,
		HintStyle:  m.theme.Hint,
		MutedStyle: m.theme.Muted,
	})
	return s
}

func (m Model) restylePicker(p widgets.Picker) widgets.Picker {
	p.SetStyles(widgets.PickerStyles{
		Border:   m.theme.PickerBorder,
		Selected: m.theme.PickerSelected,
		Item:     m.theme.PickerItem,
		Locked:   m.theme.PickerLocked,
		Filter:   m.theme.PickerFilter,
	})
	return p
}
