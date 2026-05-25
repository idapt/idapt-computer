package cmd

import (
	"fmt"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/spf13/cobra"
)

var computerTmuxCmd = &cobra.Command{
	Use:   "tmux",
	Short: "Manage tmux sessions on a computer",
}

var computerTmuxListCmd = &cobra.Command{
	Use:   "list <computer-id-or-name>",
	Short: "List tmux sessions",
	Args:  cobra.ExactArgs(1),
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

		body := map[string]interface{}{"op": "list"}
		var resp api.V1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/computers/"+id+"/tmux", body, &resp); err != nil {
			return err
		}

		formatter := f.Formatter()
		return formatter.WriteList(api.AsMapSlice(resp.Data["windows"]), []output.Column{
			{Header: "NAME", Field: "name"},
			{Header: "ACTIVE", Field: "active"},
			{Header: "PANES", Field: "pane_count"},
		})
	},
}

var computerTmuxRunCmd = &cobra.Command{
	Use:   "run <computer-id-or-name> <session-name> <command>",
	Short: "Run a command in a new tmux session",
	Args:  cobra.ExactArgs(3),
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

		body := map[string]interface{}{
			"op":      "run",
			"name":    args[1],
			"command": args[2],
		}

		if err := client.Post(cmd.Context(), "/api/v1/computers/"+id+"/tmux", body, nil); err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Tmux session %s started.\n", args[1])
		return nil
	},
}

var computerTmuxCaptureCmd = &cobra.Command{
	Use:   "capture <computer-id-or-name> <session-name>",
	Short: "Capture output from a tmux session",
	Args:  cobra.ExactArgs(2),
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

		lines, _ := cmd.Flags().GetInt("lines")
		body := map[string]interface{}{
			"op":    "capture",
			"name":  args[1],
			"lines": lines,
		}

		var resp api.V1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/computers/"+id+"/tmux", body, &resp); err != nil {
			return err
		}

		if out, ok := resp.Data["content"].(string); ok {
			fmt.Fprint(cmd.OutOrStdout(), out)
			return nil
		}

		formatter := f.Formatter()
		return formatter.WriteItem(resp.Data, []output.Column{
			{Header: "CONTENT", Field: "content"},
		})
	},
}

var computerTmuxSendCmd = &cobra.Command{
	Use:   "send <computer-id-or-name> <session-name> <keys>",
	Short: "Send keys to a tmux session",
	Args:  cobra.ExactArgs(3),
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

		body := map[string]interface{}{
			"op":   "send",
			"name": args[1],
			"keys": args[2],
		}

		if err := client.Post(cmd.Context(), "/api/v1/computers/"+id+"/tmux", body, nil); err != nil {
			return err
		}

		fmt.Fprintln(cmd.OutOrStdout(), "Keys sent.")
		return nil
	},
}

var computerTmuxKillCmd = &cobra.Command{
	Use:   "kill <computer-id-or-name> <session-name>",
	Short: "Kill a tmux session",
	Args:  cobra.ExactArgs(2),
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

		body := map[string]interface{}{
			"op":   "kill",
			"name": args[1],
		}

		if err := client.Post(cmd.Context(), "/api/v1/computers/"+id+"/tmux", body, nil); err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Session %s killed.\n", args[1])
		return nil
	},
}

func init() {
	computerTmuxCaptureCmd.Flags().Int("lines", 100, "Number of lines to capture")

	computerTmuxCmd.AddCommand(computerTmuxListCmd)
	computerTmuxCmd.AddCommand(computerTmuxRunCmd)
	computerTmuxCmd.AddCommand(computerTmuxCaptureCmd)
	computerTmuxCmd.AddCommand(computerTmuxSendCmd)
	computerTmuxCmd.AddCommand(computerTmuxKillCmd)
}
