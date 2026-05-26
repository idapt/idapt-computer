package cmd

import (
	"fmt"
	"net/url"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/input"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/idapt/idapt-cli/internal/resolve"
	"github.com/spf13/cobra"
)

var workspaceCmd = &cobra.Command{
	Use:   "workspace",
	Short: "Manage workspaces",
}

var workspaceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List workspaces",
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		var resp api.V1ListResponse
		if err := client.Get(cmd.Context(), "/api/v1/workspaces", nil, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteList(resp.Data, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "NAME", Field: "name"},
			{Header: "SLUG", Field: "slug"},
			{Header: "ICON", Field: "icon"},
			{Header: "IS_PERSONAL", Field: "is_personal"},
			{Header: "PUBLIC", Field: "public_access"},
		})
	},
}

var workspaceCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a workspace",
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		body := map[string]interface{}{}
		if cmd.Flags().Changed("json") {
			raw, _ := cmd.Flags().GetString("json")
			parsed, err := input.ParseJSONFlag(raw, f.In)
			if err != nil {
				return err
			}
			body = parsed
		}

		overrides := map[string]interface{}{}
		if cmd.Flags().Changed("name") {
			v, _ := cmd.Flags().GetString("name")
			overrides["name"] = v
		}
		if cmd.Flags().Changed("slug") {
			v, _ := cmd.Flags().GetString("slug")
			overrides["slug"] = v
		}
		if cmd.Flags().Changed("description") {
			v, _ := cmd.Flags().GetString("description")
			overrides["description"] = v
		}
		if cmd.Flags().Changed("icon") {
			v, _ := cmd.Flags().GetString("icon")
			overrides["icon"] = v
		}
		body = input.MergeFlags(body, overrides)

		var resp api.V1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/workspaces", body, &resp); err != nil {
			return err
		}
		return writeWorkspaceItem(f, resp.Data)
	},
}

var workspaceGetCmd = &cobra.Command{
	Use:   "get <id-or-slug>",
	Short: "Get workspace details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		id, err := resolveWorkspaceArg(cmd, f, args[0])
		if err != nil {
			return err
		}
		var resp api.V1ItemResponse
		if err := client.Get(cmd.Context(), "/api/v1/workspaces/"+id, nil, &resp); err != nil {
			return err
		}
		return writeWorkspaceItem(f, resp.Data)
	},
}

var workspaceEditCmd = &cobra.Command{
	Use:   "edit <id-or-slug>",
	Short: "Edit a workspace",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		id, err := resolveWorkspaceArg(cmd, f, args[0])
		if err != nil {
			return err
		}

		body := map[string]interface{}{}
		if cmd.Flags().Changed("json") {
			raw, _ := cmd.Flags().GetString("json")
			parsed, err := input.ParseJSONFlag(raw, f.In)
			if err != nil {
				return err
			}
			body = parsed
		}
		overrides := map[string]interface{}{}
		if cmd.Flags().Changed("name") {
			v, _ := cmd.Flags().GetString("name")
			overrides["name"] = v
		}
		if cmd.Flags().Changed("slug") {
			v, _ := cmd.Flags().GetString("slug")
			overrides["slug"] = v
		}
		if cmd.Flags().Changed("description") {
			v, _ := cmd.Flags().GetString("description")
			overrides["description"] = v
		}
		if cmd.Flags().Changed("icon") {
			v, _ := cmd.Flags().GetString("icon")
			overrides["icon"] = v
		}
		body = input.MergeFlags(body, overrides)

		var resp api.V1ItemResponse
		if err := client.Patch(cmd.Context(), "/api/v1/workspaces/"+id, body, &resp); err != nil {
			return err
		}
		return writeWorkspaceItem(f, resp.Data)
	},
}

var workspaceDeleteCmd = &cobra.Command{
	Use:   "delete <id-or-slug>",
	Short: "Delete a workspace",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		id, err := resolveWorkspaceArg(cmd, f, args[0])
		if err != nil {
			return err
		}
		if !globalFlags.Confirm {
			if !cmdutil.ConfirmAction(f, fmt.Sprintf("Delete workspace %s?", args[0])) {
				return fmt.Errorf("aborted")
			}
		}
		if err := client.Delete(cmd.Context(), "/api/v1/workspaces/"+id); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Workspace %s deleted.\n", args[0])
		return nil
	},
}

var workspaceInvitationCmd = &cobra.Command{
	Use:     "invitation",
	Aliases: []string{"invite"},
	Short:   "Manage workspace invitations (by slug)",
}

var workspaceInvitationListCmd = &cobra.Command{
	Use:   "list <workspace-id-or-slug>",
	Short: "List pending invitations for a workspace",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		id, err := resolveWorkspaceArg(cmd, f, args[0])
		if err != nil {
			return err
		}
		var resp api.V1ListResponse
		if err := client.Get(cmd.Context(), "/api/v1/workspaces/"+id+"/invitations", nil, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteList(resp.Data, []output.Column{
			{Header: "INVITEE SLUG", Field: "invitee_slug"},
			{Header: "INVITEE NAME", Field: "invitee_display_name"},
			{Header: "STATUS", Field: "status"},
			{Header: "EXPIRES", Field: "expires_at"},
			{Header: "CREATED", Field: "created_at"},
		})
	},
}

var workspaceInvitationCreateCmd = &cobra.Command{
	Use:   "create <workspace-id-or-slug>",
	Short: "Invite an existing idapt user (by slug) to a workspace",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		id, err := resolveWorkspaceArg(cmd, f, args[0])
		if err != nil {
			return err
		}
		slug, _ := cmd.Flags().GetString("slug")
		role, _ := cmd.Flags().GetString("role")
		if slug == "" {
			return fmt.Errorf("--slug is required (the invitee's profile slug)")
		}
		body := map[string]interface{}{"slug": slug}
		if role != "" {
			body["role"] = role
		}
		var resp api.V1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/workspaces/"+id+"/invitations", body, &resp); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Invitation sent to %s.\n", slug)
		return f.Formatter().WriteItem(resp.Data, []output.Column{
			{Header: "INVITEE SLUG", Field: "invitee_slug"},
			{Header: "STATUS", Field: "status"},
			{Header: "ROLE", Field: "role"},
		})
	},
}

var workspaceInvitationDeleteCmd = &cobra.Command{
	Use:   "delete <workspace-id-or-slug>",
	Short: "Revoke a pending workspace invitation (by invitee slug)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		id, err := resolveWorkspaceArg(cmd, f, args[0])
		if err != nil {
			return err
		}
		slug, _ := cmd.Flags().GetString("slug")
		if slug == "" {
			return fmt.Errorf("--slug is required (the invitee's profile slug)")
		}
		if !globalFlags.Confirm {
			if !cmdutil.ConfirmAction(f, fmt.Sprintf("Revoke invitation for %s?", slug)) {
				return fmt.Errorf("aborted")
			}
		}
		q := url.Values{"invitee_slug": {slug}}
		resp, err := client.Do(cmd.Context(), "DELETE",
			"/api/v1/workspaces/"+id+"/invitations", nil, api.WithQuery(q))
		if err != nil {
			return err
		}
		resp.Body.Close()
		fmt.Fprintf(cmd.OutOrStdout(), "Invitation for %s revoked.\n", slug)
		return nil
	},
}

var workspaceMemberCmd = &cobra.Command{
	Use:   "member",
	Short: "Manage workspace members",
}

var workspaceMemberListCmd = &cobra.Command{
	Use:   "list <workspace-id-or-slug>",
	Short: "List workspace members",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		id, err := resolveWorkspaceArg(cmd, f, args[0])
		if err != nil {
			return err
		}
		var resp api.V1ListResponse
		if err := client.Get(cmd.Context(), "/api/v1/workspaces/"+id+"/members", nil, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteList(resp.Data, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "ACTOR_ID", Field: "actor_id"},
			{Header: "SLUG", Field: "slug"},
			{Header: "NAME", Field: "display_name"},
			{Header: "ROLE", Field: "role"},
			{Header: "JOINED", Field: "created_at"},
		})
	},
}

var workspaceMemberAddCmd = &cobra.Command{
	Use:   "add <workspace-id-or-slug>",
	Short: "Add a member to a workspace (by actor_id)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		id, err := resolveWorkspaceArg(cmd, f, args[0])
		if err != nil {
			return err
		}
		actorID, _ := cmd.Flags().GetString("actor-id")
		role, _ := cmd.Flags().GetString("role")
		if actorID == "" {
			return fmt.Errorf("--actor-id is required (the target user's profile resourceId)")
		}
		body := map[string]interface{}{"actor_id": actorID}
		if role != "" {
			body["role"] = role
		}
		var resp api.V1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/workspaces/"+id+"/members", body, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteItem(resp.Data, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "ACTOR_ID", Field: "actor_id"},
			{Header: "ROLE", Field: "role"},
		})
	},
}

var workspaceMemberRemoveCmd = &cobra.Command{
	Use:   "remove <workspace-id-or-slug> <member-id>",
	Short: "Remove a member from a workspace",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		id, err := resolveWorkspaceArg(cmd, f, args[0])
		if err != nil {
			return err
		}
		if !globalFlags.Confirm {
			if !cmdutil.ConfirmAction(f, fmt.Sprintf("Remove member %s?", args[1])) {
				return fmt.Errorf("aborted")
			}
		}
		if err := client.Delete(cmd.Context(), "/api/v1/workspaces/"+id+"/members/"+args[1]); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Member removed.")
		return nil
	},
}

var workspaceMemberEditCmd = &cobra.Command{
	Use:   "edit <workspace-id-or-slug> <member-id>",
	Short: "Edit a member's role",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		id, err := resolveWorkspaceArg(cmd, f, args[0])
		if err != nil {
			return err
		}
		role, _ := cmd.Flags().GetString("role")
		if role == "" {
			return fmt.Errorf("--role is required")
		}
		body := map[string]interface{}{"role": role}
		var resp api.V1ItemResponse
		if err := client.Patch(cmd.Context(), "/api/v1/workspaces/"+id+"/members/"+args[1], body, &resp); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Member role updated.")
		return nil
	},
}

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
		return "", fmt.Errorf("--workspace is required (or set defaultWorkspace via `idapt config set defaultWorkspace <slug>`)")
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

func buildListQuery(cmd *cobra.Command, extra url.Values) url.Values {
	q := url.Values{}
	if cmd.Flags().Lookup("limit") != nil {
		if limit, _ := cmd.Flags().GetInt("limit"); limit > 0 {
			q.Set("limit", fmt.Sprintf("%d", limit))
		}
	}
	if cmd.Flags().Lookup("cursor") != nil {
		if cursor, _ := cmd.Flags().GetString("cursor"); cursor != "" {
			q.Set("cursor", cursor)
		}
	}
	for k, vs := range extra {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	return q
}

func writeWorkspaceItem(f *cmdutil.Factory, item map[string]interface{}) error {
	return f.Formatter().WriteItem(item, []output.Column{
		{Header: "ID", Field: "id"},
		{Header: "NAME", Field: "name"},
		{Header: "SLUG", Field: "slug"},
		{Header: "DESCRIPTION", Field: "description"},
		{Header: "ICON", Field: "icon"},
		{Header: "IS_PERSONAL", Field: "is_personal"},
		{Header: "PUBLIC", Field: "public_access"},
		{Header: "CREATED", Field: "created_at"},
	})
}

func init() {
	workspaceCreateCmd.Flags().String("name", "", "Workspace name")
	workspaceCreateCmd.Flags().String("slug", "", "Workspace slug")
	workspaceCreateCmd.Flags().String("description", "", "Workspace description")
	workspaceCreateCmd.Flags().String("icon", "", "Workspace icon emoji")
	cmdutil.AddJSONInput(workspaceCreateCmd)

	workspaceEditCmd.Flags().String("name", "", "Workspace name")
	workspaceEditCmd.Flags().String("slug", "", "Workspace slug")
	workspaceEditCmd.Flags().String("description", "", "Workspace description")
	workspaceEditCmd.Flags().String("icon", "", "Workspace icon emoji")
	cmdutil.AddJSONInput(workspaceEditCmd)

	workspaceInvitationCreateCmd.Flags().String("slug", "", "Invitee's profile slug")
	workspaceInvitationCreateCmd.Flags().String("role", "viewer", "Member role (admin | editor | viewer)")

	workspaceInvitationDeleteCmd.Flags().String("slug", "", "Invitee's profile slug (the invitation key)")

	workspaceInvitationCmd.AddCommand(workspaceInvitationListCmd)
	workspaceInvitationCmd.AddCommand(workspaceInvitationCreateCmd)
	workspaceInvitationCmd.AddCommand(workspaceInvitationDeleteCmd)

	workspaceMemberAddCmd.Flags().String("actor-id", "", "Target user's profile resourceId")
	workspaceMemberAddCmd.Flags().String("role", "viewer", "Member role (viewer, editor, admin)")
	workspaceMemberEditCmd.Flags().String("role", "", "New role (viewer, editor, admin)")

	workspaceMemberCmd.AddCommand(workspaceMemberListCmd)
	workspaceMemberCmd.AddCommand(workspaceMemberAddCmd)
	workspaceMemberCmd.AddCommand(workspaceMemberRemoveCmd)
	workspaceMemberCmd.AddCommand(workspaceMemberEditCmd)

	workspaceCmd.AddCommand(workspaceListCmd)
	workspaceCmd.AddCommand(workspaceCreateCmd)
	workspaceCmd.AddCommand(workspaceGetCmd)
	workspaceCmd.AddCommand(workspaceEditCmd)
	workspaceCmd.AddCommand(workspaceDeleteCmd)
	workspaceCmd.AddCommand(workspaceInvitationCmd)
	workspaceCmd.AddCommand(workspaceMemberCmd)
}
