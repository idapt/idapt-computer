package cmd

import (
	"fmt"
	"net/url"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/spf13/cobra"
)

var computerFwCmd = &cobra.Command{
	Use:     "firewall",
	Aliases: []string{"fw"},
	Short:   "Manage computer firewall rules (remote, via REST API)",
}

var computerFwListCmd = &cobra.Command{
	Use:   "list <computer-id-or-name>",
	Short: "List firewall rules",
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
		if err := client.Get(cmd.Context(), "/api/v1/computers/"+id+"/firewall", nil, &resp); err != nil {
			return err
		}

		formatter := f.Formatter()
		return formatter.WriteList(api.AsMapSlice(resp.Data["rules"]), []output.Column{
			{Header: "PORT", Field: "port"},
			{Header: "PROTOCOL", Field: "protocol"},
		})
	},
}

var computerFwAddCmd = &cobra.Command{
	Use:   "add <computer-id-or-name>",
	Short: "Add a firewall rule",
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

		port, _ := cmd.Flags().GetInt("port")
		protocol, _ := cmd.Flags().GetString("protocol")
		if port == 0 {
			return fmt.Errorf("--port is required")
		}

		body := map[string]interface{}{
			"port":     port,
			"protocol": protocol,
		}

		var resp api.V1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/computers/"+id+"/firewall", body, &resp); err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Firewall rule added for port %d/%s.\n", port, protocol)
		return nil
	},
}

var computerFwRemoveCmd = &cobra.Command{
	Use:   "remove <computer-id-or-name> <port>",
	Short: "Remove a firewall rule by port/protocol",
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

		protocol, _ := cmd.Flags().GetString("protocol")
		q := url.Values{"port": {args[1]}, "protocol": {protocol}}
		resp, err := client.Do(
			cmd.Context(),
			"DELETE",
			"/api/v1/computers/"+id+"/firewall",
			nil,
			api.WithQuery(q),
		)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		fmt.Fprintln(cmd.OutOrStdout(), "Firewall rule removed.")
		return nil
	},
}

func init() {
	computerFwAddCmd.Flags().Int("port", 0, "Port number")
	computerFwAddCmd.Flags().String("protocol", "tcp", "Protocol (tcp, udp)")
	computerFwRemoveCmd.Flags().String("protocol", "tcp", "Protocol (tcp, udp)")

	computerFwCmd.AddCommand(computerFwListCmd)
	computerFwCmd.AddCommand(computerFwAddCmd)
	computerFwCmd.AddCommand(computerFwRemoveCmd)
}
