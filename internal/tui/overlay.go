package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/ansi"
)

func dimBackground(base string, w, h int) string {
	dim := lipgloss.NewStyle().Faint(true)
	lines := strings.Split(base, "\n")
	out := make([]string, len(lines))
	for i, ln := range lines {
		out[i] = dim.Render(ln)
	}
	_ = w
	_ = h
	return strings.Join(out, "\n")
}

func overlayCenter(base, top string, w, h int) string {
	baseLines := splitLinesToHeight(base, w, h)
	topLines := strings.Split(top, "\n")
	if len(topLines) == 0 {
		return strings.Join(baseLines, "\n")
	}
	topW := 0
	for _, ln := range topLines {
		if visible := ansi.PrintableRuneWidth(ln); visible > topW {
			topW = visible
		}
	}
	startRow := (h - len(topLines)) / 2
	if startRow < 0 {
		startRow = 0
	}
	startCol := (w - topW) / 2
	if startCol < 0 {
		startCol = 0
	}

	for i, top := range topLines {
		row := startRow + i
		if row < 0 || row >= len(baseLines) {
			continue
		}
		prefix := ansiSlice(baseLines[row], 0, startCol)
		suffix := ansiSlice(baseLines[row], startCol+topW, w)
		topPadded := top
		if visible := ansi.PrintableRuneWidth(top); visible < topW {
			topPadded += strings.Repeat(" ", topW-visible)
		}
		baseLines[row] = prefix + topPadded + suffix
	}
	return strings.Join(baseLines, "\n")
}

func splitLinesToHeight(s string, w, h int) []string {
	in := strings.Split(s, "\n")
	out := make([]string, h)
	for i := 0; i < h; i++ {
		if i < len(in) {
			line := in[i]
			visible := ansi.PrintableRuneWidth(line)
			if visible < w {
				line += strings.Repeat(" ", w-visible)
			}
			out[i] = line
		} else {
			out[i] = strings.Repeat(" ", w)
		}
	}
	return out
}

func ansiSlice(s string, start, end int) string {
	if end <= start {
		return ""
	}
	var b strings.Builder
	col := 0
	i := 0
	for i < len(s) {
		c := s[i]
		if c == 0x1b {
			j := i + 1
			if j < len(s) && (s[j] == '[' || s[j] == ']' || s[j] == '(' || s[j] == ')') {
				j++
				for j < len(s) {
					if s[j] >= 0x40 && s[j] <= 0x7e {
						j++
						break
					}
					j++
				}
			} else if j < len(s) {
				j++
			}
			b.WriteString(s[i:j])
			i = j
			continue
		}
		r, size := decodeRune(s[i:])
		width := runeCellWidth(r)
		if col >= end {
			break
		}
		if col >= start {
			b.WriteString(s[i : i+size])
		}
		col += width
		i += size
	}
	if col < end {
		b.WriteString(strings.Repeat(" ", end-col))
	}
	return b.String()
}

func decodeRune(s string) (rune, int) {
	for i, r := range s {
		_ = i
		return r, len(string(r))
	}
	return 0, 0
}

func runeCellWidth(r rune) int {
	if r < 0x20 {
		return 0
	}
	if r >= 0x1100 {
		return 2
	}
	return 1
}
