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

var projectCmd = &cobra.Command{
	Use:     "project",
	Aliases: []string{"proj"},
	Short:   "Manage projects",
}

var projectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List projects",
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		var resp api.V1ListResponse
		if err := client.Get(cmd.Context(), "/api/v1/projects", nil, &resp); err != nil {
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

var projectCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a project",
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
		if err := client.Post(cmd.Context(), "/api/v1/projects", body, &resp); err != nil {
			return err
		}
		return writeProjectItem(f, resp.Data)
	},
}

var projectGetCmd = &cobra.Command{
	Use:   "get <id-or-slug>",
	Short: "Get project details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		id, err := resolveProjectArg(cmd, f, args[0])
		if err != nil {
			return err
		}
		var resp api.V1ItemResponse
		if err := client.Get(cmd.Context(), "/api/v1/projects/"+id, nil, &resp); err != nil {
			return err
		}
		return writeProjectItem(f, resp.Data)
	},
}

var projectEditCmd = &cobra.Command{
	Use:   "edit <id-or-slug>",
	Short: "Edit a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		id, err := resolveProjectArg(cmd, f, args[0])
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
		if err := client.Patch(cmd.Context(), "/api/v1/projects/"+id, body, &resp); err != nil {
			return err
		}
		return writeProjectItem(f, resp.Data)
	},
}

var projectDeleteCmd = &cobra.Command{
	Use:   "delete <id-or-slug>",
	Short: "Delete a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		id, err := resolveProjectArg(cmd, f, args[0])
		if err != nil {
			return err
		}
		if !globalFlags.Confirm {
			if !cmdutil.ConfirmAction(f, fmt.Sprintf("Delete project %s?", args[0])) {
				return fmt.Errorf("aborted")
			}
		}
		if err := client.Delete(cmd.Context(), "/api/v1/projects/"+id); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Project %s deleted.\n", args[0])
		return nil
	},
}

var projectForkCmd = &cobra.Command{
	Use:   "fork <id-or-slug>",
	Short: "Fork a project (deep copy)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		id, err := resolveProjectArg(cmd, f, args[0])
		if err != nil {
			return err
		}
		body := map[string]interface{}{}
		if cmd.Flags().Changed("name") {
			v, _ := cmd.Flags().GetString("name")
			body["name"] = v
		}
		if cmd.Flags().Changed("slug") {
			v, _ := cmd.Flags().GetString("slug")
			body["slug"] = v
		}
		if cmd.Flags().Changed("include-chats") {
			v, _ := cmd.Flags().GetBool("include-chats")
			body["include_chats"] = v
		}
		if cmd.Flags().Changed("resource-types") {
			vs, _ := cmd.Flags().GetStringSlice("resource-types")
			body["resource_types"] = vs
		}
		var resp api.V1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/projects/"+id+"/fork", body, &resp); err != nil {
			return err
		}
		return writeProjectItem(f, resp.Data)
	},
}

var projectInvitationCmd = &cobra.Command{
	Use:     "invitation",
	Aliases: []string{"invite"},
	Short:   "Manage project invitations (by slug)",
}

var projectInvitationListCmd = &cobra.Command{
	Use:   "list <project-id-or-slug>",
	Short: "List pending invitations for a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		id, err := resolveProjectArg(cmd, f, args[0])
		if err != nil {
			return err
		}
		var resp api.V1ListResponse
		if err := client.Get(cmd.Context(), "/api/v1/projects/"+id+"/invitations", nil, &resp); err != nil {
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

var projectInvitationCreateCmd = &cobra.Command{
	Use:   "create <project-id-or-slug>",
	Short: "Invite an existing idapt user (by slug) to a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		id, err := resolveProjectArg(cmd, f, args[0])
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
		if err := client.Post(cmd.Context(), "/api/v1/projects/"+id+"/invitations", body, &resp); err != nil {
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

var projectInvitationDeleteCmd = &cobra.Command{
	Use:   "delete <project-id-or-slug>",
	Short: "Revoke a pending project invitation (by invitee slug)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		id, err := resolveProjectArg(cmd, f, args[0])
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
			"/api/v1/projects/"+id+"/invitations", nil, api.WithQuery(q))
		if err != nil {
			return err
		}
		resp.Body.Close()
		fmt.Fprintf(cmd.OutOrStdout(), "Invitation for %s revoked.\n", slug)
		return nil
	},
}

var projectMemberCmd = &cobra.Command{
	Use:   "member",
	Short: "Manage project members",
}

var projectMemberListCmd = &cobra.Command{
	Use:   "list <project-id-or-slug>",
	Short: "List project members",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		id, err := resolveProjectArg(cmd, f, args[0])
		if err != nil {
			return err
		}
		var resp api.V1ListResponse
		if err := client.Get(cmd.Context(), "/api/v1/projects/"+id+"/members", nil, &resp); err != nil {
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

var projectMemberAddCmd = &cobra.Command{
	Use:   "add <project-id-or-slug>",
	Short: "Add a member to a project (by actor_id)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		id, err := resolveProjectArg(cmd, f, args[0])
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
		if err := client.Post(cmd.Context(), "/api/v1/projects/"+id+"/members", body, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteItem(resp.Data, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "ACTOR_ID", Field: "actor_id"},
			{Header: "ROLE", Field: "role"},
		})
	},
}

var projectMemberRemoveCmd = &cobra.Command{
	Use:   "remove <project-id-or-slug> <member-id>",
	Short: "Remove a member from a project",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		id, err := resolveProjectArg(cmd, f, args[0])
		if err != nil {
			return err
		}
		if !globalFlags.Confirm {
			if !cmdutil.ConfirmAction(f, fmt.Sprintf("Remove member %s?", args[1])) {
				return fmt.Errorf("aborted")
			}
		}
		if err := client.Delete(cmd.Context(), "/api/v1/projects/"+id+"/members/"+args[1]); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Member removed.")
		return nil
	},
}

var projectMemberEditCmd = &cobra.Command{
	Use:   "edit <project-id-or-slug> <member-id>",
	Short: "Edit a member's role",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		id, err := resolveProjectArg(cmd, f, args[0])
		if err != nil {
			return err
		}
		role, _ := cmd.Flags().GetString("role")
		if role == "" {
			return fmt.Errorf("--role is required")
		}
		body := map[string]interface{}{"role": role}
		var resp api.V1ItemResponse
		if err := client.Patch(cmd.Context(), "/api/v1/projects/"+id+"/members/"+args[1], body, &resp); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Member role updated.")
		return nil
	},
}

func resolveProjectArg(cmd *cobra.Command, f *cmdutil.Factory, idOrSlug string) (string, error) {
	if resolve.IsResourceId(idOrSlug) {
		return idOrSlug, nil
	}
	r, err := f.Resolver()
	if err != nil {
		return "", err
	}
	return r.ResolveProject(cmd.Context(), idOrSlug)
}

func resolveProjectFlag(cmd *cobra.Command, f *cmdutil.Factory) (string, error) {
	project := globalFlags.Project
	if project == "" {
		project = f.Config.DefaultProject
	}
	if project == "" {
		return "", fmt.Errorf("--project is required (or set defaultProject via `idapt config set defaultProject <slug>`)")
	}
	if resolve.IsResourceId(project) {
		return project, nil
	}
	r, err := f.Resolver()
	if err != nil {
		return "", err
	}
	return r.ResolveProject(cmd.Context(), project)
}

func resolveResource(cmd *cobra.Command, f *cmdutil.Factory, resourceType, nameOrID, projectID string) (string, error) {
	if resolve.IsResourceId(nameOrID) {
		return nameOrID, nil
	}
	r, err := f.Resolver()
	if err != nil {
		return "", err
	}
	return r.Resolve(cmd.Context(), resourceType, nameOrID, projectID)
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

func writeProjectItem(f *cmdutil.Factory, item map[string]interface{}) error {
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
	projectCreateCmd.Flags().String("name", "", "Project name")
	projectCreateCmd.Flags().String("slug", "", "Project slug")
	projectCreateCmd.Flags().String("description", "", "Project description")
	projectCreateCmd.Flags().String("icon", "", "Project icon emoji")
	cmdutil.AddJSONInput(projectCreateCmd)

	projectEditCmd.Flags().String("name", "", "Project name")
	projectEditCmd.Flags().String("slug", "", "Project slug")
	projectEditCmd.Flags().String("description", "", "Project description")
	projectEditCmd.Flags().String("icon", "", "Project icon emoji")
	cmdutil.AddJSONInput(projectEditCmd)

	projectForkCmd.Flags().String("name", "", "New project name")
	projectForkCmd.Flags().String("slug", "", "New project slug")
	projectForkCmd.Flags().Bool("include-chats", false, "Also fork chats from the source project")
	projectForkCmd.Flags().StringSlice("resource-types", nil, "Limit the fork to specific resource types (agents,skills,kbs,scripts,files,tasks)")

	projectInvitationCreateCmd.Flags().String("slug", "", "Invitee's profile slug")
	projectInvitationCreateCmd.Flags().String("role", "viewer", "Member role (admin | editor | viewer)")

	projectInvitationDeleteCmd.Flags().String("slug", "", "Invitee's profile slug (the invitation key)")

	projectInvitationCmd.AddCommand(projectInvitationListCmd)
	projectInvitationCmd.AddCommand(projectInvitationCreateCmd)
	projectInvitationCmd.AddCommand(projectInvitationDeleteCmd)

	projectMemberAddCmd.Flags().String("actor-id", "", "Target user's profile resourceId")
	projectMemberAddCmd.Flags().String("role", "viewer", "Member role (viewer, editor, admin)")
	projectMemberEditCmd.Flags().String("role", "", "New role (viewer, editor, admin)")

	projectMemberCmd.AddCommand(projectMemberListCmd)
	projectMemberCmd.AddCommand(projectMemberAddCmd)
	projectMemberCmd.AddCommand(projectMemberRemoveCmd)
	projectMemberCmd.AddCommand(projectMemberEditCmd)

	projectCmd.AddCommand(projectListCmd)
	projectCmd.AddCommand(projectCreateCmd)
	projectCmd.AddCommand(projectGetCmd)
	projectCmd.AddCommand(projectEditCmd)
	projectCmd.AddCommand(projectDeleteCmd)
	projectCmd.AddCommand(projectForkCmd)
	projectCmd.AddCommand(projectInvitationCmd)
	projectCmd.AddCommand(projectMemberCmd)
}
