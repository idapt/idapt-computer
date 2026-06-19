package resolve

import (
	"context"
	"fmt"
	"net/url"
	"regexp"

	"github.com/idapt/idapt-computer/internal/api"
)

var resourceIdRegex = regexp.MustCompile(`^[0-9a-hjkmnp-tv-z]{26}$`)

func IsResourceId(s string) bool {
	return resourceIdRegex.MatchString(s)
}

type cacheKey struct {
	resourceType string
	name         string
	workspaceID    string
}

type Resolver struct {
	client *api.Client
	cache  map[cacheKey]string
}

func New(client *api.Client) *Resolver {
	return &Resolver{
		client: client,
		cache:  make(map[cacheKey]string),
	}
}

func (r *Resolver) ResolveWorkspace(ctx context.Context, slug string) (string, error) {
	if IsResourceId(slug) {
		return slug, nil
	}
	if slug == "" {
		return "", fmt.Errorf("workspace is required; use --workspace or `idapt-computer config set defaultWorkspace <slug>`")
	}

	key := cacheKey{resourceType: "workspace", name: slug}
	if id, ok := r.cache[key]; ok {
		return id, nil
	}

	var resp api.V1ListResponse
	if err := r.client.Get(ctx, "/api/v1/workspaces", nil, &resp); err != nil {
		return "", err
	}
	for _, row := range resp.Data {
		rowSlug, _ := row["slug"].(string)
		if rowSlug == slug {
			id, _ := row["id"].(string)
			if id == "" {
				return "", fmt.Errorf("workspace %q has no id on v1 wire", slug)
			}
			r.cache[key] = id
			return id, nil
		}
	}
	return "", fmt.Errorf("workspace %q not found — run `idapt workspace list` to see your workspaces (or pass a resourceId)", slug)
}

func (r *Resolver) Resolve(ctx context.Context, resourceType, name, workspaceID string) (string, error) {
	if IsResourceId(name) {
		return name, nil
	}
	if name == "" {
		return "", fmt.Errorf("%s name cannot be empty", resourceType)
	}
	if workspaceID == "" {
		return "", fmt.Errorf("workspace is required to resolve %s %q", resourceType, name)
	}

	key := cacheKey{resourceType: resourceType, name: name, workspaceID: workspaceID}
	if id, ok := r.cache[key]; ok {
		return id, nil
	}

	apiPath, nameField, listOK := resourceV1Path(resourceType, workspaceID)
	if !listOK {
		return "", fmt.Errorf("resource type %q has no v1 lookup surface", resourceType)
	}

	q := url.Values{}
	if resourceType != "secret" {
		q.Set("workspace_id", workspaceID)
	}

	var resp api.V1ListResponse
	if err := r.client.Get(ctx, apiPath, q, &resp); err != nil {
		return "", err
	}

	matches := make([]map[string]interface{}, 0, 2)
	for _, row := range resp.Data {
		val, _ := row[nameField].(string)
		if val == name {
			matches = append(matches, row)
		}
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("%s %q not found in this workspace — check the name, pass its resourceId directly, or run the matching `list` command to see what's available", resourceType, name)
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("%s %q is ambiguous (%d matches) — pass the resourceId instead", resourceType, name, len(matches))
	}
	id, _ := matches[0]["id"].(string)
	if id == "" {
		return "", fmt.Errorf("unexpected v1 response for %s %q (no id)", resourceType, name)
	}
	r.cache[key] = id
	return id, nil
}

func resourceV1Path(resourceType, workspaceID string) (path, nameField string, ok bool) {
	switch resourceType {
	case "agent":
		return "/api/v1/agents", "name", true
	case "computer":
		return "/api/v1/computers", "name", true
	case "script":
		return "/api/v1/drive/files", "name", true
	case "file":
		return "/api/v1/drive/files", "name", true
	case "secret":
		return "/api/v1/workspaces/" + workspaceID + "/secrets", "name", true
	case "chat":
		return "/api/v1/chats", "title", true
	case "trigger":
		return "/api/v1/triggers", "name", true
	case "skill":
		return "", "", false
	default:
		return "", "", false
	}
}
