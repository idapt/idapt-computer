package cmd

import (
	"fmt"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:   "exec",
	Short: "Run code files in the cloud sandbox",
}

var execFileCmd = &cobra.Command{
	Use:   "file <file-id>",
	Short: "Run a code file in the cloud sandbox",
	Long:  "Runs a stored .js / .ts / .py / .sh / etc file by its resourceId in a sandboxed cloud environment.",
	Example: `  # Run a stored script by id (or name)
  idapt exec file my-script.py

  # Bound the run with a timeout (seconds)
  idapt exec file my-script.py --timeout 60`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		body := map[string]interface{}{"file_id": args[0]}
		if cmd.Flags().Changed("timeout") {
			v, _ := cmd.Flags().GetInt("timeout")
			body["timeout_seconds"] = v
		}

		var resp api.V1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/code-runs", body, &resp); err != nil {
			return err
		}

		if out, ok := resp.Data["stdout"].(string); ok && out != "" {
			fmt.Fprint(cmd.OutOrStdout(), out)
		}
		if stderr, ok := resp.Data["stderr"].(string); ok && stderr != "" {
			fmt.Fprint(cmd.ErrOrStderr(), stderr)
		}

		stdout, _ := resp.Data["stdout"].(string)
		stderr, _ := resp.Data["stderr"].(string)
		if stdout == "" && stderr == "" {
			return f.Formatter().WriteItem(resp.Data, []output.Column{
				{Header: "ID", Field: "id"},
				{Header: "STATUS", Field: "status"},
				{Header: "EXIT CODE", Field: "exit_code"},
				{Header: "CREATED", Field: "created_at"},
			})
		}
		return nil
	},
}

func init() {
	execFileCmd.Flags().Int("timeout", 0, "Timeout in seconds (1–300)")

	execCmd.AddCommand(execFileCmd)
}
