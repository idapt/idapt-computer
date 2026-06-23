
package cmd

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/idapt/idapt-computer/internal/api"
	"github.com/idapt/idapt-computer/internal/cliconfig"
	"github.com/idapt/idapt-computer/internal/cmdutil"
	"github.com/spf13/cobra"
)

const completionTimeout = 2 * time.Second

const completionCacheTTL = 30 * time.Second

var (
	completionCacheMu sync.Mutex
	completionCacheM  = map[string]completionCacheEntry{}
)

func resetCompletionCache() {
	completionCacheMu.Lock()
	defer completionCacheMu.Unlock()
	completionCacheM = map[string]completionCacheEntry{}
}

type completionCacheEntry struct {
	expires time.Time
	items   []map[string]any
}

func fetchList(cmd *cobra.Command, path string, query url.Values) ([]map[string]any, error) {
	cacheKey := path
	if query != nil {
		cacheKey += "?" + query.Encode()
	}

	completionCacheMu.Lock()
	if entry, ok := completionCacheM[cacheKey]; ok && time.Now().Before(entry.expires) {
		completionCacheMu.Unlock()
		return entry.items, nil
	}
	completionCacheMu.Unlock()

	f := cmdutil.FactoryFromCmd(cmd)
	if f == nil {
		return nil, fmt.Errorf("no factory in command context")
	}
	client, err := f.APIClient()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), completionTimeout)
	defer cancel()

	var resp api.V1ListResponse
	if err := client.Get(ctx, path, query, &resp); err != nil {
		return nil, err
	}

	completionCacheMu.Lock()
	completionCacheM[cacheKey] = completionCacheEntry{
		expires: time.Now().Add(completionCacheTTL),
		items:   resp.Data,
	}
	completionCacheMu.Unlock()

	return resp.Data, nil
}

func buildCompletions(items []map[string]any, idField, descField, toComplete string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		id, _ := item[idField].(string)
		if id == "" {
			continue
		}
		if toComplete != "" && !strings.HasPrefix(id, toComplete) {
			continue
		}
		if descField != "" {
			if desc, _ := item[descField].(string); desc != "" {
				out = append(out, fmt.Sprintf("%s\t%s", id, desc))
				continue
			}
		}
		out = append(out, id)
	}
	return out
}
func completeChatIDs(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	q := url.Values{"limit": {"50"}}
	items, err := fetchList(cmd, "/api/v1/chats", q)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveError
	}
	return buildCompletions(items, "id", "title", toComplete), cobra.ShellCompDirectiveNoFileComp
}

func completeAgentIDs(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	q := url.Values{"limit": {"50"}}
	items, err := fetchList(cmd, "/api/v1/agents", q)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveError
	}
	out := buildCompletions(items, "id", "name", toComplete)
	for _, item := range items {
		name, _ := item["name"].(string)
		if name == "" {
			continue
		}
		if toComplete != "" && !strings.HasPrefix(name, toComplete) {
			continue
		}
		out = append(out, name)
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

func completeWorkspaceIDs(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	q := url.Values{"limit": {"50"}}
	items, err := fetchList(cmd, "/api/v1/workspaces", q)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveError
	}
	out := buildCompletions(items, "id", "name", toComplete)
	for _, item := range items {
		slug, _ := item["slug"].(string)
		if slug == "" {
			continue
		}
		if toComplete != "" && !strings.HasPrefix(slug, toComplete) {
			continue
		}
		out = append(out, slug)
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

func completeModelIDs(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	items, err := fetchList(cmd, "/api/v1/models", nil)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveError
	}
	out := []string{}
	if toComplete == "" || strings.HasPrefix("auto", toComplete) {
		out = append(out, "auto\tcheapest-first auto-router")
	}
	out = append(out, buildCompletions(items, "id", "name", toComplete)...)
	return out, cobra.ShellCompDirectiveNoFileComp
}

func completeTriggerIDs(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	q := url.Values{"limit": {"50"}}
	items, err := fetchList(cmd, "/api/v1/triggers", q)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveError
	}
	return buildCompletions(items, "id", "name", toComplete), cobra.ShellCompDirectiveNoFileComp
}

func completeEffortLevel(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	choices := []string{"fast", "smart", "expert"}
	out := []string{}
	for _, c := range choices {
		if toComplete == "" || strings.HasPrefix(c, toComplete) {
			out = append(out, c)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

func completeConfigKeys(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) >= 1 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	out := []string{}
	for _, k := range cliconfig.Keys() {
		if toComplete == "" || strings.HasPrefix(k, toComplete) {
			out = append(out, k)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

func completeOutputFormat(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	choices := []string{"table", "json", "jsonl", "quiet"}
	out := []string{}
	for _, c := range choices {
		if toComplete == "" || strings.HasPrefix(c, toComplete) {
			out = append(out, c)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}
