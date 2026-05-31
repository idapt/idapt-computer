package cmd

import (
	"fmt"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/spf13/cobra"
)

var apikeyCmd = &cobra.Command{
	Use:     "apikey",
	Aliases: []string{"api-key"},
	Short:   "Manage API keys",
}

var apikeyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List API keys",
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		var resp api.V1ListResponse
		if err := client.Get(cmd.Context(), "/api/v1/api-keys", nil, &resp); err != nil {
			return err
		}
		return f.ListFormatter("No API keys found.").WriteList(resp.Data, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "NAME", Field: "name"},
			{Header: "PREFIX", Field: "prefix"},
			{Header: "PREVIEW", Field: "key_preview"},
			{Header: "ENABLED", Field: "enabled"},
			{Header: "EXPIRES", Field: "expires_at"},
			{Header: "LAST USED", Field: "last_used_at"},
			{Header: "CREATED", Field: "created_at"},
		})
	},
}

var apikeyCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an API key",
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		name, _ := cmd.Flags().GetString("name")
		if name == "" {
			return fmt.Errorf("--name is required")
		}

		body := map[string]interface{}{"name": name}
		if cmd.Flags().Changed("permissions") {
			v, _ := cmd.Flags().GetString("permissions")
			body["permissions"] = []map[string]interface{}{
				{"resource": "*", "access": v},
			}
		}
		if cmd.Flags().Changed("expires-in") {
			v, _ := cmd.Flags().GetInt("expires-in")
			body["expires_in"] = v
		}

		var resp api.V1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/api-keys", body, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteItem(resp.Data, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "NAME", Field: "name"},
			{Header: "KEY (save now — shown only once)", Field: "key"},
			{Header: "EXPIRES", Field: "expires_at"},
		})
	},
}

var apikeyDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete (revoke) an API key",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		if !globalFlags.Confirm {
			if !cmdutil.ConfirmAction(f, fmt.Sprintf("Delete API key %s?", args[0])) {
				return fmt.Errorf("aborted")
			}
		}
		if err := client.Delete(cmd.Context(), "/api/v1/api-keys/"+args[0]); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "API key deleted.")
		return nil
	},
}

func init() {
	apikeyCreateCmd.Flags().String("name", "", "API key name")
	apikeyCreateCmd.Flags().String("permissions", "", "Permission level (read | write | admin)")
	apikeyCreateCmd.Flags().Int("expires-in", 0, "Seconds until expiration (0 for tier-max)")

	apikeyCmd.AddCommand(apikeyListCmd)
	apikeyCmd.AddCommand(apikeyCreateCmd)
	apikeyCmd.AddCommand(apikeyDeleteCmd)
}
