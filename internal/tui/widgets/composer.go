package widgets

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type FileAttachment struct {
	Path string
	ID   string // empty until upload completes
}

const (
	minComposerRows = 1
	maxComposerRows = 8
)

type ComposerStyles struct {
	FileChip lipgloss.Style
	Hint     lipgloss.Style
	Border   lipgloss.Style
	Prompt   lipgloss.Style
	BgSoft   lipgloss.Style // currently unused, reserved for highlight backdrops
}

type Composer struct {
	area     textarea.Model
	files    []FileAttachment
	disabled bool
	width    int

	styles       ComposerStyles
	disabledHint string

	recognizedCommand string
}

func NewComposer(s ComposerStyles) Composer {
	ta := textarea.New()
	ta.Placeholder = "Type a message, or / for commands"
	ta.Prompt = "  "
	ta.CharLimit = 0
	ta.ShowLineNumbers = false
	ta.SetHeight(1)
	clear := lipgloss.NewStyle()
	for _, style := range []*textarea.Style{&ta.FocusedStyle, &ta.BlurredStyle} {
		style.Base = clear
		style.CursorLine = clear
		style.CursorLineNumber = clear
		style.EndOfBuffer = clear
		style.LineNumber = clear
		style.Text = clear
	}
	ta.KeyMap.InsertNewline = key.NewBinding(
		key.WithKeys("ctrl+j"),
		key.WithHelp("ctrl+j / shift+enter / ctrl+enter", "newline"),
	)
	ta.Focus()
	return Composer{
		area:   ta,
		styles: s,
	}
}

func (c *Composer) SetStyles(s ComposerStyles) { c.styles = s }

func (c *Composer) Focus() tea.Cmd { return c.area.Focus() }

func (c *Composer) Blur() { c.area.Blur() }

func (c *Composer) Value() string { return c.area.Value() }

func (c *Composer) CursorByteOffset() int {
	val := c.area.Value()
	lineIdx := c.area.Line()
	info := c.area.LineInfo()
	col := info.StartColumn + info.ColumnOffset

	off := 0
	lines := strings.SplitAfter(val, "\n") // keeps the \n on each line
	for i := 0; i < lineIdx && i < len(lines); i++ {
		off += len(lines[i])
	}
	if lineIdx < len(lines) {
		line := strings.TrimSuffix(lines[lineIdx], "\n")
		runeCount := 0
		for byteIdx := range line {
			if runeCount == col {
				off += byteIdx
				return off
			}
			runeCount++
		}
		off += len(line)
	}
	return off
}

func (c *Composer) Reset() {
	c.area.Reset()
	c.files = nil
}

func (c *Composer) ResetText() {
	c.area.Reset()
}

func (c *Composer) SetValue(v string) { c.area.SetValue(v) }

func (c *Composer) SetValueAt(v string, byteOffset int) {
	c.area.SetValue(v)
	if byteOffset < 0 || byteOffset > len(v) {
		return
	}
	before := v[:byteOffset]
	line := strings.Count(before, "\n")
	lineStart := strings.LastIndex(before, "\n") + 1
	col := 0
	for range before[lineStart:] {
		col++
	}
	c.area.CursorStart()
	for i := 0; i < line; i++ {
		c.area.CursorDown()
	}
	c.area.SetCursor(col)
}

func (c *Composer) SetSize(w, h int) {
	c.width = w
	c.area.SetWidth(w - 2)
	c.syncHeight()
	_ = h
}

func (c *Composer) syncHeight() {
	val := c.area.Value()
	lines := strings.Count(val, "\n") + 1
	if lines < minComposerRows {
		lines = minComposerRows
	}
	if lines > maxComposerRows {
		lines = maxComposerRows
	}
	c.area.SetHeight(lines)
}

func (c *Composer) InsertNewline() {
	buf := c.area.Value()
	cur := c.CursorByteOffset()
	if cur < 0 {
		cur = 0
	}
	if cur > len(buf) {
		cur = len(buf)
	}
	next := buf[:cur] + "\n" + buf[cur:]
	c.SetValueAt(next, cur+1)
	c.syncHeight()
}

func (c *Composer) AttachFile(path string) {
	for _, f := range c.files {
		if f.Path == path {
			return
		}
	}
	c.files = append(c.files, FileAttachment{Path: path})
}

func (c *Composer) DetachFile(path string) bool {
	for i, f := range c.files {
		if f.Path == path {
			c.files = append(c.files[:i], c.files[i+1:]...)
			return true
		}
	}
	return false
}

func (c *Composer) Files() []FileAttachment {
	out := make([]FileAttachment, len(c.files))
	copy(out, c.files)
	return out
}

func (c *Composer) ClearFiles() { c.files = nil }

func (c *Composer) SetFileID(path, id string) bool {
	for i := range c.files {
		if c.files[i].Path == path {
			c.files[i].ID = id
			return true
		}
	}
	return false
}

func (c *Composer) SetDisabled(d bool) {
	c.disabled = d
	if d {
		c.disabledHint = "Streaming… (Ctrl+C to cancel)"
	} else {
		c.disabledHint = ""
	}
}

func (c *Composer) Disabled() bool { return c.disabled }

func (c *Composer) SetRecognizedCommand(verb string) { c.recognizedCommand = verb }

func (c *Composer) NewlineBindingKeys() []string { return c.area.KeyMap.InsertNewline.Keys() }

func (c *Composer) RecognizedCommand() string { return c.recognizedCommand }

func (c *Composer) Update(msg tea.Msg) (Composer, tea.Cmd) {
	var cmd tea.Cmd
	c.area, cmd = c.area.Update(msg)
	c.syncHeight()
	return *c, cmd
}

func (c Composer) View() string {
	var sections []string
	if len(c.files) > 0 {
		chips := make([]string, 0, len(c.files))
		for _, f := range c.files {
			label := "📎 " + f.Path
			if f.ID == "" {
				label += " (uploading…)"
			}
			chips = append(chips, c.styles.FileChip.Render(label))
		}
		sections = append(sections, strings.Join(chips, "  "))
	}
	promptSymbol := ">"
	if c.recognizedCommand != "" {
		promptSymbol = "▸"
	}
	prompt := c.styles.Prompt.Render(promptSymbol)
	body := c.area.View()
	if strings.HasPrefix(body, "  ") {
		body = prompt + body[1:]
	}
	borderFg := c.styles.Border.GetForeground()
	if c.recognizedCommand != "" {
		borderFg = c.styles.Prompt.GetForeground()
	}
	rule := lipgloss.NormalBorder()
	box := lipgloss.NewStyle().
		BorderStyle(rule).
		BorderTop(true).
		BorderBottom(true).
		BorderLeft(false).
		BorderRight(false).
		BorderForeground(borderFg).
		Width(c.width).
		Render(body)
	sections = append(sections, box)
	if c.disabledHint != "" {
		sections = append(sections, c.styles.Hint.Render(c.disabledHint))
	}
	return strings.Join(sections, "\n")
}

func (c Composer) HelpHint() string {
	if c.disabled {
		return "Ctrl+C cancel · Esc dismiss"
	}
	if len(c.files) > 0 {
		return fmt.Sprintf("%d file(s) attached · /files to list", len(c.files))
	}
	return "Enter send · Shift+Enter newline · / for commands"
}
