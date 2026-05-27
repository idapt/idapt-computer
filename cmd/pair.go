package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/idapt/idapt-cli/internal/config"
	"github.com/spf13/cobra"
)

var (
	pairToken      string
	pairAppURL     string
	pairConfigPath string
)

var pairCmd = &cobra.Command{
	Use:   "pair",
	Short: "Pair this computer with Idapt (legacy — prefer `idapt up`)",
	Long: `Legacy one-shot pair-token flow. Kept for backwards compatibility
with the install one-liner and CI / mass-provision tooling.

For interactive use, prefer ` + "`idapt up`" + `, which presents a Tailscale-style
device flow (no clipboard-borne secret) and is idempotent across re-runs.
This command is equivalent to ` + "`idapt up --token <token> --config <path>`" + `.

Exchanges a one-time IDAPT_TOKEN for a long-lived computer identity.
Writes /etc/idapt/config.json (or --config path) so the daemon can serve.`,
	RunE: runPair,
}

func init() {
	pairCmd.Flags().StringVar(&pairToken, "token", "", "Pair token (required)")
	pairCmd.Flags().StringVar(&pairAppURL, "app-url", "https://idapt.ai", "Idapt app URL")
	pairCmd.Flags().StringVar(&pairConfigPath, "config", "/etc/idapt/config.json", "Path to write config")
	rootCmd.AddCommand(pairCmd)
}

func runPair(cmd *cobra.Command, args []string) error {
	if pairToken == "" {
		return fmt.Errorf("--token is required (or set IDAPT_TOKEN)")
	}
	fmt.Fprintln(cmd.ErrOrStderr(), "Note: `idapt pair` is the legacy bootstrap flow. New installs should use `idapt up` (Tailscale-style device flow).")

	hostname, _ := os.Hostname()
	defaultUser := os.Getenv("USER")
	if defaultUser == "" {
		defaultUser = "ubuntu"
	}
	hostKind := "server"
	if isLikelyDesktop() {
		hostKind = "desktop"
	}

	body := map[string]any{
		"token":          pairToken,
		"hostname":       hostname,
		"os":             runtime.GOOS,
		"arch":           runtime.GOARCH,
		"kernel_version": kernelVersion(),
		"cli_version":    Version,
		"host_kind":      hostKind,
		"default_user":   defaultUser,
	}
	if existing, _ := config.Load(pairConfigPath); existing != nil && !existing.IsLocalMode() {
		resourceID := existing.ComputerResourceID
		if resourceID == "" {
			resourceID = existing.ComputerID
		}
		if resourceID != "" {
			body["existing_computer_resource_id"] = resourceID
		}
	}
	bodyBytes, _ := json.Marshal(body)

	url := strings.TrimRight(pairAppURL, "/") + "/api/v1/computers/pair"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("pair request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		var bodyBuf bytes.Buffer
		_, _ = bodyBuf.ReadFrom(resp.Body)
		return fmt.Errorf("pair returned %d: %s", resp.StatusCode, bodyBuf.String())
	}

	var result struct {
		Data struct {
			ComputerID    string `json:"computer_id"`
			ComputerToken string `json:"computer_token"`
			Domain       string `json:"domain"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	cfg := map[string]any{
		"computerId":         result.Data.ComputerID,
		"computerResourceId": result.Data.ComputerID,
		"appUrl":             pairAppURL,
		"domain":             result.Data.Domain,
		"jwksUrl":            strings.TrimRight(pairAppURL, "/") + "/api/cloud-computers/jwks",
		"computerToken":      result.Data.ComputerToken,
		"defaultBackendPort": 80,
		"defaultUser":        defaultUser,
	}
	cfgBytes, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.MkdirAll(strings.TrimSuffix(pairConfigPath, "/config.json"), 0o755); err != nil {
	}
	if err := os.WriteFile(pairConfigPath, cfgBytes, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Paired! computerId=%s domain=%s\n", result.Data.ComputerID, result.Data.Domain)
	return nil
}

func kernelVersion() string {
	if runtime.GOOS == "linux" {
		out, err := exec.Command("uname", "-r").Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
	}
	return ""
}

func isLikelyDesktop() bool {
	if os.Getenv("XDG_SESSION_TYPE") != "" {
		return true
	}
	if os.Getenv("DISPLAY") != "" {
		return true
	}
	return false
}
