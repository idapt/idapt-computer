package cmd

import (
	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/resolve"
	"github.com/spf13/cobra"
)

const DefaultWorkspaceSlug = "personal"

type v1ItemResponse = api.V1ItemResponse

func resolveWorkspaceArg(cmd *cobra.Command, f *cmdutil.Factory, idOrSlug string) (string, error) {
	if resolve.IsResourceId(idOrSlug) {
		return idOrSlug, nil
	}
	r, err := f.Resolver()
	if err != nil {
		return "", err
	}
	return r.ResolveWorkspace(cmd.Context(), idOrSlug)
}

func resolveWorkspaceFlag(cmd *cobra.Command, f *cmdutil.Factory) (string, error) {
	workspace := globalFlags.Workspace
	if workspace == "" {
		workspace = f.Config.DefaultWorkspace
	}
	if workspace == "" {
		workspace = DefaultWorkspaceSlug
	}
	if resolve.IsResourceId(workspace) {
		return workspace, nil
	}
	r, err := f.Resolver()
	if err != nil {
		return "", err
	}
	return r.ResolveWorkspace(cmd.Context(), workspace)
}

func resolveResource(cmd *cobra.Command, f *cmdutil.Factory, resourceType, nameOrID, workspaceID string) (string, error) {
	if resolve.IsResourceId(nameOrID) {
		return nameOrID, nil
	}
	r, err := f.Resolver()
	if err != nil {
		return "", err
	}
	return r.Resolve(cmd.Context(), resourceType, nameOrID, workspaceID)
}

func resolveComputer(cmd *cobra.Command, f *cmdutil.Factory, nameOrID string) (string, error) {
	if resolve.IsResourceId(nameOrID) {
		return nameOrID, nil
	}
	workspaceID, err := resolveWorkspaceFlag(cmd, f)
	if err != nil {
		return "", err
	}
	return resolveResource(cmd, f, "computer", nameOrID, workspaceID)
}

func validEffortLevel(s string) bool {
	switch s {
	case "fast", "smart", "expert":
		return true
	}
	return false
}
