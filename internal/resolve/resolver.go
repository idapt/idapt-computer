package resolve

import (
	"context"
	"fmt"
	"net/url"
	"regexp"

	"github.com/idapt/idapt-cli/internal/api"
)

var resourceIdRegex = regexp.MustCompile(`^[0-9a-hjkmnp-tv-z]{26}$`)

func IsResourceId(s string) bool {
	return resourceIdRegex.MatchString(s)
}

type cacheKey struct {
	resourceType string
	name         string
	projectID    string
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

func (r *Resolver) ResolveProject(ctx context.Context, slug string) (string, error) {
	if IsResourceId(slug) {
		return slug, nil
	}
	if slug == "" {
		return "", fmt.Errorf("project is required; use --project or `idapt config set defaultProject <slug>`")
	}

	key := cacheKey{resourceType: "project", name: slug}
	if id, ok := r.cache[key]; ok {
		return id, nil
	}

	var resp api.V1ListResponse
	if err := r.client.Get(ctx, "/api/v1/projects", nil, &resp); err != nil {
		return "", err
	}
	for _, row := range resp.Data {
		rowSlug, _ := row["slug"].(string)
		if rowSlug == slug {
			id, _ := row["id"].(string)
			if id == "" {
				return "", fmt.Errorf("project %q has no id on v1 wire", slug)
			}
			r.cache[key] = id
			return id, nil
		}
	}
	return "", fmt.Errorf("project %q not found", slug)
}

func (r *Resolver) Resolve(ctx context.Context, resourceType, name, projectID string) (string, error) {
	if IsResourceId(name) {
		return name, nil
	}
	if name == "" {
		return "", fmt.Errorf("%s name cannot be empty", resourceType)
	}
	if projectID == "" {
		return "", fmt.Errorf("project is required to resolve %s %q", resourceType, name)
	}

	key := cacheKey{resourceType: resourceType, name: name, projectID: projectID}
	if id, ok := r.cache[key]; ok {
		return id, nil
	}

	apiPath, nameField, listOK := resourceV1Path(resourceType, projectID)
	if !listOK {
		return "", fmt.Errorf("resource type %q has no v1 lookup surface", resourceType)
	}

	q := url.Values{}
	if resourceType != "secret" {
		q.Set("project_id", projectID)
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
		return "", fmt.Errorf("%s %q not found in project", resourceType, name)
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("%s %q is ambiguous (%d matches)", resourceType, name, len(matches))
	}
	id, _ := matches[0]["id"].(string)
	if id == "" {
		return "", fmt.Errorf("unexpected v1 response for %s %q (no id)", resourceType, name)
	}
	r.cache[key] = id
	return id, nil
}

func resourceV1Path(resourceType, projectID string) (path, nameField string, ok bool) {
	switch resourceType {
	case "agent":
		return "/api/v1/agents", "name", true
	case "machine":
		return "/api/v1/machines", "name", true
	case "script":
		return "/api/v1/files", "name", true
	case "file":
		return "/api/v1/files", "name", true
	case "secret":
		return "/api/v1/projects/" + projectID + "/secrets", "name", true
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
