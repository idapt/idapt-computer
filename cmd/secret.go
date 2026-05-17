package cmd

import (
	"fmt"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/input"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/spf13/cobra"
)

var secretCmd = &cobra.Command{
	Use:   "secret",
	Short: "Manage project secrets",
}

var secretListCmd = &cobra.Command{
	Use:   "list",
	Short: "List secrets in the current project",
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		projectID, err := resolveProjectFlag(cmd, f)
		if err != nil {
			return err
		}
		var resp api.V1ListResponse
		if err := client.Get(cmd.Context(), "/api/v1/projects/"+projectID+"/secrets", nil, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteList(resp.Data, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "NAME", Field: "name"},
			{Header: "DESCRIPTION", Field: "description"},
			{Header: "CREATED", Field: "created_at"},
		})
	},
}

var secretCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a secret",
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		projectID, err := resolveProjectFlag(cmd, f)
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
		if cmd.Flags().Changed("value") {
			v, _ := cmd.Flags().GetString("value")
			overrides["value"] = v
		}
		if cmd.Flags().Changed("value-file") {
			path, _ := cmd.Flags().GetString("value-file")
			content, err := input.ReadFileFlag(path)
			if err != nil {
				return err
			}
			overrides["value"] = content
		}
		if cmd.Flags().Changed("description") {
			v, _ := cmd.Flags().GetString("description")
			overrides["description"] = v
		}
		body = input.MergeFlags(body, overrides)
		if _, ok := body["name"]; !ok {
			return fmt.Errorf("--name is required")
		}
		if _, ok := body["value"]; !ok {
			return fmt.Errorf("--value or --value-file is required")
		}

		var resp api.V1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/projects/"+projectID+"/secrets", body, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteItem(resp.Data, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "NAME", Field: "name"},
			{Header: "DESCRIPTION", Field: "description"},
		})
	},
}

var secretGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get secret metadata (value is never returned on read)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		projectID, err := resolveProjectFlag(cmd, f)
		if err != nil {
			return err
		}
		var resp api.V1ItemResponse
		if err := client.Get(cmd.Context(), "/api/v1/projects/"+projectID+"/secrets/"+args[0], nil, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteItem(resp.Data, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "NAME", Field: "name"},
			{Header: "DESCRIPTION", Field: "description"},
			{Header: "CREATED", Field: "created_at"},
			{Header: "UPDATED", Field: "updated_at"},
		})
	},
}

var secretEditCmd = &cobra.Command{
	Use:   "edit <id>",
	Short: "Edit a secret",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		projectID, err := resolveProjectFlag(cmd, f)
		if err != nil {
			return err
		}

		body := map[string]interface{}{}
		if cmd.Flags().Changed("value") {
			v, _ := cmd.Flags().GetString("value")
			body["value"] = v
		}
		if cmd.Flags().Changed("value-file") {
			path, _ := cmd.Flags().GetString("value-file")
			content, err := input.ReadFileFlag(path)
			if err != nil {
				return err
			}
			body["value"] = content
		}
		if cmd.Flags().Changed("description") {
			v, _ := cmd.Flags().GetString("description")
			body["description"] = v
		}
		if len(body) == 0 {
			return fmt.Errorf("nothing to update (pass --value, --value-file, or --description)")
		}

		if err := client.Patch(cmd.Context(), "/api/v1/projects/"+projectID+"/secrets/"+args[0], body, nil); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Secret updated.")
		return nil
	},
}

var secretDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a secret",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		projectID, err := resolveProjectFlag(cmd, f)
		if err != nil {
			return err
		}
		if !globalFlags.Confirm {
			if !cmdutil.ConfirmAction(f, fmt.Sprintf("Delete secret %s?", args[0])) {
				return fmt.Errorf("aborted")
			}
		}
		if err := client.Delete(cmd.Context(), "/api/v1/projects/"+projectID+"/secrets/"+args[0]); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Secret %s deleted.\n", args[0])
		return nil
	},
}

func init() {
	secretCreateCmd.Flags().String("name", "", "Secret name (e.g. STRIPE_API_KEY)")
	secretCreateCmd.Flags().String("value", "", "Secret value")
	secretCreateCmd.Flags().String("value-file", "", "Path to file containing secret value")
	secretCreateCmd.Flags().String("description", "", "Description")
	cmdutil.AddJSONInput(secretCreateCmd)

	secretEditCmd.Flags().String("value", "", "New secret value")
	secretEditCmd.Flags().String("value-file", "", "Path to file containing new secret value")
	secretEditCmd.Flags().String("description", "", "Description")

	secretCmd.AddCommand(secretListCmd)
	secretCmd.AddCommand(secretCreateCmd)
	secretCmd.AddCommand(secretGetCmd)
	secretCmd.AddCommand(secretEditCmd)
	secretCmd.AddCommand(secretDeleteCmd)
}
