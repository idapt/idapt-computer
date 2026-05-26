package cmd

import (
	"fmt"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/spf13/cobra"
)

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Web search and fetch",
}

var webSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search the web",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		body := map[string]interface{}{"query": args[0]}
		if cmd.Flags().Changed("limit") {
			limit, _ := cmd.Flags().GetInt("limit")
			body["num_results"] = limit
		}

		var resp api.V1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/web/search", body, &resp); err != nil {
			return err
		}

		results, _ := resp.Data["results"].([]interface{})
		rows := make([]map[string]interface{}, 0, len(results))
		for _, r := range results {
			if m, ok := r.(map[string]interface{}); ok {
				rows = append(rows, m)
			}
		}
		return f.Formatter().WriteList(rows, []output.Column{
			{Header: "TITLE", Field: "title", Width: 50},
			{Header: "URL", Field: "url", Width: 60},
			{Header: "TEXT", Field: "text", Width: 80},
		})
	},
}

var webFetchCmd = &cobra.Command{
	Use:   "fetch <url>",
	Short: "Fetch and extract content from a URL",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		body := map[string]interface{}{"url": args[0]}
		if cmd.Flags().Changed("selector") {
			v, _ := cmd.Flags().GetString("selector")
			body["selector"] = v
		}
		if cmd.Flags().Changed("max-length") {
			v, _ := cmd.Flags().GetInt("max-length")
			body["max_length"] = v
		}

		var resp api.V1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/web/fetch", body, &resp); err != nil {
			return err
		}

		if content, ok := resp.Data["content"].(string); ok && content != "" {
			fmt.Fprintln(cmd.OutOrStdout(), content)
			return nil
		}

		return f.Formatter().WriteItem(resp.Data, []output.Column{
			{Header: "TITLE", Field: "title"},
			{Header: "URL", Field: "url"},
			{Header: "CONTENT TYPE", Field: "content_type"},
			{Header: "CONTENT", Field: "content", Width: 120},
		})
	},
}

func init() {
	webSearchCmd.Flags().Int("limit", 10, "Number of results")
	webFetchCmd.Flags().String("selector", "", "Optional CSS selector to extract a subtree")
	webFetchCmd.Flags().Int("max-length", 0, "Truncate response (chars)")

	webCmd.AddCommand(webSearchCmd)
	webCmd.AddCommand(webFetchCmd)
}
