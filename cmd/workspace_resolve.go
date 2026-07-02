package cmd

import (
	"fmt"

	"github.com/idapt/idapt-computer/internal/api"
	"github.com/idapt/idapt-computer/internal/resolve"
	"github.com/spf13/cobra"
)

func resolveWorkspaceID(cmd *cobra.Command, client *api.Client, workspace string) (string, error) {
	if resolve.IsResourceId(workspace) {
		return workspace, nil
	}

	var resp api.V1ListResponse
	if err := client.Get(cmd.Context(), "/api/v1/workspaces", nil, &resp); err != nil {
		return "", err
	}
	for _, row := range resp.Data {
		id, _ := row["id"].(string)
		slug, _ := row["slug"].(string)
		if slug == workspace || id == workspace {
			return id, nil
		}
	}
	return "", fmt.Errorf("workspace %q not found", workspace)
}
