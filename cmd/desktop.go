package cmd

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/idapt/idapt-computer/internal/config"
	"github.com/spf13/cobra"
)

var desktopCmd = &cobra.Command{
	Use:   "desktop",
	Short: "Lifecycle hooks invoked by the Idapt desktop app",
	Long: `Commands the Idapt desktop shell calls into the bundled CLI to keep
the local autostart unit + computer identity in sync with the user's
sign-in state.

These verbs are meant for the desktop app's main process to spawn as
child_process; they accept --json for machine-readable output. End
users typically reach the same behavior through ` + "`idapt-computer up`" + ` /
` + "`idapt-computer logout`" + ` directly.`,
}

var desktopIdentityCmd = &cobra.Command{
	Use:   "identity",
	Short: "Print the locally-persisted computer identity (or empty)",
	Long: `Resolves the per-user daemon config and prints the computer
resourceId stored on disk, or an empty result if no pairing exists.

Output is JSON by default for machine consumption from Electron:
  { "computerResourceId": "...", "domain": "...", "appUrl": "..." }
or
  { "computerResourceId": null }

Pass --plain to print only the resourceId (or empty string) on stdout.`,
	RunE: runDesktopIdentity,
}

var desktopRegisterCmd = &cobra.Command{
	Use:   "register",
	Short: "Redeem a pair token and install the autostart unit",
	Long: `Desktop sign-in hook. Redeems the supplied pair token and installs
the per-OS autostart unit. Symmetric to ` + "`idapt-computer desktop unregister`" + `.

If the daemon already has a ` + "`config.json`" + ` on disk (it survived
a prior sign-out / app uninstall), the pair body includes its
` + "`existing_computer_resource_id`" + ` so the server re-binds to the
same row, unarchiving it if needed.`,
	RunE: runDesktopRegister,
}

var desktopUnregisterCmd = &cobra.Command{
	Use:   "unregister",
	Short: "Archive the computer + stop the daemon + remove the autostart unit (keep identity)",
	Long: `Desktop sign-out hook. Best-effort archives the computer on the
server (HMAC-signed call to /api/cloud-computers/<id>/archive) so it
disappears from the user's list while the daemon is down, then stops
the running daemon and removes the per-OS autostart unit. Keeps the
per-user daemon config so a subsequent ` + "`idapt-computer desktop register`" + `
re-binds to the same row (the server will unarchive it).

The archive step is best-effort: if the server is unreachable or the
HMAC is rejected, the local teardown still runs.`,
	RunE: runDesktopUnregister,
}

var desktopArchiveCmd = &cobra.Command{
	Use:   "archive",
	Short: "Best-effort archive the local computer via HMAC (no service changes)",
	Long: `Calls the server's HMAC-authenticated archive endpoint using the
daemon's stored computerToken. Doesn't touch the autostart unit or
local config — purely an API call.

Invoked from OS-level package uninstall hooks (deb postrm, rpm
%postun, NSIS uninstall section) so the computer disappears from the
user's list when the desktop app is removed without a graceful sign-
out first. A subsequent reinstall + sign-in unarchives via the
pair-redeem reattach path.`,
	RunE: runDesktopArchive,
}

var desktopPurgeCmd = &cobra.Command{
	Use:   "purge",
	Short: "Full desktop teardown: archive computer, stop daemon, remove unit, wipe local identity",
	Long: `Run on full desktop-app uninstall. Performs ` + "`unregister`" + `
(which archives + tears down service) THEN removes the per-user
daemon config so the next install starts clean. Doesn't touch the
CLI binary itself — the desktop-app package manager owns that.`,
	RunE: runDesktopPurge,
}

func init() {
	desktopIdentityCmd.Flags().Bool("plain", false, "Print just the resourceId on stdout (or empty string)")
	desktopRegisterCmd.Flags().String("token", "", "Pair token (required)")
	desktopRegisterCmd.Flags().String("app-url", "", "Idapt app URL (defaults to IDAPT_APP_URL or https://idapt.app)")
	desktopRegisterCmd.Flags().Bool("no-service", false, "Skip installing/starting the autostart unit (pair only)")
	desktopArchiveCmd.Flags().Bool("best-effort", false, "Always exit 0; print warnings on stderr but never fail (for package-uninstall hooks)")

	desktopCmd.AddCommand(desktopIdentityCmd)
	desktopCmd.AddCommand(desktopRegisterCmd)
	desktopCmd.AddCommand(desktopUnregisterCmd)
	desktopCmd.AddCommand(desktopArchiveCmd)
	desktopCmd.AddCommand(desktopPurgeCmd)
	rootCmd.AddCommand(desktopCmd)
}

type desktopIdentityView struct {
	ComputerResourceID string `json:"computerResourceId,omitempty"`
	Domain             string `json:"domain,omitempty"`
	AppURL             string `json:"appUrl,omitempty"`
	HasIdentity        bool   `json:"hasIdentity"`
}

func runDesktopIdentity(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	plain, _ := cmd.Flags().GetBool("plain")

	cfgPath, err := config.UserConfigPath()
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}
	cfg, _ := config.Load(cfgPath)
	view := desktopIdentityView{}
	if cfg != nil && !cfg.IsLocalMode() {
		resourceID := cfg.ComputerResourceID
		if resourceID == "" {
			resourceID = cfg.ComputerID
		}
		view.ComputerResourceID = resourceID
		view.Domain = cfg.Domain
		view.AppURL = cfg.AppURL
		view.HasIdentity = resourceID != ""
	}

	if plain {
		fmt.Fprintln(out, view.ComputerResourceID)
		return nil
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(view)
}

func runDesktopRegister(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	token, _ := cmd.Flags().GetString("token")
	appURL, _ := cmd.Flags().GetString("app-url")
	noService, _ := cmd.Flags().GetBool("no-service")

	if strings.TrimSpace(token) == "" {
		return errors.New("--token is required")
	}

	upToken = strings.TrimSpace(token)
	upCode = ""
	upAppURL = strings.TrimSpace(appURL)
	upNoService = noService
	upForce = true
	upConfigPath = ""

	if err := runUp(cmd, nil); err != nil {
		return err
	}
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Desktop registration complete.")
	return nil
}

func runDesktopUnregister(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	if err := archiveLocalComputer(); err != nil {
		fmt.Fprintf(out, "WARN: archive: %v\n", err)
	}
	if err := serviceDown(cmd); err != nil {
		fmt.Fprintf(out, "WARN: service down: %v\n", err)
	}
	if err := serviceUninstall(cmd); err != nil {
		fmt.Fprintf(out, "WARN: service uninstall: %v\n", err)
	}
	fmt.Fprintln(out, "Desktop unregistered (identity preserved).")
	return nil
}

func runDesktopArchive(cmd *cobra.Command, _ []string) error {
	bestEffort, _ := cmd.Flags().GetBool("best-effort")
	err := archiveLocalComputer()
	if err != nil {
		if bestEffort {
			fmt.Fprintf(cmd.ErrOrStderr(), "WARN: archive failed: %v\n", err)
			return nil
		}
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Computer archived.")
	return nil
}

func runDesktopPurge(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	if err := runDesktopUnregister(cmd, nil); err != nil {
		fmt.Fprintf(out, "WARN: unregister errored: %v\n", err)
	}
	cfgPath, err := config.UserConfigPath()
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}
	if _, statErr := os.Stat(cfgPath); statErr == nil {
		if err := os.Remove(cfgPath); err != nil {
			return fmt.Errorf("remove %s: %w", cfgPath, err)
		}
		fmt.Fprintf(out, "Removed daemon config: %s\n", cfgPath)
	}
	fmt.Fprintln(out, "Desktop purge complete.")
	return nil
}

func archiveLocalComputer() error {
	cfgPath, err := config.UserConfigPath()
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg == nil || cfg.IsLocalMode() {
		return nil
	}
	resourceID := cfg.ComputerResourceID
	if resourceID == "" {
		resourceID = cfg.ComputerID
	}
	if resourceID == "" {
		return errors.New("config has no computer id")
	}
	if cfg.ComputerToken == "" {
		return errors.New("config has no computer token")
	}
	appURL := strings.TrimRight(cfg.AppURL, "/")
	if appURL == "" {
		appURL = "https://idapt.app"
	}

	path := fmt.Sprintf("/api/cloud-computers/%s/archive", resourceID)
	url := appURL + path
	body := []byte("{}")
	bodyHash := sha256.Sum256(body)
	bodyHashHex := hex.EncodeToString(bodyHash[:])
	timestamp := fmt.Sprintf("%d", time.Now().Unix())

	keyBytes, decodeErr := hex.DecodeString(cfg.ComputerToken)
	if decodeErr != nil {
		keyBytes = []byte(cfg.ComputerToken)
	}
	message := fmt.Sprintf("POST:%s:%s:%s", path, timestamp, bodyHashHex)
	mac := hmac.New(sha256.New, keyBytes)
	mac.Write([]byte(message))
	signature := hex.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Computer-Signature", signature)
	req.Header.Set("X-Computer-Timestamp", timestamp)
	req.Header.Set("X-Computer-Body-Sha256", bodyHashHex)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("archive request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("archive returned %d", resp.StatusCode)
	}
	return nil
}
