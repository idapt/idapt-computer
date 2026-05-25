package cmd

import (
	"net/url"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/spf13/cobra"
)

var modelCmd = &cobra.Command{
	Use:   "model",
	Short: "Browse models",
}

var modelListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available models",
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		q := url.Values{}
		if cmd.Flags().Changed("provider") {
			v, _ := cmd.Flags().GetString("provider")
			q.Set("provider", v)
		}

		var resp api.V1ListResponse
		if err := client.Get(cmd.Context(), "/api/v1/models", q, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteList(resp.Data, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "NAME", Field: "display_name"},
			{Header: "PROVIDER", Field: "provider"},
			{Header: "MODALITY", Field: "modality"},
			{Header: "CONTEXT", Field: "capabilities.context_length"},
			{Header: "IMAGE INPUT", Field: "capabilities.image_input"},
		})
	},
}

var modelSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search models by name",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		q := url.Values{"q": {args[0]}}
		var resp api.V1ListResponse
		if err := client.Get(cmd.Context(), "/api/v1/models", q, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteList(resp.Data, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "NAME", Field: "display_name"},
			{Header: "PROVIDER", Field: "provider"},
			{Header: "MODALITY", Field: "modality"},
		})
	},
}

func init() {
	modelListCmd.Flags().String("provider", "", "Filter by provider (openai, anthropic, google, etc.)")

	modelCmd.AddCommand(modelListCmd)
	modelCmd.AddCommand(modelSearchCmd)
}
