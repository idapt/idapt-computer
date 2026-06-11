package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/idapt/idapt-cli/internal/api"
)

type ChatRunRequest struct {
	ChatID  string
	NewChat bool

	Text string

	ModelID     string
	AgentID     string
	WorkspaceID   string
	EffortLevel string
	BranchFrom  string
	FileIDs     []string

	StreamMode bool // request SSE if supported
	TimeoutSec int  // sync mode only

	JSONMode bool
	NoColor  bool

	Out io.Writer
}

type ChatRunResult struct {
	ChatID    string
	MessageID string
	ModelID   string
	Cost      float64
	Tokens    int
	Body      string
}

func RunChat(ctx context.Context, client *api.Client, req ChatRunRequest) (*ChatRunResult, error) {
	if client == nil {
		return nil, errors.New("chat runner: nil client")
	}
	if strings.TrimSpace(req.Text) == "" {
		return nil, errors.New("chat runner: empty prompt")
	}
	if req.Out == nil {
		req.Out = io.Discard
	}

	if req.StreamMode {
		return runStreaming(ctx, client, req)
	}
	return runSync(ctx, client, req)
}

func runSync(ctx context.Context, client *api.Client, req ChatRunRequest) (*ChatRunResult, error) {
	body := map[string]any{"content": req.Text}
	if req.ModelID != "" {
		body["model_id"] = req.ModelID
	}
	if req.AgentID != "" {
		body["agent_id"] = req.AgentID
	}
	if req.EffortLevel != "" {
		body["effort_level"] = req.EffortLevel
	}
	if req.BranchFrom != "" {
		body["branch_from"] = req.BranchFrom
	}
	if req.TimeoutSec > 0 {
		body["timeout"] = req.TimeoutSec
	}
	if len(req.FileIDs) > 0 {
		body["file_ids"] = req.FileIDs
	}

	path := "/api/v1/chats/" + req.ChatID + "/messages"
	if req.NewChat {
		body["agent_id"] = req.AgentID
		body["workspace_id"] = req.WorkspaceID
		body["first_message"] = req.Text
		path = "/api/v1/chats"
	}

	var resp v1ItemResponse
	if err := client.Post(ctx, path, body, &resp); err != nil {
		return nil, err
	}

	res := &ChatRunResult{
		ChatID:    asString(resp.Data["id"]),
		ModelID:   asString(resp.Data["model_id"]),
		MessageID: asString(resp.Data["message_id"]),
	}
	if message, ok := resp.Data["message"].(map[string]any); ok {
		if content, _ := message["content"].(string); content != "" {
			res.Body = content
		}
		if id, _ := message["id"].(string); id != "" {
			res.MessageID = id
		}
	}
	if res.Body == "" {
		if content, _ := resp.Data["content"].(string); content != "" {
			res.Body = content
		}
	}
	if req.JSONMode {
		_ = json.NewEncoder(req.Out).Encode(resp.Data)
	} else if res.Body != "" {
		_, _ = io.WriteString(req.Out, res.Body)
		_, _ = io.WriteString(req.Out, "\n")
	}
	return res, nil
}

func runStreaming(ctx context.Context, client *api.Client, req ChatRunRequest) (*ChatRunResult, error) {
	body := map[string]any{"text": req.Text}
	if req.ModelID != "" {
		body["model"] = req.ModelID
	}
	if req.AgentID != "" {
		body["agent_id"] = req.AgentID
	}
	if req.WorkspaceID != "" {
		body["workspace_id"] = req.WorkspaceID
	}
	if len(req.FileIDs) > 0 {
		body["file_ids"] = req.FileIDs
	}

	path := "/api/v1/chats/" + req.ChatID + "/messages?stream=true"
	if req.NewChat {
		path = "/api/v1/chats?stream=true"
	}
	reader, err := client.StreamSSE(ctx, "POST", path, body,
		api.WithHeartbeat(45*time.Second),
	)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	res := &ChatRunResult{}
	var bodyBuf strings.Builder
	for {
		ev, rerr := reader.Next()
		if errors.Is(rerr, io.EOF) {
			break
		}
		if rerr != nil {
			return nil, rerr
		}
		switch ev.Event {
		case "done", "complete":
			cost, tokens := parseDonePayload(ev.Data)
			res.Cost = cost
			res.Tokens = tokens
			if id, _ := jsonField(ev.Data, "chat_id"); id != "" {
				res.ChatID = id
			}
			if id, _ := jsonField(ev.Data, "message_id"); id != "" {
				res.MessageID = id
			}
			if req.JSONMode {
				_, _ = fmt.Fprintf(req.Out, "%s\n", ev.Data)
			}
			res.Body = bodyBuf.String()
			return res, nil
		case "error":
			return nil, fmt.Errorf("stream error: %s", ev.Data)
		default:
			chunk := chunkText(ev.Data)
			if chunk == "" {
				continue
			}
			bodyBuf.WriteString(chunk)
			if req.JSONMode {
				_, _ = fmt.Fprintf(req.Out, "{\"event\":\"chunk\",\"text\":%q}\n", chunk)
			} else {
				_, _ = io.WriteString(req.Out, chunk)
			}
		}
	}
	res.Body = bodyBuf.String()
	if !req.JSONMode {
		_, _ = io.WriteString(req.Out, "\n")
	}
	return res, nil
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

func parseDonePayload(data string) (cost float64, tokens int) {
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

func jsonField(data, key string) (string, bool) {
	var m map[string]any
	if err := json.Unmarshal([]byte(data), &m); err != nil {
		return "", false
	}
	if v, ok := m[key].(string); ok {
		return v, true
	}
	return "", false
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}
