package widgets

import (
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type PickerItem struct {
	ID       string
	Label    string
	Subtitle string
	Locked   bool   // dimmed, unclickable
	Reason   string // optional reason rendered next to a locked label
}

func (p PickerItem) FilterValue() string { return p.Label + " " + p.Subtitle }

type pickerItemDelegate struct {
	styles PickerStyles
}

func (d pickerItemDelegate) Height() int                             { return 1 }
func (d pickerItemDelegate) Spacing() int                            { return 0 }
func (d pickerItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d pickerItemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	it, ok := listItem.(PickerItem)
	if !ok {
		return
	}
	label := it.Label
	if it.Subtitle != "" {
		label = label + "  " + d.styles.Item.Render(d.muted(it.Subtitle))
	}
	if it.Locked {
		suffix := ""
		if it.Reason != "" {
			suffix = "  — " + it.Reason
		}
		label = d.styles.Locked.Render(it.Label + suffix)
	} else if index == m.Index() {
		label = d.styles.Selected.Render("› " + label)
	} else {
		label = "  " + label
	}
	_, _ = io.WriteString(w, label)
}

func (d pickerItemDelegate) muted(s string) string {
	return lipgloss.NewStyle().Faint(true).Render(s)
}

type PickerStyles struct {
	Border   lipgloss.Style
	Selected lipgloss.Style
	Item     lipgloss.Style
	Locked   lipgloss.Style
	Filter   lipgloss.Style
}

type Picker struct {
	title    string
	list     list.Model
	filter   textinput.Model
	allItems []PickerItem // canonical list; visible subset filtered on input
	styles   PickerStyles
	visible  bool
	width    int
	height   int
}

func (p *Picker) SetStyles(s PickerStyles) {
	p.styles = s
	p.list.SetDelegate(pickerItemDelegate{styles: s})
}

func (p *Picker) SetTitle(title string) { p.title = title }

func NewPicker(title string, s PickerStyles) Picker {
	l := list.New(nil, pickerItemDelegate{styles: s}, 50, 10)
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowTitle(false)
	l.SetShowPagination(false) // we render our own footer; paging eats rows
	l.SetFilteringEnabled(false)
	l.DisableQuitKeybindings()

	ti := textinput.New()
	ti.Placeholder = "filter…"
	ti.CharLimit = 64

	return Picker{title: title, list: l, filter: ti, styles: s}
}

func (p *Picker) ShrinkListToContent(n int) {
	if n <= 0 {
		return
	}
	if n < p.list.Height() {
		p.list.SetHeight(n)
	}
}

func (p *Picker) Open(items []PickerItem) {
	p.allItems = items
	li := make([]list.Item, 0, len(items))
	for _, it := range items {
		li = append(li, it)
	}
	p.list.SetItems(li)
	n := len(items)
	if n > 0 && n < p.list.Height() {
		p.list.SetHeight(n)
	}
	p.filter.SetValue("")
	p.filter.Focus()
	p.visible = true
}

func (p *Picker) Close() {
	p.visible = false
	p.filter.Blur()
}

func (p Picker) Visible() bool { return p.visible }

func (p Picker) Items() []PickerItem {
	out := make([]PickerItem, len(p.allItems))
	copy(out, p.allItems)
	return out
}

func (p Picker) Selected() *PickerItem {
	if !p.visible || len(p.list.Items()) == 0 {
		return nil
	}
	it, ok := p.list.Items()[p.list.Index()].(PickerItem)
	if !ok {
		return nil
	}
	if it.Locked {
		return nil
	}
	return &it
}

func (p *Picker) SetSize(w, h int) {
	p.width = w
	p.height = h
	listH := h - 7
	if listH < 3 {
		listH = 3
	}
	p.list.SetSize(w-4, listH)
}

func (p *Picker) Update(msg tea.Msg) (Picker, tea.Cmd) {
	if !p.visible {
		return *p, nil
	}
	var cmd tea.Cmd
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "up", "down", "pgup", "pgdown", "home", "end":
			p.list, cmd = p.list.Update(msg)
			return *p, cmd
		}
	}
	p.filter, cmd = p.filter.Update(msg)
	p.applyFilter()
	return *p, cmd
}

func (p *Picker) applyFilter() {
	q := strings.ToLower(strings.TrimSpace(p.filter.Value()))
	all := p.allItems
	if q == "" {
		if len(all) > 0 {
			items := make([]list.Item, 0, len(all))
			for _, it := range all {
				items = append(items, it)
			}
			p.list.SetItems(items)
		}
		return
	}
	filtered := make([]list.Item, 0)
	for _, it := range all {
		if strings.Contains(strings.ToLower(it.Label), q) || strings.Contains(strings.ToLower(it.Subtitle), q) {
			filtered = append(filtered, it)
		}
	}
	p.list.SetItems(filtered)
}

func (p Picker) View() string {
	if !p.visible {
		return ""
	}
	title := lipgloss.NewStyle().Bold(true).Render(p.title)
	filt := p.styles.Filter.Render(p.filter.View())
	btnSelect := lipgloss.NewStyle().
		Bold(true).
		Foreground(p.styles.Selected.GetForeground()).
		Render("[ ↵  Enter · Select ]")
	btnClose := lipgloss.NewStyle().
		Foreground(p.styles.Border.GetForeground()).
		Render("[ ✕  Esc · Close ]")
	footer := btnSelect + "   " + btnClose
	body := strings.Join([]string{
		title,
		filt,
		p.list.View(),
		footer,
	}, "\n")
	return p.styles.Border.
		BorderStyle(lipgloss.RoundedBorder()).
		Padding(1, 2).
		Render(body)
}
