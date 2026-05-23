package tui

import (
	"context"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cliconfig"
	"github.com/idapt/idapt-cli/internal/models"
	"github.com/idapt/idapt-cli/internal/tui/commands"
	"github.com/idapt/idapt-cli/internal/tui/widgets"
)

type pickerLoadedMsg struct {
	Kind  pickerKind
	Items []widgets.PickerItem
	Err   error
}

type pickerSelectMsg struct {
	Kind pickerKind
	Item widgets.PickerItem
}

func (m Model) fetchPickerItems(kind pickerKind) tea.Cmd {
	client := m.api
	projectID := m.projectID
	attachedSnap := m.composer.Files()
	apiKey := ""
	apiURL := ""
	if client != nil {
		apiKey = client.APIKey()
	}
	apiURL = m.cfg.APIURL
	recents := append([]string(nil), m.cfg.RecentModelIDs...)
	forceRefetch := m.modelPickerForceRefetch
	return func() tea.Msg {
		ctx := context.Background()
		switch kind {
		case pickerModel:
			items, err := loadModels(ctx, client, apiURL, apiKey, recents, forceRefetch)
			return pickerLoadedMsg{Kind: kind, Items: items, Err: err}
		case pickerAgent:
			items, err := loadAgents(ctx, client, projectID)
			return pickerLoadedMsg{Kind: kind, Items: items, Err: err}
		case pickerProject:
			items, err := loadProjects(ctx, client)
			return pickerLoadedMsg{Kind: kind, Items: items, Err: err}
		case pickerMenu:
			return pickerLoadedMsg{Kind: kind, Items: loadMenuVerbs()}
		case pickerThemeArgs:
			return pickerLoadedMsg{Kind: kind, Items: loadThemeArgs()}
		case pickerUnfileArgs:
			return pickerLoadedMsg{Kind: kind, Items: loadUnfileArgs(attachedSnap)}
		}
		return pickerLoadedMsg{Kind: kind, Err: nil}
	}
}

func loadThemeArgs() []widgets.PickerItem {
	return []widgets.PickerItem{
		{ID: "auto", Label: "Auto", Subtitle: "Adapt to terminal background"},
		{ID: "light", Label: "Light", Subtitle: "Force the light palette"},
		{ID: "dark", Label: "Dark", Subtitle: "Force the dark palette"},
	}
}

func loadUnfileArgs(attached []widgets.FileAttachment) []widgets.PickerItem {
	if len(attached) == 0 {
		return nil
	}
	out := make([]widgets.PickerItem, 0, len(attached))
	for _, f := range attached {
		sub := "attached"
		if f.ID == "" {
			sub = "uploading…"
		}
		out = append(out, widgets.PickerItem{ID: f.Path, Label: f.Path, Subtitle: sub})
	}
	return out
}

func loadMenuVerbs() []widgets.PickerItem {
	verbs := commands.VisibleVerbs()
	out := make([]widgets.PickerItem, 0, len(verbs))
	for _, v := range verbs {
		label := "/" + v.Name
		if v.ArgsHint != "" {
			label += " " + v.ArgsHint
		}
		out = append(out, widgets.PickerItem{
			ID:       v.Name,
			Label:    label,
			Subtitle: v.Short,
		})
	}
	return out
}

func loadModels(ctx context.Context, client *api.Client, apiURL, apiKey string, recents []string, forceRefetch bool) ([]widgets.PickerItem, error) {
	cachePath, _ := models.DefaultPath()

	if !forceRefetch {
		if entry, ok := models.LoadFromCache(cachePath, apiURL, apiKey); ok && entry.Fresh(timeNowForModels()) {
			return modelsToItems(entry.Models, recents), nil
		}
	}

	rows, fetchErr := fetchModelRows(ctx, client)
	if fetchErr == nil {
		_ = models.SaveToCache(cachePath, apiURL, apiKey, rows)
		return modelsToItems(rows, recents), nil
	}

	if entry, ok := models.LoadFromCache(cachePath, apiURL, apiKey); ok {
		return modelsToItems(entry.Models, recents), nil
	}
	return nil, fetchErr
}

func fetchModelRows(ctx context.Context, client *api.Client) ([]models.Row, error) {
	if client == nil {
		return nil, fmt.Errorf("no api client")
	}
	var resp api.V1ListResponse
	if err := client.Get(ctx, "/api/v1/models", nil, &resp); err != nil {
		return nil, err
	}
	out := make([]models.Row, 0, len(resp.Data))
	for _, row := range resp.Data {
		r := models.Row{}
		r.ID, _ = row["id"].(string)
		r.DisplayName, _ = row["display_name"].(string)
		r.Modality, _ = row["modality"].(string)
		r.Provider, _ = row["provider"].(string)
		if caps, ok := row["capabilities"].(map[string]any); ok {
			if v, ok := caps["context_length"].(float64); ok {
				r.ContextWindow = int(v)
			}
			if v, ok := caps["image_input"].(bool); ok {
				r.Vision = v
			}
		}
		if pricing, ok := row["pricing"].(map[string]any); ok {
			if v, ok := pricing["input_per_million"].(float64); ok {
				r.InputPrice = v
			}
			if v, ok := pricing["output_per_million"].(float64); ok {
				r.OutputPrice = v
			}
		}
		if locked, ok := row["locked"].(bool); ok {
			r.Locked = locked
		}
		if reason, ok := row["locked_reason"].(string); ok {
			r.LockedReason = reason
		}
		out = append(out, r)
	}
	return out, nil
}

func modelsToItems(rows []models.Row, recents []string) []widgets.PickerItem {
	byID := make(map[string]models.Row, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}
	used := make(map[string]bool, len(recents))
	out := make([]widgets.PickerItem, 0, len(rows))

	for _, id := range recents {
		r, ok := byID[id]
		if !ok {
			continue
		}
		used[id] = true
		it := rowToItem(r)
		it.Subtitle = "★ recent · " + it.Subtitle
		out = append(out, it)
	}
	for _, r := range rows {
		if used[r.ID] {
			continue
		}
		out = append(out, rowToItem(r))
	}
	return out
}

func rowToItem(r models.Row) widgets.PickerItem {
	name := r.DisplayName
	if name == "" {
		name = r.ID
	}
	parts := []string{}
	if r.Provider != "" {
		parts = append(parts, r.Provider)
	}
	if r.ContextWindow > 0 {
		parts = append(parts, formatContextSize(r.ContextWindow))
	}
	if r.InputPrice > 0 || r.OutputPrice > 0 {
		parts = append(parts, fmt.Sprintf("$%.1f/$%.1f", r.InputPrice, r.OutputPrice))
	}
	glyphs := capabilityGlyphs(r)
	if glyphs != "" {
		parts = append(parts, glyphs)
	}
	return widgets.PickerItem{
		ID:       r.ID,
		Label:    name,
		Subtitle: joinDot(parts),
		Locked:   r.Locked,
		Reason:   r.LockedReason,
	}
}

func formatContextSize(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%dk ctx", n/1000)
	}
	return fmt.Sprintf("%d ctx", n)
}

func capabilityGlyphs(r models.Row) string {
	if r.Vision {
		return "👁"
	}
	return ""
}

func joinDot(parts []string) string {
	out := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if out != "" {
			out += " · "
		}
		out += p
	}
	return out
}

var timeNowForModels = func() time.Time { return time.Now() }

func loadAgents(ctx context.Context, client *api.Client, projectID string) ([]widgets.PickerItem, error) {
	path := "/api/v1/agents"
	if projectID != "" {
		path += "?project_id=" + projectID
	}
	var resp api.V1ListResponse
	if err := client.Get(ctx, path, nil, &resp); err != nil {
		return nil, err
	}
	out := make([]widgets.PickerItem, 0, len(resp.Data))
	for _, row := range resp.Data {
		id, _ := row["id"].(string)
		name, _ := row["name"].(string)
		desc, _ := row["description"].(string)
		out = append(out, widgets.PickerItem{ID: id, Label: name, Subtitle: desc})
	}
	return out, nil
}

func loadProjects(ctx context.Context, client *api.Client) ([]widgets.PickerItem, error) {
	var resp api.V1ListResponse
	if err := client.Get(ctx, "/api/v1/projects", nil, &resp); err != nil {
		return nil, err
	}
	out := make([]widgets.PickerItem, 0, len(resp.Data))
	for _, row := range resp.Data {
		id, _ := row["id"].(string)
		name, _ := row["name"].(string)
		slug, _ := row["slug"].(string)
		if name == "" {
			name = slug
		}
		out = append(out, widgets.PickerItem{ID: id, Label: name, Subtitle: slug})
	}
	return out, nil
}

var osStat = func(path string) (os.FileInfo, error) { return os.Stat(path) }

func persistConfigDirect(cfg cliconfig.Config, path string) error {
	return cliconfig.Save(path, cfg)
}
