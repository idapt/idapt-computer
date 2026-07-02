package cmd

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/idapt/idapt-computer/internal/config"
	"github.com/spf13/cobra"
)

type tunnelInfo struct {
	Port     int    `json:"port"`
	AuthMode string `json:"authMode"`
	Hostname string `json:"hostname"`
	URL      string `json:"url"`
}

var exposeAuthMode string

var exposeCmd = &cobra.Command{
	Use:   "expose <port>",
	Short: "Expose a local port publicly through the idapt tunnel",
	Long: "Expose a local port at https://<port>--<id>.idapt.computer, reachable\n" +
		"from anywhere behind idapt authentication. The local service must be\n" +
		"listening on 127.0.0.1:<port>. The tunnel persists until `idapt-computer unexpose`.",
	Args: cobra.ExactArgs(1),
	RunE: runExpose,
}

var unexposeCmd = &cobra.Command{
	Use:   "unexpose <port>",
	Short: "Stop exposing a local port through the idapt tunnel",
	Args:  cobra.ExactArgs(1),
	RunE:  runUnexpose,
}

func init() {
	exposeCmd.Flags().StringVar(&exposeAuthMode, "auth", "private",
		"who may reach the tunnel: private (workspace owner), workspace (members), idapt (any signed-in user)")
	rootCmd.AddCommand(exposeCmd)
	rootCmd.AddCommand(unexposeCmd)
}

func runExpose(cmd *cobra.Command, args []string) error {
	port, err := parseTunnelPort(args[0])
	if err != nil {
		return err
	}
	switch exposeAuthMode {
	case "private", "workspace", "idapt":
	default:
		return fmt.Errorf("invalid --auth %q: must be private, workspace, or idapt", exposeAuthMode)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load computer config: %w", err)
	}
	body, _ := json.Marshal(map[string]any{"port": port, "authMode": exposeAuthMode})
	respBody, err := daemonTunnelRequest(cfg, http.MethodPost, "/api/tunnels", body)
	if err != nil {
		return err
	}
	var info tunnelInfo
	if err := json.Unmarshal(respBody, &info); err != nil {
		return fmt.Errorf("parse daemon response: %w", err)
	}
	fmt.Printf("Exposed 127.0.0.1:%d  (auth: %s)\n", info.Port, info.AuthMode)
	fmt.Printf("  %s\n", info.URL)
	return nil
}

func runUnexpose(cmd *cobra.Command, args []string) error {
	port, err := parseTunnelPort(args[0])
	if err != nil {
		return err
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load computer config: %w", err)
	}
	if _, err := daemonTunnelRequest(cfg, http.MethodDelete,
		fmt.Sprintf("/api/tunnels?port=%d", port), nil); err != nil {
		return err
	}
	fmt.Printf("Unexposed port %d\n", port)
	return nil
}

func parseTunnelPort(s string) (int, error) {
	port, err := strconv.Atoi(s)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid port %q: must be 1-65535", s)
	}
	return port, nil
}

func daemonBaseURL() string {
	if p := os.Getenv("IDAPT_HTTPS_PORT"); p != "" && p != "443" {
		return "http://localhost:" + p
	}
	return "http://localhost"
}

func computeHMAC(message, secret string) string {
	keyBytes, err := hex.DecodeString(secret)
	if err != nil {
		keyBytes = []byte(secret)
	}
	mac := hmac.New(sha256.New, keyBytes)
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

func daemonTunnelRequest(cfg *config.Config, method, path string, body []byte) ([]byte, error) {
	sigPath := path
	if i := strings.IndexByte(sigPath, '?'); i >= 0 {
		sigPath = sigPath[:i]
	}
	ts := fmt.Sprintf("%d", time.Now().Unix())
	sig := computeHMAC(method+":"+sigPath+":"+ts, cfg.ComputerToken)

	var reader io.Reader
	if body != nil {
		reader = strings.NewReader(string(body))
	}
	req, err := http.NewRequest(method, daemonBaseURL()+path, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Computer-Signature", sig)
	req.Header.Set("X-Computer-Timestamp", ts)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to contact the local idapt daemon (is `idapt-computer serve` running?): %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return respBody, nil
}
