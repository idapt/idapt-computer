package cmd

import (
	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/spf13/cobra"
)

var subscriptionCmd = &cobra.Command{
	Use:   "subscription",
	Short: "Subscription and usage info",
}

var subscriptionStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show subscription status",
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		var resp api.V1ItemResponse
		if err := client.Get(cmd.Context(), "/api/v1/subscription", nil, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteItem(resp.Data, []output.Column{
			{Header: "PLAN", Field: "plan"},
			{Header: "STATUS", Field: "status"},
			{Header: "PERIOD START", Field: "period_start"},
			{Header: "PERIOD END", Field: "period_end"},
			{Header: "CANCEL AT PERIOD END", Field: "cancel_at_period_end"},
			{Header: "CANCEL AT", Field: "cancel_at"},
			{Header: "CANCELED AT", Field: "canceled_at"},
			{Header: "TRIALING", Field: "is_trialing"},
			{Header: "TRIAL START", Field: "trial_start"},
			{Header: "TRIAL END", Field: "trial_end"},
			{Header: "PENDING DOWNGRADE TO", Field: "pending_downgrade_to"},
		})
	},
}

var subscriptionUsageCmd = &cobra.Command{
	Use:   "usage",
	Short: "Show current usage (storage + rate-limit window)",
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		var resp api.V1ItemResponse
		if err := client.Get(cmd.Context(), "/api/v1/me/usage", nil, &resp); err != nil {
			return err
		}
		storage, _ := resp.Data["storage"].(map[string]interface{})
		row := map[string]interface{}{
			"used_bytes":     anyValue(storage, "used_bytes"),
			"capacity_bytes": anyValue(storage, "capacity_bytes"),
			"snapshot_bytes": anyValue(storage, "snapshot_bytes"),
		}
		return f.Formatter().WriteItem(row, []output.Column{
			{Header: "STORAGE USED (bytes)", Field: "used_bytes"},
			{Header: "STORAGE CAPACITY (bytes)", Field: "capacity_bytes"},
			{Header: "SNAPSHOT (bytes)", Field: "snapshot_bytes"},
		})
	},
}

func anyValue(m map[string]interface{}, key string) interface{} {
	if m == nil {
		return nil
	}
	return m[key]
}

func init() {
	subscriptionCmd.AddCommand(subscriptionStatusCmd)
	subscriptionCmd.AddCommand(subscriptionUsageCmd)
}
