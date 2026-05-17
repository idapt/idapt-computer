package widgets

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/idapt/idapt-cli/internal/tui/commands"
)

type SuggestStyles struct {
	Border   lipgloss.Style
	Selected lipgloss.Style
	Item     lipgloss.Style
	Hint     lipgloss.Style
}

type SlashWord struct {
	Start   int    // byte index of the leading '/'
	End     int    // byte index one past the last char of the slash word
	Word    string // verb-portion as typed, e.g. "hel" (excludes leading '/')
	Matches bool   // true when Word case-insensitively matches a registry entry exactly
}

type Suggest struct {
	items   []commands.VerbSpec
	index   int
	word    SlashWord // captured at the last UpdateFromBuffer call
	styles  SuggestStyles
	maxRows int
	visible bool
}

func NewSuggest(s SuggestStyles) Suggest {
	return Suggest{styles: s, maxRows: 14}
}

func (s *Suggest) SetStyles(st SuggestStyles) { s.styles = st }

func (s Suggest) Visible() bool { return s.visible }

func (s Suggest) Items() []commands.VerbSpec { return s.items }

func (s Suggest) CurrentWord() SlashWord { return s.word }

func (s Suggest) Selected() *commands.VerbSpec {
	if !s.visible || len(s.items) == 0 {
		return nil
	}
	v := s.items[s.index]
	return &v
}

func (s *Suggest) UpdateFromBuffer(buf string, cursor int) bool {
	prev := s.visible
	s.word = detectSlashWord(buf, cursor)

	if s.word.Start < 0 {
		s.hide()
		return prev != s.visible
	}
	if s.word.Matches {
		s.hide()
		return prev != s.visible
	}

	s.items = filterVerbs(s.word.Word)
	if len(s.items) == 0 {
		s.hide()
		return prev != s.visible
	}
	s.visible = true
	if s.index >= len(s.items) {
		s.index = 0
	}
	return prev != s.visible
}

func (s *Suggest) hide() {
	s.visible = false
	s.items = nil
	s.index = 0
}

func (s *Suggest) Next() {
	if !s.visible || len(s.items) == 0 {
		return
	}
	s.index = (s.index + 1) % len(s.items)
}

func (s *Suggest) Prev() {
	if !s.visible || len(s.items) == 0 {
		return
	}
	s.index--
	if s.index < 0 {
		s.index = len(s.items) - 1
	}
}

func (s *Suggest) SelectIndex(i int) {
	if !s.visible || i < 0 || i >= len(s.items) {
		return
	}
	s.index = i
}

func (s Suggest) HitTest(x, y, originX, originY int) int {
	if !s.visible {
		return -1
	}
	relY := y - originY - 1 // -1 to account for the top border row
	if relY < 0 || relY >= len(s.items) {
		return -1
	}
	if x < originX || x >= originX+s.Width() {
		return -1
	}
	return relY
}

func (s Suggest) Width() int {
	w := 0
	for _, it := range s.items {
		l := len(it.Name) + len(it.ArgsHint) + len(it.Short) + 8
		if l > w {
			w = l
		}
	}
	if w < 32 {
		w = 32
	}
	return w
}

func (s *Suggest) Close() {
	s.visible = false
	s.items = nil
	s.index = 0
}

func (s Suggest) ApplyCompletion(buf string, cursor int, addTrailingSpace bool) (string, int) {
	sel := s.Selected()
	if sel == nil || s.word.Start < 0 {
		return buf, cursor
	}
	before := buf[:s.word.Start]
	after := buf[s.word.End:]
	insert := "/" + sel.Name
	if addTrailingSpace && sel.ArgsHint != "" && !strings.HasPrefix(after, " ") && !strings.HasPrefix(after, "\t") {
		insert += " "
	}
	newBuf := before + insert + after
	newCur := len(before) + len(insert)
	return newBuf, newCur
}

func (s Suggest) View() string {
	if !s.visible {
		return ""
	}
	lines := make([]string, 0, len(s.items))
	for i, it := range s.items {
		name := "/" + it.Name
		hint := ""
		if it.ArgsHint != "" {
			hint = " " + it.ArgsHint
		}
		body := name + s.styles.Hint.Render(hint) + "   " + s.styles.Hint.Render(it.Short)
		if i == s.index {
			body = s.styles.Selected.Render(" ▸ " + body)
		} else {
			body = s.styles.Item.Render("   " + body)
		}
		lines = append(lines, body)
	}
	body := strings.Join(lines, "\n")
	return s.styles.Border.
		BorderStyle(lipgloss.RoundedBorder()).
		Render(body)
}

func detectSlashWord(buf string, cursor int) SlashWord {
	none := SlashWord{Start: -1}
	if cursor < 0 || cursor > len(buf) {
		return none
	}
	lineStart := strings.LastIndexByte(buf[:cursor], '\n') + 1
	rest := buf[cursor:]
	lineEnd := cursor + len(rest)
	if i := strings.IndexByte(rest, '\n'); i >= 0 {
		lineEnd = cursor + i
	}
	line := buf[lineStart:lineEnd]
	lineCursor := cursor - lineStart

	for i := 0; i < len(line); i++ {
		if line[i] != '/' {
			continue
		}
		if i > 0 {
			c := line[i-1]
			if c != ' ' && c != '\t' {
				continue
			}
		}
		if i+1 < len(line) && line[i+1] == '/' {
			continue
		}
		j := i + 1
		for j < len(line) && line[j] != ' ' && line[j] != '\t' {
			j++
		}
		if lineCursor < i || lineCursor > j {
			continue
		}
		word := line[i+1 : j]
		return SlashWord{
			Start:   lineStart + i,
			End:     lineStart + j,
			Word:    word,
			Matches: isVerbExactMatch(word),
		}
	}
	return none
}

func isVerbExactMatch(w string) bool {
	if w == "" {
		return false
	}
	_, ok := commands.Registry[strings.ToLower(w)]
	return ok
}

func RecognizedVerbAtStart(buf string) string {
	line := buf
	if i := strings.IndexByte(buf, '\n'); i >= 0 {
		line = buf[:i]
	}
	line = strings.TrimLeft(line, " \t")
	if !strings.HasPrefix(line, "/") || strings.HasPrefix(line, "//") {
		return ""
	}
	rest := line[1:]
	if i := strings.IndexAny(rest, " \t"); i >= 0 {
		rest = rest[:i]
	}
	if isVerbExactMatch(rest) {
		return rest
	}
	return ""
}

func filterVerbs(q string) []commands.VerbSpec {
	all := commands.VisibleVerbs()
	if q == "" {
		return all
	}
	out := make([]commands.VerbSpec, 0, len(all))
	for _, v := range all {
		if strings.Contains(strings.ToLower(v.Name), strings.ToLower(q)) {
			out = append(out, v)
		}
	}
	return out
}
