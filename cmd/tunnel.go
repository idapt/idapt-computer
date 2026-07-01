package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/idapt/idapt-computer/internal/config"
	"github.com/spf13/cobra"
)

var tunnelCmd = &cobra.Command{
	Use:   "tunnel",
	Short: "Manage idapt tunnels exposing local ports",
	Long:  "List and stop the tunnels that expose this computer's local ports publicly.",
}

var tunnelListCmd = &cobra.Command{
	Use:   "list",
	Short: "List ports currently exposed through the idapt tunnel",
	RunE:  runTunnelList,
}

var tunnelStopCmd = &cobra.Command{
	Use:   "stop <port>",
	Short: "Stop a tunnel (alias for `idapt-computer unexpose`)",
	Args:  cobra.ExactArgs(1),
	RunE:  runUnexpose,
}

func init() {
	tunnelCmd.AddCommand(tunnelListCmd)
	tunnelCmd.AddCommand(tunnelStopCmd)
	rootCmd.AddCommand(tunnelCmd)
}

func runTunnelList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load computer config: %w", err)
	}
	respBody, err := daemonTunnelRequest(cfg, http.MethodGet, "/api/tunnels", nil)
	if err != nil {
		return err
	}
	var resp struct {
		Tunnels []tunnelInfo `json:"tunnels"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return fmt.Errorf("parse daemon response: %w", err)
	}
	if len(resp.Tunnels) == 0 {
		fmt.Println("No tunnels are currently exposed.")
		return nil
	}
	fmt.Printf("%-8s %-10s %s\n", "PORT", "AUTH", "URL")
	fmt.Println(strings.Repeat("-", 60))
	for _, t := range resp.Tunnels {
		fmt.Printf("%-8d %-10s %s\n", t.Port, t.AuthMode, t.URL)
	}
	return nil
}
