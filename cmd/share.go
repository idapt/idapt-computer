package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/spf13/cobra"
)

var shareCmd = &cobra.Command{
	Use:   "share",
	Short: "Manage resource sharing (chats, files, agents, projects)",
}

var shareListCmd = &cobra.Command{
	Use:   "list",
	Short: "List resources shared with you",
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		q := url.Values{}
		if cmd.Flags().Changed("type") {
			t, _ := cmd.Flags().GetString("type")
			q.Set("resource_type", t)
		}
		var resp api.V1ListResponse
		if err := client.Get(cmd.Context(), "/api/v1/shared-with-me", q, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteList(resp.Data, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "TYPE", Field: "resource_type"},
			{Header: "NAME", Field: "name"},
			{Header: "PERMISSION", Field: "permission"},
			{Header: "SHARED BY", Field: "shared_by_actor_id"},
			{Header: "SHARED AT", Field: "shared_at"},
		})
	},
}

var shareGranteesCmd = &cobra.Command{
	Use:   "grantees <resource-type> <resource-id>",
	Short: "List who can access a resource",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		q := url.Values{
			"resource_type": {args[0]},
			"resource_id":   {args[1]},
		}
		var resp api.V1ListResponse
		if err := client.Get(cmd.Context(), "/api/v1/shares", q, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteList(resp.Data, []output.Column{
			{Header: "GRANTEE_ACTOR_ID", Field: "grantee_actor_id"},
			{Header: "PERMISSION", Field: "permission"},
			{Header: "SHARED BY", Field: "shared_by_actor_id"},
			{Header: "SHARED AT", Field: "shared_at"},
		})
	},
}

var shareCreateCmd = &cobra.Command{
	Use:   "create <resource-type> <resource-id>",
	Short: "Grant another idapt user access to a resource",
	Long:  "resource-type: chat | agent | file | project. Pass --actor-id (the grantee's profile resourceId) and --permission read|write|admin.",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		actorID, _ := cmd.Flags().GetString("actor-id")
		permission, _ := cmd.Flags().GetString("permission")
		if actorID == "" {
			return fmt.Errorf("--actor-id is required (target user's profile resourceId)")
		}
		body := map[string]interface{}{
			"resource_type":    args[0],
			"resource_id":      args[1],
			"grantee_actor_id": actorID,
			"permission":       permission,
		}
		var resp api.V1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/shares", body, &resp); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Shared %s %s with %s (%s).\n",
			args[0], args[1], actorID, permission)
		return nil
	},
}

var shareUpdateCmd = &cobra.Command{
	Use:   "update <resource-type> <resource-id>",
	Short: "Change a grantee's permission on a resource",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		actorID, _ := cmd.Flags().GetString("actor-id")
		permission, _ := cmd.Flags().GetString("permission")
		if actorID == "" || permission == "" {
			return fmt.Errorf("--actor-id and --permission are required")
		}
		q := url.Values{
			"resource_type":    {args[0]},
			"resource_id":      {args[1]},
			"grantee_actor_id": {actorID},
		}
		bodyBytes, err := json.Marshal(map[string]interface{}{"permission": permission})
		if err != nil {
			return fmt.Errorf("marshaling body: %w", err)
		}
		resp, err := client.Do(cmd.Context(), "PATCH", "/api/v1/shares",
			bytes.NewReader(bodyBytes),
			api.WithQuery(q),
			api.WithHeader("Content-Type", "application/json"),
		)
		if err != nil {
			return err
		}
		resp.Body.Close()
		fmt.Fprintln(cmd.OutOrStdout(), "Share updated.")
		return nil
	},
}

var shareDeleteCmd = &cobra.Command{
	Use:   "delete <resource-type> <resource-id>",
	Short: "Revoke a grantee's access",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		actorID, _ := cmd.Flags().GetString("actor-id")
		if actorID == "" {
			return fmt.Errorf("--actor-id is required (grantee profile resourceId)")
		}
		q := url.Values{
			"resource_type":    {args[0]},
			"resource_id":      {args[1]},
			"grantee_actor_id": {actorID},
		}
		resp, err := client.Do(cmd.Context(), "DELETE", "/api/v1/shares", nil, api.WithQuery(q))
		if err != nil {
			return err
		}
		resp.Body.Close()
		fmt.Fprintf(cmd.OutOrStdout(), "Share revoked: %s %s for %s.\n", args[0], args[1], actorID)
		return nil
	},
}

func init() {
	shareListCmd.Flags().String("type", "", "Filter by resource type (chat|agent|file|project)")

	shareCreateCmd.Flags().String("actor-id", "", "Grantee profile resourceId")
	shareCreateCmd.Flags().String("permission", "read", "Permission level (read|write|admin)")

	shareUpdateCmd.Flags().String("actor-id", "", "Grantee profile resourceId")
	shareUpdateCmd.Flags().String("permission", "", "New permission (read|write|admin)")

	shareDeleteCmd.Flags().String("actor-id", "", "Grantee profile resourceId")

	shareCmd.AddCommand(shareListCmd)
	shareCmd.AddCommand(shareGranteesCmd)
	shareCmd.AddCommand(shareCreateCmd)
	shareCmd.AddCommand(shareUpdateCmd)
	shareCmd.AddCommand(shareDeleteCmd)
}
