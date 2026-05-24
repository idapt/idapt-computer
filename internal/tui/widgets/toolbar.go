package widgets

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type ToolbarStyles struct {
	Idle    lipgloss.Style
	Hover   lipgloss.Style
	Primary lipgloss.Style
	Danger  lipgloss.Style
	Bar     lipgloss.Style
}

type ButtonKind int

const (
	BtnIdle ButtonKind = iota
	BtnPrimary
	BtnDanger
)

type Button struct {
	ID    string     // action id (e.g. "new", "help", "theme", "stop", "quit")
	Label string     // visible text e.g. "Stop"
	Kind  ButtonKind // visual treatment
}

type Toolbar struct {
	buttons    []Button
	styles     ToolbarStyles
	hoverIndex int // -1 when no hover
	width      int

	hitMap []hitSpan
}

type hitSpan struct {
	start, end int
	index      int
}

func NewToolbar(s ToolbarStyles) Toolbar { return Toolbar{styles: s, hoverIndex: -1} }

func (t *Toolbar) SetStyles(s ToolbarStyles) { t.styles = s }

func (t *Toolbar) SetButtons(b []Button) {
	t.buttons = b
	if t.hoverIndex >= len(b) {
		t.hoverIndex = -1
	}
}

func (t *Toolbar) SetWidth(w int) { t.width = w }

func (t *Toolbar) SetHover(i int) { t.hoverIndex = i }

func (t Toolbar) Buttons() []Button {
	out := make([]Button, len(t.buttons))
	copy(out, t.buttons)
	return out
}

func (t Toolbar) HitTest(x int) int {
	for _, h := range t.hitMap {
		if x >= h.start && x < h.end {
			return h.index
		}
	}
	return -1
}

func (t *Toolbar) View() string {
	if len(t.buttons) == 0 {
		return ""
	}
	t.hitMap = t.hitMap[:0]
	var parts []string
	col := 0
	for i, b := range t.buttons {
		label := "[ " + b.Label + " ]"
		var st lipgloss.Style
		switch {
		case i == t.hoverIndex:
			st = t.styles.Hover
		case b.Kind == BtnPrimary:
			st = t.styles.Primary
		case b.Kind == BtnDanger:
			st = t.styles.Danger
		default:
			st = t.styles.Idle
		}
		rendered := st.Render(label)
		span := hitSpan{start: col, end: col + runeCount(label), index: i}
		t.hitMap = append(t.hitMap, span)
		col = span.end + 1 // single-space separator
		parts = append(parts, rendered)
	}
	line := strings.Join(parts, " ")
	if t.width > 0 {
		visible := col - 1
		if visible < t.width {
			line += strings.Repeat(" ", t.width-visible)
		}
	}
	return t.styles.Bar.Render(line)
}

func runeCount(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}
