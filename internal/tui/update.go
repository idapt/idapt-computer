package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/idapt/idapt-cli/internal/tui/commands"
	"github.com/idapt/idapt-cli/internal/tui/stream"
	"github.com/idapt/idapt-cli/internal/tui/widgets"
)
type sendUserMessage struct{ Text string }

type streamChunkMsg struct {
	MessageID string
	Text      string
}

type streamDoneMsg struct {
	MessageID string
	Cost      float64
	Tokens    int
}

type streamErrMsg struct{ Err error }

type reconnectingMsg struct{ Attempt int }

type slashCommandMsg struct{ Cmd commands.Parsed }

type quitAfterSaveMsg struct{}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		return m.handleResize(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case sendUserMessage:
		return m.handleSendUserMessage(msg)

	case stream.ReadyMsg:
		m.streamCancel = msg.Cancel
		m.streamCh = msg.Ch
		return m, stream.ReadNext(msg.Ch)

	case stream.ChunkMsg:
		next, _ := m.handleStreamChunk(streamChunkMsg{MessageID: msg.MessageID, Text: msg.Text})
		nm, _ := next.(Model)
		return nm, stream.ReadNext(nm.streamCh)

	case stream.DoneMsg:
		if msg.LastEventID != "" {
			m.lastEventID = msg.LastEventID
		}
		return m.handleStreamDone(streamDoneMsg{MessageID: msg.MessageID, Cost: msg.Cost, Tokens: msg.Tokens})

	case stream.ErrMsg:
		return m.handleStreamErr(streamErrMsg{Err: msg.Err})

	case streamChunkMsg:
		return m.handleStreamChunk(msg)

	case streamDoneMsg:
		return m.handleStreamDone(msg)

	case streamErrMsg:
		return m.handleStreamErr(msg)

	case reconnectingMsg:
		s := m.statusState()
		s.Kind = widgets.StatusReconnecting
		s.Message = "Reconnecting…"
		m.status.SetState(s)
		return m, nil

	case slashCommandMsg:
		return m.dispatchSlash(msg.Cmd)

	case pickerLoadedMsg:
		if msg.Kind != m.pickerKind {
			return m, nil
		}
		if msg.Err != nil {
			m.transcript.AppendError("picker load failed: " + msg.Err.Error())
			m.state = viewChat
			m.picker.Close()
			return m, m.composer.Focus()
		}
		m.picker.Open(msg.Items)
		m.picker.SetSize(modalDims(m.width, m.height))
		if n := len(msg.Items); n > 0 {
			m.picker.ShrinkListToContent(n)
		}
		return m, nil

	case pickerSelectMsg:
		switch msg.Kind {
		case pickerMenu:
			verb := msg.Item.ID
			m.state = viewChat
			m.picker.Close()
			focusCmd := m.composer.Focus()
			next, cmd := m.dispatchSlashByVerb(verb, nil)
			return next, tea.Batch(focusCmd, cmd)
		case pickerThemeArgs:
			arg := msg.Item.ID
			m.state = viewChat
			m.picker.Close()
			focusCmd := m.composer.Focus()
			next, cmd := m.dispatchSlashByVerb("theme", []string{arg})
			return next, tea.Batch(focusCmd, cmd)
		case pickerUnfileArgs:
			arg := msg.Item.ID
			m.state = viewChat
			m.picker.Close()
			focusCmd := m.composer.Focus()
			next, cmd := m.dispatchSlashByVerb("unfile", []string{arg})
			return next, tea.Batch(focusCmd, cmd)
		}
		switch msg.Kind {
		case pickerModel:
			m.modelID = msg.Item.ID
			m.modelName = msg.Item.Label
			m.cfg.PushRecentModelID(msg.Item.ID)
		case pickerAgent:
			m.agentID = msg.Item.ID
			m.agentName = msg.Item.Label
		case pickerProject:
			m.projectID = msg.Item.ID
			m.projectName = msg.Item.Label
		}
		m.header.SetState(m.headerState())
		m.state = viewChat
		m.picker.Close()
		return m, tea.Batch(m.composer.Focus(), m.persistContext())

	case fileUploadedMsg:
		if msg.Err != nil {
			m.transcript.AppendError("upload " + msg.Path + ": " + msg.Err.Error())
			m.composer.DetachFile(msg.Path)
			return m, nil
		}
		m.composer.SetFileID(msg.Path, msg.ID)
		return m, nil

	case quitAfterSaveMsg:
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) handleResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	const (
		headerLines  = 1
		statusLines  = 1
		toolbarLines = 1
		composerLines = 3
	)
	transcriptH := m.height - headerLines - toolbarLines - statusLines - composerLines
	if transcriptH < 3 {
		transcriptH = 3
	}
	m.header.SetSize(m.width)
	m.transcript.SetSize(m.width, transcriptH)
	m.composer.SetSize(m.width, 1)
	m.status.SetSize(m.width)
	m.refreshToolbar()
	if m.state == viewPicker && m.picker.Visible() {
		m.picker.SetSize(modalDims(m.width, m.height))
	}
	return m, nil
}

func (m *Model) refreshToolbar() {
	m.toolbar.SetButtons(defaultToolbarButtons(m.streaming))
	m.toolbar.SetWidth(m.width)
	_ = m.toolbar.View() // side-effect: populates hitMap for the next mouse press
	m.toolbarSeeded = true
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.state == viewPicker {
		return m.handlePickerKey(msg)
	}
	switch msg.String() {
	case "ctrl+c":
		return m.handleCtrlC()
	case "ctrl+d":
		if !m.streaming && strings.TrimSpace(m.composer.Value()) == "" {
			return m, m.quitAfterPersist()
		}
	case "alt+enter":
		if !m.composer.Disabled() {
			m.composer.InsertNewline()
			m.refreshSuggestion()
		}
		return m, nil
	}

	if msg.Type == tea.KeyRunes && looksLikeUnparsedCSI(string(msg.Runes)) {
		return m, nil
	}

	if m.suggest.Visible() {
		switch msg.String() {
		case "up":
			m.suggest.Prev()
			return m, nil
		case "down":
			m.suggest.Next()
			return m, nil
		case "esc":
			m.suggest.Close()
			return m, nil
		case "tab":
			m.applySuggestionCompletion(true) // keep trailing space
			return m, nil
		case "enter":
			if !m.composer.Disabled() {
				m.applySuggestionCompletion(false)
				m.suggest.Close()
				return m.submitComposer()
			}
		}
	}

	if msg.String() == "enter" && !m.composer.Disabled() {
		return m.submitComposer()
	}

	c, cmd := m.composer.Update(msg)
	m.composer = c
	m.refreshSuggestion()
	return m, cmd
}

func (m *Model) refreshSuggestion() {
	buf := m.composer.Value()
	cur := m.composer.CursorByteOffset()
	m.suggest.UpdateFromBuffer(buf, cur)
	m.composer.SetRecognizedCommand(widgets.RecognizedVerbAtStart(buf))
}

func (m *Model) applySuggestionCompletion(addTrailingSpace bool) {
	buf := m.composer.Value()
	cur := m.composer.CursorByteOffset()
	newBuf, newCur := m.suggest.ApplyCompletion(buf, cur, addTrailingSpace)
	if newBuf == buf {
		return
	}
	m.composer.SetValueAt(newBuf, newCur)
	m.refreshSuggestion()
}

func (m Model) submitComposer() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.composer.Value())
	if text == "" {
		return m, nil
	}
	if commands.IsSlash(text) {
		parsed, err := commands.Parse(text)
		if err != nil {
			m.transcript.AppendError(err.Error())
			m.composer.ResetText()
			m.suggest.Close()
			return m, nil
		}
		m.composer.ResetText()
		m.suggest.Close()
		return m.dispatchSlash(parsed)
	}
	m.composer.Reset()
	m.suggest.Close()
	return m, func() tea.Msg { return sendUserMessage{Text: text} }
}

func (m Model) handlePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state = viewChat
		m.picker.Close()
		m.modelPickerForceRefetch = false
		return m, m.composer.Focus()
	case "enter":
		sel := m.picker.Selected()
		if sel == nil {
			return m, nil
		}
		kind := m.pickerKind
		item := *sel
		return m, func() tea.Msg { return pickerSelectMsg{Kind: kind, Item: item} }
	case "ctrl+r":
		if m.pickerKind == pickerModel {
			m.modelPickerForceRefetch = true
			m.picker.Open([]widgets.PickerItem{{ID: "", Label: "refetching…", Locked: true}})
			cmd := m.fetchPickerItems(m.pickerKind)
			m.modelPickerForceRefetch = false
			return m, cmd
		}
		return m, nil
	}
	p, cmd := m.picker.Update(msg)
	m.picker = p
	return m, cmd
}

func (m Model) handleCtrlC() (tea.Model, tea.Cmd) {
	now := m.now()
	if m.streaming {
		if !m.lastCtrlC.IsZero() && now.Sub(m.lastCtrlC) < 200*time.Millisecond {
			m.cancelStream()
			return m, m.quitAfterPersist()
		}
		m.lastCtrlC = now
		m.cancelStream()
		return m, nil
	}
	m.lastCtrlC = now
	if strings.TrimSpace(m.composer.Value()) == "" {
		return m, m.quitAfterPersist()
	}
	m.composer.Reset()
	return m, nil
}

func (m Model) dispatchSlashByVerb(verb string, args []string) (tea.Model, tea.Cmd) {
	return m.dispatchSlash(commands.Parsed{Verb: verb, Args: args})
}

func (m Model) dispatchSlash(p commands.Parsed) (tea.Model, tea.Cmd) {
	if m.streaming && p.Verb != "quit" && p.Verb != "exit" {
		m.transcript.AppendError("blocked while streaming — Ctrl+C first")
		return m, nil
	}
	switch p.Verb {
	case "help":
		m.transcript.Append(widgets.Message{Role: widgets.RoleSystem, Body: helpText(m.keymap)})
		return m, nil
	case "new", "clear":
		if m.streaming {
			m.cancelStream()
		}
		m.transcript.Clear()
		m.chatID = ""
		m.lastEventID = ""
		s := m.statusState()
		s.Kind = widgets.StatusIdle
		s.Message = "new chat"
		m.status.SetState(s)
		return m, m.persistContext()
	case "quit", "exit":
		if m.streaming {
			m.cancelStream()
		}
		return m, m.quitAfterPersist()
	case "theme":
		return m.handleThemeSlash(p.Args)
	case "menu":
		return m.openPicker(pickerMenu)
	case "copy":
		last := m.transcript.LastAssistant()
		if last == nil {
			m.transcript.AppendError("nothing to copy yet")
			return m, nil
		}
		return m, copyToClipboardCmd(last.Body)
	case "edit":
		last := m.transcript.LastUser()
		if last == nil {
			m.transcript.AppendError("no previous message to edit")
			return m, nil
		}
		m.composer.SetValue(last.Body)
		return m, nil
	case "regen":
		m.transcript.AppendError("regen not wired into the interactive TUI yet — use `idapt chat ask --branch-from <message-id>` for now")
		return m, nil
	case "model":
		if len(p.Args) > 0 {
			m.modelID = p.Args[0]
			m.modelName = p.Args[0]
			m.header.SetState(m.headerState())
			return m, m.persistContext()
		}
		return m.openPicker(pickerModel)
	case "agent":
		if len(p.Args) > 0 {
			m.agentID = p.Args[0]
			m.agentName = p.Args[0]
			m.header.SetState(m.headerState())
			return m, m.persistContext()
		}
		return m.openPicker(pickerAgent)
	case "project":
		if len(p.Args) > 0 {
			m.projectID = p.Args[0]
			m.projectName = p.Args[0]
			m.header.SetState(m.headerState())
			return m, m.persistContext()
		}
		return m.openPicker(pickerProject)
	case "file":
		if len(p.Args) == 0 {
			m.transcript.AppendError("/file requires a path")
			return m, nil
		}
		path := p.Args[0]
		if _, err := osStat(path); err != nil {
			m.transcript.AppendError("file not found: " + path)
			return m, nil
		}
		m.composer.AttachFile(path)
		return m, uploadAttachmentCmd(m.api, m.projectID, path)
	case "files":
		files := m.composer.Files()
		if len(files) == 0 {
			m.transcript.AppendError("no files attached")
			return m, nil
		}
		var lines []string
		for _, f := range files {
			lines = append(lines, "  "+f.Path)
		}
		m.transcript.Append(widgets.Message{
			Role: widgets.RoleSystem,
			Body: "Attached files:\n" + strings.Join(lines, "\n"),
		})
		return m, nil
	case "unfile":
		if len(p.Args) == 0 {
			if len(m.composer.Files()) == 0 {
				m.transcript.AppendError("no files attached")
				return m, nil
			}
			return m.openPicker(pickerUnfileArgs)
		}
		if !m.composer.DetachFile(p.Args[0]) {
			m.transcript.AppendError("not attached: " + p.Args[0])
		}
		return m, nil
	}
	return m, nil
}

func (m Model) openPicker(kind pickerKind) (tea.Model, tea.Cmd) {
	m.pickerKind = kind
	m.state = viewPicker
	m.composer.Blur()
	m.picker.SetTitle(pickerTitleFor(kind))
	m.picker.SetSize(modalDims(m.width, m.height))
	m.picker.Open([]widgets.PickerItem{{ID: "", Label: "loading…", Locked: true}})
	return m, m.fetchPickerItems(kind)
}

func modalDims(termW, termH int) (int, int) {
	w := termW - 10
	if w > 80 {
		w = 80
	}
	if w < 40 {
		w = 40
	}
	h := termH - 8
	if h > 22 {
		h = 22
	}
	if h < 10 {
		h = 10
	}
	return w, h
}

func pickerTitleFor(kind pickerKind) string {
	switch kind {
	case pickerModel:
		return "Pick a model"
	case pickerAgent:
		return "Pick an agent"
	case pickerProject:
		return "Pick a project"
	case pickerMenu:
		return "Command palette"
	case pickerThemeArgs:
		return "Choose theme"
	case pickerUnfileArgs:
		return "Detach which file?"
	}
	return "Pick"
}

func (m Model) headerState() widgets.HeaderState {
	return widgets.HeaderState{
		Project: firstNonEmpty(m.projectName, m.projectID),
		Agent:   firstNonEmpty(m.agentName, m.agentID),
		Model:   firstNonEmpty(m.modelName, m.modelID),
	}
}

func (m Model) persistContext() tea.Cmd {
	cfg := m.cfg
	cfg.LastAgentID = m.agentID
	cfg.LastModelID = m.modelID
	cfg.LastChatID = m.chatID
	if m.projectID != "" {
		cfg.DefaultProject = m.projectID
	}
	path := m.cfgPath
	return func() tea.Msg {
		if path != "" {
			_ = persistConfigDirect(cfg, path)
		}
		return nil
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func helpText(km KeyMap) string {
	var b strings.Builder
	b.WriteString("Keybindings\n")
	for _, k := range km.AllBindings() {
		help := k.Help()
		b.WriteString("  ")
		b.WriteString(help.Key)
		b.WriteString("\t")
		b.WriteString(help.Desc)
		b.WriteString("\n")
	}
	b.WriteString("\nSlash commands\n")
	for _, v := range commands.CanonicalVerbs() {
		b.WriteString("  /")
		b.WriteString(v.Name)
		if v.ArgsHint != "" {
			b.WriteString(" ")
			b.WriteString(v.ArgsHint)
		}
		b.WriteString("\t")
		b.WriteString(v.Short)
		b.WriteString("\n")
	}
	return b.String()
}
