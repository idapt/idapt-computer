package tui

import (
	"os"

	"github.com/charmbracelet/lipgloss"
)

type ThemeMode int

const (
	ThemeAuto ThemeMode = iota
	ThemeLight
	ThemeDark
)

func ParseThemeMode(s string) ThemeMode {
	switch s {
	case "light":
		return ThemeLight
	case "dark":
		return ThemeDark
	default:
		return ThemeAuto
	}
}

func (m ThemeMode) String() string {
	switch m {
	case ThemeLight:
		return "light"
	case ThemeDark:
		return "dark"
	default:
		return "auto"
	}
}

func (m ThemeMode) Next() ThemeMode {
	return (m + 1) % 3
}

type Theme struct {
	Mode ThemeMode

	HeaderBar   lipgloss.Style
	HeaderTitle lipgloss.Style
	HeaderMeta  lipgloss.Style
	HeaderAccent lipgloss.Style

	UserLabel      lipgloss.Style
	AssistantLabel lipgloss.Style
	SystemLabel    lipgloss.Style

	ComposerBorder lipgloss.Style
	ComposerPrompt lipgloss.Style
	FileChip       lipgloss.Style

	StatusBar       lipgloss.Style
	StatusOK        lipgloss.Style
	StatusStreaming lipgloss.Style
	StatusWarn      lipgloss.Style
	StatusError     lipgloss.Style

	PickerBorder   lipgloss.Style
	PickerSelected lipgloss.Style
	PickerItem     lipgloss.Style
	PickerLocked   lipgloss.Style
	PickerFilter   lipgloss.Style

	SuggestBorder   lipgloss.Style
	SuggestSelected lipgloss.Style
	SuggestItem     lipgloss.Style
	SuggestHint     lipgloss.Style

	ButtonIdle    lipgloss.Style
	ButtonHover   lipgloss.Style
	ButtonPrimary lipgloss.Style
	ButtonDanger  lipgloss.Style

	Dim   lipgloss.Style
	Muted lipgloss.Style
	Hint  lipgloss.Style
	Error lipgloss.Style
}

type palette struct {
	Primary lipgloss.TerminalColor
	Accent  lipgloss.TerminalColor
	Subtle  lipgloss.TerminalColor
	Muted   lipgloss.TerminalColor
	OK      lipgloss.TerminalColor
	Warn    lipgloss.TerminalColor
	Err     lipgloss.TerminalColor
	BgSoft  lipgloss.TerminalColor
}

func NewTheme(mode ThemeMode, noColor bool) *Theme {
	if !noColor && os.Getenv("NO_COLOR") != "" {
		noColor = true
	}
	mk := func(s lipgloss.Style) lipgloss.Style {
		if noColor {
			return lipgloss.NewStyle()
		}
		return s
	}

	var p palette
	switch mode {
	case ThemeLight:
		p = palette{
			Primary: lipgloss.Color("#0061FF"),
			Accent:  lipgloss.Color("#A626A4"),
			Subtle:  lipgloss.Color("#5C6370"),
			Muted:   lipgloss.Color("#A0A1A7"),
			OK:      lipgloss.Color("#43A047"),
			Warn:    lipgloss.Color("#C07003"),
			Err:     lipgloss.Color("#C2185B"),
			BgSoft:  lipgloss.Color("#EEF1F8"),
		}
	case ThemeDark:
		p = palette{
			Primary: lipgloss.Color("#7AA2F7"),
			Accent:  lipgloss.Color("#BB9AF7"),
			Subtle:  lipgloss.Color("#7E8AA0"),
			Muted:   lipgloss.Color("#565F89"),
			OK:      lipgloss.Color("#9ECE6A"),
			Warn:    lipgloss.Color("#E0AF68"),
			Err:     lipgloss.Color("#F7768E"),
			BgSoft:  lipgloss.Color("#1F2335"),
		}
	default: // Auto
		p = palette{
			Primary: lipgloss.AdaptiveColor{Light: "#0061FF", Dark: "#7AA2F7"},
			Accent:  lipgloss.AdaptiveColor{Light: "#A626A4", Dark: "#BB9AF7"},
			Subtle:  lipgloss.AdaptiveColor{Light: "#5C6370", Dark: "#7E8AA0"},
			Muted:   lipgloss.AdaptiveColor{Light: "#A0A1A7", Dark: "#565F89"},
			OK:      lipgloss.AdaptiveColor{Light: "#43A047", Dark: "#9ECE6A"},
			Warn:    lipgloss.AdaptiveColor{Light: "#C07003", Dark: "#E0AF68"},
			Err:     lipgloss.AdaptiveColor{Light: "#C2185B", Dark: "#F7768E"},
			BgSoft:  lipgloss.AdaptiveColor{Light: "#EEF1F8", Dark: "#1F2335"},
		}
	}

	return &Theme{
		Mode: mode,

		HeaderBar:    mk(lipgloss.NewStyle().Bold(true)),
		HeaderTitle:  mk(lipgloss.NewStyle().Foreground(p.Primary).Bold(true)),
		HeaderMeta:   mk(lipgloss.NewStyle().Foreground(p.Subtle)),
		HeaderAccent: mk(lipgloss.NewStyle().Foreground(p.Accent).Bold(true)),

		UserLabel:      mk(lipgloss.NewStyle().Foreground(p.Primary).Bold(true)),
		AssistantLabel: mk(lipgloss.NewStyle().Foreground(p.Accent).Bold(true)),
		SystemLabel:    mk(lipgloss.NewStyle().Foreground(p.Muted).Italic(true)),

		ComposerBorder: mk(lipgloss.NewStyle().Foreground(p.Primary)),
		ComposerPrompt: mk(lipgloss.NewStyle().Foreground(p.Primary).Bold(true)),
		FileChip:       mk(lipgloss.NewStyle().Foreground(p.Accent).Bold(true).Background(p.BgSoft).Padding(0, 1)),

		StatusBar:       mk(lipgloss.NewStyle().Foreground(p.Subtle)),
		StatusOK:        mk(lipgloss.NewStyle().Foreground(p.OK)),
		StatusStreaming: mk(lipgloss.NewStyle().Foreground(p.Primary).Bold(true)),
		StatusWarn:      mk(lipgloss.NewStyle().Foreground(p.Warn).Bold(true)),
		StatusError:     mk(lipgloss.NewStyle().Foreground(p.Err).Bold(true)),

		PickerBorder:   mk(lipgloss.NewStyle().Foreground(p.Subtle)),
		PickerSelected: mk(lipgloss.NewStyle().Foreground(p.Primary).Bold(true).Reverse(true)),
		PickerItem:     mk(lipgloss.NewStyle()),
		PickerLocked:   mk(lipgloss.NewStyle().Foreground(p.Muted).Strikethrough(true)),
		PickerFilter:   mk(lipgloss.NewStyle().Foreground(p.Accent)),

		SuggestBorder:   mk(lipgloss.NewStyle().Foreground(p.Subtle)),
		SuggestSelected: mk(lipgloss.NewStyle().Foreground(p.Primary).Bold(true).Background(p.BgSoft)),
		SuggestItem:     mk(lipgloss.NewStyle()),
		SuggestHint:     mk(lipgloss.NewStyle().Foreground(p.Muted).Italic(true)),

		ButtonIdle:    mk(lipgloss.NewStyle().Foreground(p.Subtle).Padding(0, 1)),
		ButtonHover:   mk(lipgloss.NewStyle().Foreground(p.Primary).Bold(true).Background(p.BgSoft).Padding(0, 1)),
		ButtonPrimary: mk(lipgloss.NewStyle().Foreground(p.Primary).Bold(true).Padding(0, 1)),
		ButtonDanger:  mk(lipgloss.NewStyle().Foreground(p.Err).Bold(true).Padding(0, 1)),

		Dim:   mk(lipgloss.NewStyle().Foreground(p.Muted)),
		Muted: mk(lipgloss.NewStyle().Foreground(p.Subtle)),
		Hint:  mk(lipgloss.NewStyle().Foreground(p.Subtle).Italic(true)),
		Error: mk(lipgloss.NewStyle().Foreground(p.Err).Bold(true)),
	}
}
