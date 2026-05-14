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

	"github.com/spf13/cobra"
)

var (
	pairToken      string
	pairAppURL     string
	pairConfigPath string
)

var pairCmd = &cobra.Command{
	Use:   "pair",
	Short: "Pair this machine with the Idapt app",
	Long: `Exchanges a one-time IDAPT_TOKEN for a long-lived machine identity.
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
		"token":         pairToken,
		"hostname":      hostname,
		"os":            runtime.GOOS,
		"arch":          runtime.GOARCH,
		"kernelVersion": kernelVersion(),
		"cliVersion":    Version,
		"hostKind":      hostKind,
		"defaultUser":   defaultUser,
	}
	bodyBytes, _ := json.Marshal(body)

	url := strings.TrimRight(pairAppURL, "/") + "/api/machines/pair"
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
		MachineID    string `json:"machineId"`
		MachineToken string `json:"machineToken"`
		Domain       string `json:"domain"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	cfg := map[string]any{
		"machineId":          result.MachineID,
		"appUrl":             pairAppURL,
		"domain":             result.Domain,
		"jwksUrl":            strings.TrimRight(pairAppURL, "/") + "/api/managed-machines/jwks",
		"machineToken":       result.MachineToken,
		"defaultBackendPort": 80,
		"defaultUser":        defaultUser,
	}
	cfgBytes, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.MkdirAll(strings.TrimSuffix(pairConfigPath, "/config.json"), 0o755); err != nil {
	}
	if err := os.WriteFile(pairConfigPath, cfgBytes, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Paired! machineId=%s domain=%s\n", result.MachineID, result.Domain)
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
