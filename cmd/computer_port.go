package cmd

import (
	"fmt"
	"strconv"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/spf13/cobra"
)

var computerPortCmd = &cobra.Command{
	Use:   "port",
	Short: "Manage computer port labels",
}

var computerPortListCmd = &cobra.Command{
	Use:   "list <computer-id-or-name>",
	Short: "List open ports on a computer",
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

		var resp api.V1ItemResponse
		if err := client.Get(cmd.Context(), "/api/v1/computers/"+id+"/ports", nil, &resp); err != nil {
			return err
		}

		formatter := f.Formatter()
		return formatter.WriteList(api.AsMapSlice(resp.Data["ports"]), []output.Column{
			{Header: "PORT", Field: "port"},
			{Header: "PROTOCOL", Field: "protocol"},
			{Header: "LABEL", Field: "display_name"},
			{Header: "PROCESS", Field: "process_name"},
			{Header: "HIDDEN", Field: "hidden"},
		})
	},
}

var computerPortLabelCmd = &cobra.Command{
	Use:   "label <computer-id-or-name> <port> <label>",
	Short: "Set a label for a port",
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

		port, err := strconv.Atoi(args[1])
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("port must be an integer between 1 and 65535")
		}
		protocol, _ := cmd.Flags().GetString("protocol")

		body := map[string]interface{}{
			"port":         port,
			"protocol":     protocol,
			"display_name": args[2],
		}
		if err := client.Patch(cmd.Context(), "/api/v1/computers/"+id+"/ports", body, nil); err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Port %s labeled as %s.\n", args[1], args[2])
		return nil
	},
}

func init() {
	computerPortLabelCmd.Flags().String("protocol", "tcp", "Protocol (tcp, udp)")

	computerPortCmd.AddCommand(computerPortListCmd)
	computerPortCmd.AddCommand(computerPortLabelCmd)
}
