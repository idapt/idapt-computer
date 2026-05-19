package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/idapt/idapt-cli/internal/cliconfig"
	"github.com/idapt/idapt-cli/internal/tui/stream"
	"github.com/idapt/idapt-cli/internal/tui/widgets"
)

func (m Model) statusState() widgets.StatusState {
	s := widgets.StatusState{
		Kind:    widgets.StatusIdle,
		Message: "",
		Hint:    m.composer.HelpHint(),
	}
	if m.streaming {
		s.Kind = widgets.StatusStreaming
		s.Message = "streaming…"
	}
	return s
}

func (m Model) handleSendUserMessage(msg sendUserMessage) (tea.Model, tea.Cmd) {
	m.transcript.Append(widgets.Message{
		ID:   generateID(),
		Role: widgets.RoleUser,
		Body: msg.Text,
	})
	assistantID := generateID()
	m.transcript.BeginStreaming(assistantID)
	m.streaming = true
	m.streamMsgID = assistantID
	m.streamBuf = ""

	atts := []stream.Attachment{}
	skipped := 0
	for _, f := range m.composer.Files() {
		if f.ID == "" {
			skipped++
			continue
		}
		atts = append(atts, stream.Attachment{ID: f.ID, Path: f.Path})
	}
	if skipped > 0 {
		m.transcript.AppendError(fmt.Sprintf("%d attachment(s) still uploading — sent without them", skipped))
	}
	m.composer.ClearFiles()

	m.composer.SetDisabled(true)
	m.status.SetState(m.statusState())
	m.refreshToolbar()

	ctx, cancel := context.WithCancel(context.Background())
	m.streamCancel = cancel
	cmd := stream.Start(ctx, stream.Params{
		Client:      m.api,
		ChatID:      m.chatID,
		ProjectID:   m.projectID,
		AgentID:     m.agentID,
		ModelID:     m.modelID,
		Text:        msg.Text,
		MessageID:   assistantID,
		LastEventID: m.lastEventID,
		Attachments: atts,
	})
	return m, cmd
}

func (m Model) handleStreamChunk(msg streamChunkMsg) (tea.Model, tea.Cmd) {
	if msg.MessageID != "" && msg.MessageID != m.streamMsgID {
		return m, nil
	}
	m.streamBuf += msg.Text
	now := m.now()
	if now.Sub(m.lastRender) >= streamThrottle {
		m.transcript.UpdateStreaming(m.streamMsgID, m.streamBuf)
		m.lastRender = now
	}
	return m, nil
}

func (m Model) handleStreamDone(msg streamDoneMsg) (tea.Model, tea.Cmd) {
	if msg.MessageID != "" && msg.MessageID != m.streamMsgID {
		return m, nil
	}
	m.transcript.UpdateStreaming(m.streamMsgID, m.streamBuf)
	m.transcript.Finalize(m.streamMsgID, msg.Cost, msg.Tokens)
	m.streaming = false
	m.streamMsgID = ""
	m.streamBuf = ""
	m.composer.SetDisabled(false)
	m.streamCancel = nil
	s := m.statusState()
	s.Tokens = msg.Tokens
	s.Cost = msg.Cost
	m.status.SetState(s)
	m.refreshToolbar()
	_ = persistConfig(m.cfg, m.cfgPath, m)
	return m, nil
}

func (m Model) handleStreamErr(msg streamErrMsg) (tea.Model, tea.Cmd) {
	err := msg.Err
	if isAuthError(err) {
		m.transcript.AppendError("not authenticated — set IDAPT_API_KEY or run `idapt config set api-key <token>`")
		return m.finishStream(widgets.StatusError, "auth required"), nil
	}
	if isSpendingCap(err) {
		m.transcript.AppendError("spending cap reached — upgrade your plan to continue")
		return m.finishStream(widgets.StatusError, "spending cap"), nil
	}
	if isRateLimit(err) {
		m.transcript.AppendError("rate-limited — wait a moment and retry")
		return m.finishStream(widgets.StatusWarn, "rate-limited"), nil
	}
	if m.reconnectAttempt < stream.MaxAttempts && m.streaming {
		m.reconnectAttempt++
		s := m.statusState()
		s.Kind = widgets.StatusReconnecting
		s.Message = "Reconnecting…"
		m.status.SetState(s)
		ctx, cancel := context.WithCancel(context.Background())
		m.streamCancel = cancel
		params := stream.Params{
			Client:      m.api,
			ChatID:      m.chatID,
			ProjectID:   m.projectID,
			AgentID:     m.agentID,
			ModelID:     m.modelID,
			Text:        m.streamBuf,
			MessageID:   m.streamMsgID,
			LastEventID: m.lastEventID,
		}
		return m, stream.Reconnect(ctx, params, m.reconnectAttempt)
	}
	m.transcript.AppendError(err.Error())
	return m.finishStream(widgets.StatusError, "error"), nil
}

func (m Model) finishStream(kind widgets.StatusKind, msg string) Model {
	m.streaming = false
	m.streamMsgID = ""
	m.streamBuf = ""
	m.streamCh = nil
	m.reconnectAttempt = 0
	m.composer.SetDisabled(false)
	s := m.statusState()
	s.Kind = kind
	s.Message = msg
	m.status.SetState(s)
	m.refreshToolbar()
	return m
}

func isAuthError(err error) bool {
	type authErr interface{ ExitCode() int }
	if e, ok := err.(authErr); ok && e.ExitCode() == 2 {
		return true
	}
	return false
}

func isSpendingCap(err error) bool {
	type capErr interface{ ExitCode() int }
	if e, ok := err.(capErr); ok && e.ExitCode() == 6 {
		return true
	}
	return false
}

func isRateLimit(err error) bool {
	type rlErr interface{ ExitCode() int }
	if e, ok := err.(rlErr); ok && e.ExitCode() == 10 {
		return true
	}
	return false
}

func (m *Model) cancelStream() {
	if m.streamCancel != nil {
		m.streamCancel()
		m.streamCancel = nil
	}
	if m.streamMsgID != "" {
		m.transcript.MarkCancelled(m.streamMsgID)
	}
	m.streaming = false
	m.composer.SetDisabled(false)
	s := m.statusState()
	s.Kind = widgets.StatusWarn
	s.Message = "cancelled"
	m.status.SetState(s)
	m.refreshToolbar()
}

func (m Model) quitAfterPersist() tea.Cmd {
	cfg := m.cfg
	cfg.LastChatID = m.chatID
	cfg.LastAgentID = m.agentID
	cfg.LastModelID = m.modelID
	path := m.cfgPath
	return func() tea.Msg {
		if path != "" {
			_ = cliconfig.Save(path, cfg)
		}
		return quitAfterSaveMsg{}
	}
}

func persistConfig(cfg cliconfig.Config, path string, m Model) error {
	if path == "" {
		return nil
	}
	cfg.LastChatID = m.chatID
	cfg.LastAgentID = m.agentID
	cfg.LastModelID = m.modelID
	return cliconfig.Save(path, cfg)
}
