package markdown

import (
	"os"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
)

const (
	syntheticFenceMarker = "\n```\n"
)

var (
	rendererOnce sync.Once
	colorR       *glamour.TermRenderer
	asciiR       *glamour.TermRenderer
	renderMu     sync.Mutex
)

func ensureRenderers(width int) {
	rendererOnce.Do(func() {
		c, _ := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(width),
		)
		a, _ := glamour.NewTermRenderer(
			glamour.WithStandardStyle("ascii"),
			glamour.WithWordWrap(width),
		)
		colorR = c
		asciiR = a
	})
}

func pick(noColor bool) *glamour.TermRenderer {
	if noColor || os.Getenv("NO_COLOR") != "" {
		return asciiR
	}
	return colorR
}

func RenderFinal(body string, width int, noColor bool) string {
	ensureRenderers(width)
	renderMu.Lock()
	defer renderMu.Unlock()
	r := pick(noColor)
	if r == nil {
		return body
	}
	out, err := r.Render(body)
	if err != nil || out == "" {
		return body
	}
	return out
}

func RenderStreaming(body string, width int, noColor bool) string {
	synthesized := false
	if hasOpenFence(body) {
		body = body + syntheticFenceMarker
		synthesized = true
	}
	out := RenderFinal(body, width, noColor)
	if synthesized {
		out = strings.TrimRight(out, "\n")
	}
	return out
}

func hasOpenFence(s string) bool {
	count := 0
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "```") {
			count++
		}
	}
	return count%2 == 1
}
