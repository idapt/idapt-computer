package stream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/idapt/idapt-cli/internal/api"
)

type Params struct {
	Client *api.Client

	ChatID    string
	WorkspaceID string
	AgentID   string
	ModelID   string
	Text      string
	MessageID string // optimistic ID the Model uses for the assistant row

	LastEventID string // for resume

	Attachments []Attachment
}

type Attachment struct {
	ID   string `json:"file_id"`
	Path string `json:"-"`
}

type ChunkMsg struct {
	MessageID string
	Text      string
}

type DoneMsg struct {
	MessageID   string
	Cost        float64
	Tokens      int
	LastEventID string
}

type ErrMsg struct{ Err error }

type ReadyMsg struct {
	Ch     <-chan tea.Msg
	Cancel context.CancelFunc
}

type ReconnectingMsg struct{ Attempt int }

func Start(ctx context.Context, p Params) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(ctx)
		ch := make(chan tea.Msg, 64)

		body := buildBody(p)

		go runOnce(ctx, p, body, ch)

		return ReadyMsg{Ch: ch, Cancel: cancel}
	}
}

func ReadNext(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		m, ok := <-ch
		if !ok {
			return DoneMsg{}
		}
		return m
	}
}

func buildBody(p Params) map[string]any {
	b := map[string]any{
		"text": p.Text,
	}
	if p.ModelID != "" {
		b["model"] = p.ModelID
	}
	if p.AgentID != "" {
		b["agent_id"] = p.AgentID
	}
	if p.WorkspaceID != "" {
		b["workspace_id"] = p.WorkspaceID
	}
	if len(p.Attachments) > 0 {
		ids := make([]string, 0, len(p.Attachments))
		for _, a := range p.Attachments {
			ids = append(ids, a.ID)
		}
		b["file_ids"] = ids
	}
	return b
}

func runOnce(ctx context.Context, p Params, body map[string]any, ch chan<- tea.Msg) {
	defer close(ch)

	if p.Client == nil {
		ch <- ErrMsg{Err: errors.New("no API client")}
		return
	}
	path := streamPath(p.ChatID)
	method := "POST"
	if p.ChatID == "" {
		path = "/api/v1/chats?stream=true"
	}

	opts := []api.SSEOption{api.WithHeartbeat(45 * time.Second)}
	if p.LastEventID != "" {
		opts = append(opts, api.WithResume(p.LastEventID))
	}
	reader, err := p.Client.StreamSSE(ctx, method, path, body, opts...)
	if err != nil {
		ch <- ErrMsg{Err: err}
		return
	}
	defer reader.Close()

	lastID := p.LastEventID
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		ev, err := reader.Next()
		if errors.Is(err, io.EOF) {
			ch <- DoneMsg{MessageID: p.MessageID, LastEventID: lastID}
			return
		}
		if err != nil {
			ch <- ErrMsg{Err: err}
			return
		}
		if ev == nil {
			continue
		}
		if ev.ID != "" {
			lastID = ev.ID
		}
		switch ev.Event {
		case "done", "complete":
			cost, tokens := parseTermini(ev.Data)
			ch <- DoneMsg{MessageID: p.MessageID, Cost: cost, Tokens: tokens, LastEventID: lastID}
			return
		case "error":
			ch <- ErrMsg{Err: fmt.Errorf("stream error: %s", ev.Data)}
			return
		default:
			text := chunkText(ev.Data)
			if text == "" {
				continue
			}
			ch <- ChunkMsg{MessageID: p.MessageID, Text: text}
		}
	}
}

func chunkText(data string) string {
	if data == "" {
		return ""
	}
	if !strings.HasPrefix(strings.TrimSpace(data), "{") {
		return data
	}
	var env struct {
		Text  string `json:"text"`
		Delta string `json:"delta"`
	}
	if json.Unmarshal([]byte(data), &env) == nil {
		if env.Text != "" {
			return env.Text
		}
		return env.Delta
	}
	return data
}

func parseTermini(data string) (cost float64, tokens int) {
	if data == "" {
		return 0, 0
	}
	var env struct {
		Cost   float64 `json:"cost"`
		Tokens int     `json:"tokens"`
	}
	_ = json.Unmarshal([]byte(data), &env)
	return env.Cost, env.Tokens
}

func streamPath(chatID string) string {
	if chatID == "" {
		return ""
	}
	return "/api/v1/chats/" + chatID + "/messages?stream=true"
}
