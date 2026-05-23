package widgets

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/idapt/idapt-cli/internal/tui/markdown"
)

type Role int

const (
	RoleUser Role = iota
	RoleAssistant
	RoleSystem
	RoleError
)

type Message struct {
	ID        string
	Role      Role
	Body      string
	Streaming bool
	Cancelled bool
	Cost      float64
	Tokens    int
}

type TranscriptStyles struct {
	User      lipgloss.Style
	Assistant lipgloss.Style
	System    lipgloss.Style
	Error     lipgloss.Style
	Muted     lipgloss.Style
	NoColor   bool
}

type Transcript struct {
	vp       viewport.Model
	messages []Message
	width    int
	height   int
	styles   TranscriptStyles
	pinBot   bool
}

func NewTranscript(s TranscriptStyles) Transcript {
	vp := viewport.New(80, 20)
	return Transcript{vp: vp, styles: s, pinBot: true}
}

func (t *Transcript) SetStyles(s TranscriptStyles) {
	t.styles = s
	t.rerender()
}

func (t *Transcript) SetSize(w, h int) {
	t.width = w
	t.height = h
	t.vp.Width = w
	t.vp.Height = h
	t.rerender()
}

func (t *Transcript) Append(m Message) {
	t.messages = append(t.messages, m)
	t.rerender()
	if t.pinBot {
		t.vp.GotoBottom()
	}
}

func (t *Transcript) BeginStreaming(id string) {
	t.messages = append(t.messages, Message{ID: id, Role: RoleAssistant, Streaming: true})
	t.rerender()
	if t.pinBot {
		t.vp.GotoBottom()
	}
}

func (t *Transcript) UpdateStreaming(id, body string) {
	for i := range t.messages {
		if t.messages[i].ID == id {
			t.messages[i].Body = body
			break
		}
	}
	t.rerender()
	if t.pinBot {
		t.vp.GotoBottom()
	}
}

func (t *Transcript) Finalize(id string, cost float64, tokens int) {
	for i := range t.messages {
		if t.messages[i].ID == id {
			t.messages[i].Streaming = false
			t.messages[i].Cost = cost
			t.messages[i].Tokens = tokens
			break
		}
	}
	t.rerender()
}

func (t *Transcript) MarkCancelled(id string) {
	for i := range t.messages {
		if t.messages[i].ID == id {
			t.messages[i].Streaming = false
			t.messages[i].Cancelled = true
			break
		}
	}
	t.rerender()
}

func (t *Transcript) AppendError(text string) {
	t.Append(Message{Role: RoleError, Body: text})
}

func (t *Transcript) Clear() {
	t.messages = nil
	t.rerender()
}

func (t *Transcript) LastAssistant() *Message {
	for i := len(t.messages) - 1; i >= 0; i-- {
		if t.messages[i].Role == RoleAssistant {
			return &t.messages[i]
		}
	}
	return nil
}

func (t *Transcript) LastUser() *Message {
	for i := len(t.messages) - 1; i >= 0; i-- {
		if t.messages[i].Role == RoleUser {
			return &t.messages[i]
		}
	}
	return nil
}

func (t *Transcript) Update(msg tea.Msg) (Transcript, tea.Cmd) {
	var cmd tea.Cmd
	t.vp, cmd = t.vp.Update(msg)
	t.pinBot = t.vp.AtBottom()
	return *t, cmd
}

func (t Transcript) View() string {
	return t.vp.View()
}

func (t Transcript) Messages() []Message {
	out := make([]Message, len(t.messages))
	copy(out, t.messages)
	return out
}

func (t *Transcript) rerender() {
	if t.width == 0 {
		t.vp.SetContent("")
		return
	}
	var b strings.Builder
	for i, m := range t.messages {
		label := t.label(m)
		b.WriteString(label)
		b.WriteString("\n")
		body := m.Body
		switch m.Role {
		case RoleAssistant:
			if m.Streaming {
				body = markdown.RenderStreaming(body, t.width, t.styles.NoColor)
			} else {
				body = markdown.RenderFinal(body, t.width, t.styles.NoColor)
			}
		case RoleUser:
			body = "  " + strings.ReplaceAll(body, "\n", "\n  ")
		case RoleSystem:
			body = t.styles.Muted.Render("  " + body)
		case RoleError:
			indented := "  " + strings.ReplaceAll(body, "\n", "\n  ")
			body = t.styles.Error.Render("  ✖ " + strings.TrimPrefix(indented, "  "))
		}
		b.WriteString(strings.TrimRight(body, "\n"))
		if m.Cancelled {
			b.WriteString("\n  ")
			b.WriteString(t.styles.Muted.Render("(cancelled)"))
		}
		if !m.Streaming && (m.Cost > 0 || m.Tokens > 0) {
			b.WriteString("\n  ")
			b.WriteString(t.styles.Muted.Render(footer(m.Cost, m.Tokens)))
		}
		if i < len(t.messages)-1 {
			b.WriteString("\n\n")
		}
	}
	t.vp.SetContent(b.String())
}

func (t Transcript) label(m Message) string {
	switch m.Role {
	case RoleUser:
		return t.styles.User.Render("You")
	case RoleAssistant:
		s := "Assistant"
		if m.Streaming {
			s = "Assistant ▎"
		}
		return t.styles.Assistant.Render(s)
	case RoleSystem:
		return t.styles.System.Render("system")
	case RoleError:
		return t.styles.Error.Render("✖ Error")
	}
	return ""
}

func footer(cost float64, tokens int) string {
	switch {
	case cost > 0 && tokens > 0:
		return sprintfMoney("$%.4f · %d tok", cost, tokens)
	case cost > 0:
		return sprintfMoney("$%.4f", cost)
	case tokens > 0:
		return sprintfMoney("%d tok", tokens)
	}
	return ""
}
