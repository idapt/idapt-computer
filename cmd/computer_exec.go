package cmd

import (
	"fmt"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/spf13/cobra"
)

var computerExecCmd = &cobra.Command{
	Use:   "exec <id-or-name> <command>",
	Short: "Execute a command on a computer",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		id, err := resolveComputer(cmd, f, args[0])
		if err != nil {
			return err
		}

		command := args[1]
		if len(args) > 2 {
			command = ""
			for i := 1; i < len(args); i++ {
				if i > 1 {
					command += " "
				}
				command += args[i]
			}
		}

		user, _ := cmd.Flags().GetString("user")
		timeout, _ := cmd.Flags().GetInt("timeout")

		body := map[string]interface{}{
			"command": command,
		}
		if user != "" {
			body["user"] = user
		}
		if timeout > 0 {
			body["timeout_seconds"] = timeout
		}

		var resp api.V1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/computers/"+id+"/exec", body, &resp); err != nil {
			return err
		}

		if out, ok := resp.Data["stdout"].(string); ok {
			fmt.Fprint(cmd.OutOrStdout(), out)
			return nil
		}

		formatter := f.Formatter()
		return formatter.WriteItem(resp.Data, []output.Column{
			{Header: "EXIT CODE", Field: "exit_code"},
			{Header: "STDOUT", Field: "stdout"},
			{Header: "STDERR", Field: "stderr"},
			{Header: "DURATION_MS", Field: "duration_ms"},
			{Header: "TIMED_OUT", Field: "timed_out"},
		})
	},
}

func init() {
	computerExecCmd.Flags().String("user", "", "Run as user")
	computerExecCmd.Flags().Int("timeout", 0, "Timeout in seconds")
}
