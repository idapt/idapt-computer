package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/spf13/cobra"
)

var settingsCmd = &cobra.Command{
	Use:   "settings",
	Short: "Manage account settings",
}

var settingsGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Get settings (all or a specific key)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		var resp api.V1ItemResponse
		if err := client.Get(cmd.Context(), "/api/v1/settings", nil, &resp); err != nil {
			return err
		}

		if len(args) > 0 {
			key := args[0]
			val, ok := resp.Data[key]
			if !ok {
				return fmt.Errorf("unknown setting: %s", key)
			}
			switch v := val.(type) {
			case nil:
				fmt.Fprintln(cmd.OutOrStdout(), "")
			case string, bool, float64, int, int64:
				fmt.Fprintln(cmd.OutOrStdout(), v)
			default:
				b, _ := json.Marshal(v)
				fmt.Fprintln(cmd.OutOrStdout(), string(b))
			}
			return nil
		}

		return f.Formatter().WriteItem(resp.Data, []output.Column{
			{Header: "NAME", Field: "name"},
			{Header: "SLUG", Field: "slug"},
			{Header: "IS_PUBLIC", Field: "is_public"},
			{Header: "AUTO_COMPACT", Field: "is_auto_compact_enabled"},
			{Header: "CONSENT_ANALYTICS", Field: "consent_analytics"},
			{Header: "CONSENT_MARKETING", Field: "consent_marketing"},
		})
	},
}

var settingsSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a setting value (snake_case keys; booleans accept true/false)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		var value interface{} = args[1]
		switch args[1] {
		case "true":
			value = true
		case "false":
			value = false
		case "null":
			value = nil
		}

		body := map[string]interface{}{args[0]: value}
		if err := client.Patch(cmd.Context(), "/api/v1/settings", body, nil); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Setting %s updated.\n", args[0])
		return nil
	},
}

func init() {
	settingsCmd.AddCommand(settingsGetCmd)
	settingsCmd.AddCommand(settingsSetCmd)
}
