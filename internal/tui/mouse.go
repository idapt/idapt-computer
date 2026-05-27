package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		t, cmd := m.transcript.Update(msg)
		m.transcript = t
		return m, cmd
	}

	if msg.Action != tea.MouseActionPress {
		return m, nil
	}

	if m.state == viewPicker && m.picker.Visible() {
		if !m.pointInsidePicker(msg.X, msg.Y) {
			m.state = viewChat
			m.picker.Close()
			m.modelPickerForceRefetch = false
			return m, m.composer.Focus()
		}
		return m, nil
	}

	rows := m.height
	if rows < 5 {
		return m, nil
	}
	toolbarRow := rows - 2
	composerTop := rows - 2 - composerHeight()

	if msg.Y == toolbarRow {
		btns := defaultToolbarButtons(m.streaming)
		m.toolbar.SetWidth(m.width)
		m.toolbar.SetButtons(btns)
		_ = m.toolbar.View() // populates hit-map as a side effect
		if id := m.toolbar.HitTest(msg.X); id >= 0 {
			if id < len(btns) {
				return m.dispatchToolbarAction(btns[id].ID)
			}
		}
		return m, nil
	}

	if m.suggest.Visible() {
		items := m.suggest.Items()
		popupHeight := len(items) + 2 // +2 for the rounded border lines
		popupTop := composerTop - popupHeight
		if msg.Y >= popupTop && msg.Y < composerTop {
			if idx := m.suggest.HitTest(msg.X, msg.Y, 0, popupTop); idx >= 0 {
				m.suggest.SelectIndex(idx)
				m.applySuggestionCompletion(false)
				m.suggest.Close()
				return m.submitComposer()
			}
			return m, nil
		}
	}

	return m, nil
}

func composerHeight() int { return 3 }

func (m Model) pointInsidePicker(x, y int) bool {
	v := m.picker.View()
	if v == "" {
		return false
	}
	lines := strings.Split(v, "\n")
	if len(lines) == 0 {
		return false
	}
	maxW := 0
	for _, ln := range lines {
		if w := visualWidth(ln); w > maxW {
			maxW = w
		}
	}
	startRow := (m.height - len(lines)) / 2
	if startRow < 0 {
		startRow = 0
	}
	startCol := (m.width - maxW) / 2
	if startCol < 0 {
		startCol = 0
	}
	return x >= startCol && x < startCol+maxW &&
		y >= startRow && y < startRow+len(lines)
}

func visualWidth(s string) int {
	w := 0
	i := 0
	for i < len(s) {
		c := s[i]
		if c == 0x1b {
			j := i + 1
			if j < len(s) && (s[j] == '[' || s[j] == ']') {
				j++
				for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
					j++
				}
				if j < len(s) {
					j++
				}
			} else if j < len(s) {
				j++
			}
			i = j
			continue
		}
		r, size := decodeRune(s[i:])
		w += runeCellWidth(r)
		i += size
	}
	return w
}

func (m Model) dispatchToolbarAction(id string) (tea.Model, tea.Cmd) {
	switch id {
	case "menu":
		return m.dispatchSlashByVerb("menu", nil)
	case "new":
		return m.dispatchSlashByVerb("new", nil)
	case "help":
		return m.dispatchSlashByVerb("help", nil)
	case "model":
		return m.dispatchSlashByVerb("model", nil)
	case "agent":
		return m.dispatchSlashByVerb("agent", nil)
	case "workspace":
		return m.dispatchSlashByVerb("workspace", nil)
	case "theme":
		return m.dispatchSlashByVerb("theme", nil)
	case "stop":
		if m.streaming {
			m.cancelStream()
		}
		return m, nil
	case "quit":
		if m.streaming {
			m.cancelStream()
		}
		return m, m.quitAfterPersist()
	}
	return m, nil
}
