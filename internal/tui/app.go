package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cliconfig"
	"github.com/idapt/idapt-cli/internal/credential"
	"github.com/idapt/idapt-cli/internal/tui/widgets"
)

type ViewState int

const (
	viewChat ViewState = iota
	viewPicker
	viewHelp
)

const streamThrottle = 50 * time.Millisecond

type pickerKind int

const (
	pickerNone pickerKind = iota
	pickerModel
	pickerAgent
	pickerProject
	pickerMenu        // shows all slash commands; selecting dispatches that verb
	pickerThemeArgs   // shows auto / light / dark for /theme
	pickerUnfileArgs  // shows currently-attached files for /unfile
)

type Model struct {
	width, height int

	state ViewState

	projectID   string
	projectName string
	agentID     string
	agentName   string
	modelID     string
	modelName   string
	chatID      string

	transcript widgets.Transcript
	composer   widgets.Composer
	header     widgets.Header
	status     widgets.Status
	picker     widgets.Picker
	pickerKind pickerKind
	suggest    widgets.Suggest
	toolbar    widgets.Toolbar

	modelPickerForceRefetch bool

	toolbarSeeded bool

	streaming    bool
	streamMsgID  string
	streamBuf    string
	streamCh     <-chan tea.Msg
	streamCancel context.CancelFunc
	lastEventID  string
	lastRender   time.Time

	lastCtrlC time.Time

	reconnectAttempt int

	api     *api.Client
	cfg     cliconfig.Config
	cfgPath string
	creds   credential.Credentials
	theme   *Theme
	noColor bool // remembered for in-session theme rebuilds
	keymap  KeyMap

	now func() time.Time
}

func NewModel(client *api.Client, cfg cliconfig.Config, cfgPath string, creds credential.Credentials, noColor bool) Model {
	mode := ParseThemeMode(cfg.Theme)
	theme := NewTheme(mode, noColor)
	composer := widgets.NewComposer(widgets.ComposerStyles{
		FileChip: theme.FileChip,
		Hint:     theme.Hint,
		Border:   theme.ComposerBorder,
	})
	transcript := widgets.NewTranscript(widgets.TranscriptStyles{
		User:      theme.UserLabel,
		Assistant: theme.AssistantLabel,
		System:    theme.SystemLabel,
		Error:     theme.Error,
		Muted:     theme.Muted,
		NoColor:   noColor,
	})
	header := widgets.NewHeader(widgets.HeaderStyles{
		Title: theme.HeaderTitle,
		Meta:  theme.HeaderMeta,
		Bar:   theme.HeaderBar,
	})
	status := widgets.NewStatus(widgets.StatusStyles{
		Bar:        theme.StatusBar,
		OK:         theme.StatusOK,
		Streaming:  theme.StatusStreaming,
		Warn:       theme.StatusWarn,
		Error:      theme.StatusError,
		HintStyle:  theme.Hint,
		MutedStyle: theme.Muted,
	})
	picker := widgets.NewPicker("Pick", widgets.PickerStyles{
		Border:   theme.PickerBorder,
		Selected: theme.PickerSelected,
		Item:     theme.PickerItem,
		Locked:   theme.PickerLocked,
		Filter:   theme.PickerFilter,
	})
	suggest := widgets.NewSuggest(widgets.SuggestStyles{
		Border:   theme.SuggestBorder,
		Selected: theme.SuggestSelected,
		Item:     theme.SuggestItem,
		Hint:     theme.SuggestHint,
	})
	toolbar := widgets.NewToolbar(widgets.ToolbarStyles{
		Idle:    theme.ButtonIdle,
		Hover:   theme.ButtonHover,
		Primary: theme.ButtonPrimary,
		Danger:  theme.ButtonDanger,
		Bar:     theme.StatusBar,
	})
	return Model{
		state:      viewChat,
		api:        client,
		cfg:        cfg,
		cfgPath:    cfgPath,
		creds:      creds,
		noColor:    noColor,
		theme:      theme,
		keymap:     DefaultKeyMap(),
		composer:   composer,
		transcript: transcript,
		header:     header,
		status:     status,
		picker:     picker,
		suggest:    suggest,
		toolbar:    toolbar,
		projectID:  cfg.DefaultProject,
		agentID:    cfg.LastAgentID,
		modelID:    cfg.LastModelID,
		chatID:     cfg.LastChatID,
		now:        time.Now,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		m.composer.Focus(),
	)
}

func generateID() string { return uuid.NewString() }
