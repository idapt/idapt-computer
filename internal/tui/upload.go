package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/idapt/idapt-cli/internal/api"
)

type fileUploadedMsg struct {
	Path string // user-typed path; lookup key on the composer chip
	ID   string // server-assigned file id, empty on failure
	Err  error
}

func uploadAttachmentCmd(client *api.Client, workspaceID, path string) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return fileUploadedMsg{Path: path, Err: fmt.Errorf("no api client")}
		}
		f, err := os.Open(path)
		if err != nil {
			return fileUploadedMsg{Path: path, Err: err}
		}
		defer f.Close()

		fields := map[string]string{"name": filepath.Base(path)}
		if workspaceID != "" {
			fields["workspace_id"] = workspaceID
		}
		resp, err := client.Upload(context.Background(), "/api/v1/drive/files", filepath.Base(path), f, fields)
		if err != nil {
			return fileUploadedMsg{Path: path, Err: err}
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			return fileUploadedMsg{Path: path, Err: fmt.Errorf("upload failed: status %d", resp.StatusCode)}
		}
		var env struct {
			Data map[string]any `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
			return fileUploadedMsg{Path: path, Err: fmt.Errorf("decoding upload response: %w", err)}
		}
		id, _ := env.Data["id"].(string)
		if id == "" {
			return fileUploadedMsg{Path: path, Err: fmt.Errorf("upload response missing id")}
		}
		return fileUploadedMsg{Path: path, ID: id}
	}
}
