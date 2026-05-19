package cmd

import (
	"fmt"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/spf13/cobra"
)

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage your profile",
}

var profileGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get your profile",
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		var resp api.V1ItemResponse
		if err := client.Get(cmd.Context(), "/api/v1/me", nil, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteItem(resp.Data, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "EMAIL", Field: "email"},
			{Header: "NAME", Field: "name"},
			{Header: "SLUG", Field: "slug"},
			{Header: "CREATED", Field: "created_at"},
		})
	},
}

var profileEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit your profile",
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
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
		if len(body) == 0 {
			return fmt.Errorf("at least one of --name or --slug is required")
		}

		if err := client.Patch(cmd.Context(), "/api/v1/settings", body, nil); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Profile updated.")
		return nil
	},
}

func init() {
	profileEditCmd.Flags().String("name", "", "Display name")
	profileEditCmd.Flags().String("slug", "", "Profile slug/username")

	profileCmd.AddCommand(profileGetCmd)
	profileCmd.AddCommand(profileEditCmd)
}
