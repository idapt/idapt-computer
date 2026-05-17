package tui

import (
	"github.com/charmbracelet/bubbles/key"
)

type KeyMap struct {
	Send        key.Binding
	Newline     key.Binding
	NewlineAlt  key.Binding
	Cancel      key.Binding
	Quit        key.Binding
	ClearScreen key.Binding
	NewChat     key.Binding
	ProjectPick key.Binding
	ModelPick   key.Binding
	AgentPick   key.Binding
	Palette     key.Binding
	ScrollUp    key.Binding
	ScrollDown  key.Binding
	Help        key.Binding
	Dismiss     key.Binding
	Copy        key.Binding
}

func DefaultKeyMap() KeyMap {
	return KeyMap{
		Send:        key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "send")),
		Newline:     key.NewBinding(key.WithKeys("alt+enter"), key.WithHelp("alt+enter", "newline")),
		NewlineAlt:  key.NewBinding(key.WithKeys("ctrl+j"), key.WithHelp("ctrl+j", "newline (fallback)")),
		Cancel:      key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "cancel stream / quit empty")),
		Quit:        key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "quit (composer empty)")),
		ClearScreen: key.NewBinding(key.WithKeys("ctrl+l"), key.WithHelp("ctrl+l", "clear transcript")),
		NewChat:     key.NewBinding(key.WithKeys("ctrl+n"), key.WithHelp("ctrl+n", "new chat")),
		ProjectPick: key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp("ctrl+p", "project picker")),
		ModelPick:   key.NewBinding(key.WithKeys("ctrl+m"), key.WithHelp("ctrl+m", "model picker")),
		AgentPick:   key.NewBinding(key.WithKeys("ctrl+g"), key.WithHelp("ctrl+g", "agent picker")),
		Palette:     key.NewBinding(key.WithKeys("ctrl+k"), key.WithHelp("ctrl+k", "command palette")),
		ScrollUp:    key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "scroll up")),
		ScrollDown:  key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdown", "scroll down")),
		Help:        key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help overlay")),
		Dismiss:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "dismiss modal / cancel")),
		Copy:        key.NewBinding(key.WithKeys("ctrl+y"), key.WithHelp("ctrl+y", "copy last reply")),
	}
}

func (k KeyMap) AllBindings() []key.Binding {
	return []key.Binding{
		k.Send, k.Newline, k.NewlineAlt,
		k.Cancel, k.Quit, k.ClearScreen,
		k.NewChat, k.ProjectPick, k.ModelPick, k.AgentPick,
		k.Palette, k.ScrollUp, k.ScrollDown,
		k.Help, k.Dismiss, k.Copy,
	}
}
