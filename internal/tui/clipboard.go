package tui

import (
	"encoding/base64"

	tea "github.com/charmbracelet/bubbletea"
)

const osc52MaxBytes = 100 * 1024

func copyToClipboardCmd(s string) tea.Cmd {
	if len(s) > osc52MaxBytes {
		s = s[:osc52MaxBytes]
	}
	enc := base64.StdEncoding.EncodeToString([]byte(s))
	seq := "\033]52;c;" + enc + "\a"
	return func() tea.Msg {
		return tea.Printf("%s", seq)()
	}
}
