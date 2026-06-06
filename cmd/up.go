package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/config"
	"github.com/idapt/idapt-cli/internal/deviceflow"
	"github.com/idapt/idapt-cli/internal/progress"
	"github.com/spf13/cobra"
)

var (
	upToken      string
	upCode       string
	upAppURL     string
	upConfigPath string
	upNoService  bool
	upForce      bool
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Bring this computer online with idapt (install + authorize + start)",
	Long: `Idempotent end-to-end "make this computer work with idapt" verb.

Three things in one verb:

  1. Authorize this computer with your idapt account. By default opens a
     Tailscale-style device-flow: the CLI prints a URL, you open it in an
     already-signed-in browser, click Confirm, and the CLI picks up the
     credentials. No secret in the clipboard, no curl|bash with a token
     in argv.

  2. Write the daemon config to the per-user config dir
     (~/.config/idapt/ on Linux, ~/Library/Application Support/idapt/ on
     macOS, %AppData%\idapt\ on Windows). No sudo needed.

  3. Install the autostart unit and start the daemon (skip with
     --no-service if you only want to authorize).

Examples:

  idapt up
      Device flow — opens a URL the user confirms in a browser.

  idapt up --code ABCD-2345
      Use a pre-minted code from the web UI ("Add computer" dialog).

  idapt up --token pt_xxx
      Legacy pair-token flow (CI / mass-provision / install-script).

  idapt up --no-service
      Authorize but don't install the autostart unit. Useful in
      containers where systemd / launchd isn't available.

  idapt up --force
      Allow overwriting an existing pairing (e.g. linking the same
      machine to a different account, which is otherwise refused).`,
	RunE: runUp,
}

func init() {
	upCmd.Flags().StringVar(&upToken, "token", "", "Pair token (legacy / CI flow)")
	upCmd.Flags().StringVar(&upCode, "code", "", "Device code minted elsewhere (paste from the web UI)")
	upCmd.Flags().StringVar(&upAppURL, "app-url", "", "Idapt app URL (defaults to IDAPT_APP_URL or https://idapt.ai)")
	upCmd.Flags().StringVar(&upConfigPath, "config", "", "Path to write daemon config (defaults to per-user XDG path)")
	upCmd.Flags().BoolVar(&upNoService, "no-service", false, "Skip installing/starting the autostart unit")
	upCmd.Flags().BoolVar(&upForce, "force", false, "Overwrite an existing pairing without confirmation")
	rootCmd.AddCommand(upCmd)

	loginCmd := &cobra.Command{
		Use:    "login",
		Short:  "Alias for `idapt up` (Tailscale-style device flow)",
		Hidden: false,
		RunE:   runUp,
	}
	loginCmd.Flags().AddFlagSet(upCmd.Flags())
	rootCmd.AddCommand(loginCmd)
}

func trustedAppHosts() map[string]struct{} {
	return map[string]struct{}{
		"idapt.ai":     {},
		"www.idapt.ai": {},
		"idapt.app":    {},
	}
}

func resolveAppURL() string {
	if upAppURL != "" {
		return strings.TrimRight(upAppURL, "/")
	}
	if v := os.Getenv("IDAPT_APP_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "https://idapt.ai"
}

func appURLFromFlag() bool {
	return upAppURL != ""
}

func validateAppURLForPairing(appURL string) error {
	if !appURLFromFlag() {
		return nil // env / hard-coded default — operator trust root.
	}
	if os.Getenv("IDAPT_ALLOW_INSECURE_APP_URL") == "1" {
		return nil // explicit operator break-glass for self-hosting.
	}
	u, err := url.Parse(appURL)
	if err != nil {
		return fmt.Errorf("invalid --app-url %q: %w", appURL, err)
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return nil
	}
	if u.Scheme != "https" {
		return fmt.Errorf(
			"refusing to pair against non-https app URL %q — the pair token and computer credential must never travel over http (set IDAPT_ALLOW_INSECURE_APP_URL=1 for a trusted self-hosted origin)",
			appURL,
		)
	}
	if _, ok := trustedAppHosts()[host]; !ok {
		return fmt.Errorf(
			"refusing to pair against untrusted app host %q — only Idapt origins are accepted via --app-url (set IDAPT_APP_URL or IDAPT_ALLOW_INSECURE_APP_URL=1 for a self-hosted origin)",
			host,
		)
	}
	return nil
}

func resolveUpConfigPath() (string, error) {
	if upConfigPath != "" {
		return upConfigPath, nil
	}
	return config.EnsureUserConfigPath()
}

func runUp(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	appURL := resolveAppURL()
	configPath, err := resolveUpConfigPath()
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}

	existing, _ := config.Load(configPath)
	if existing != nil && !existing.IsLocalMode() {
		probe := deviceflow.ProbePairing(
			cmd.Context(),
			existing.AppURL,
			existing.ComputerID,
			existing.ComputerToken,
		)
		switch probe {
		case deviceflow.PairingRevoked:
			fmt.Fprintln(out, "Existing pairing was revoked server-side. Re-pairing...")
		case deviceflow.PairingValid, deviceflow.PairingUnknown:
			if !upForce {
				fmt.Fprintf(
					out,
					"This computer is already linked to %s (%s).\n",
					existing.Domain,
					existing.AppURL,
				)
				fmt.Fprintln(out, "")
				fmt.Fprintln(out, "Run `idapt logout` to clear the pairing first,")
				fmt.Fprintln(out, "or pass --force to overwrite the existing pairing.")
				fmt.Fprintln(out, "")
				fmt.Fprintf(out, "Computer: %s\n", existing.Domain)
				fmt.Fprintf(out, "App URL:  %s\n", existing.AppURL)
				if !upNoService {
					fmt.Fprintln(out, "")
					fmt.Fprintln(out, "Daemon: run `idapt service status` to check the daemon state,")
					fmt.Fprintln(out, "or `idapt service up` to (re)install the autostart unit.")
				}
				return nil
			}
			fmt.Fprintln(out, "Overwriting existing pairing (--force).")
		}
	}

	if err := validateAppURLForPairing(appURL); err != nil {
		return err
	}

	hostname, _ := os.Hostname()
	defaultUser := os.Getenv("USER")
	if defaultUser == "" {
		defaultUser = "ubuntu"
	}

	switch {
	case upToken != "":
		if err := runTokenFlow(cmd, appURL, upToken, configPath, hostname, defaultUser); err != nil {
			return err
		}
	default:
		if err := runDeviceFlow(cmd, appURL, upCode, configPath, hostname, defaultUser); err != nil {
			return err
		}
	}

	if upNoService {
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "Skipped service install (--no-service).")
		fmt.Fprintln(out, "Start the daemon later with: idapt service up")
		return nil
	}

	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Installing daemon autostart unit...")
	if err := serviceUp(cmd, false); err != nil {
		fmt.Fprintf(out, "WARN: service install failed: %v\n", err)
		fmt.Fprintln(out, "Pairing succeeded. Run `idapt service up` manually once the issue is fixed.")
		return nil
	}
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "All set. Your computer is online.")
	return nil
}

func runDeviceFlow(cmd *cobra.Command, appURL, presetCode, configPath, hostname, defaultUser string) error {
	out := cmd.OutOrStdout()
	ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Minute)
	defer cancel()
	client := deviceflow.NewClient(appURL, &http.Client{Timeout: 30 * time.Second})

	var (
		code       string
		confirmURL string
	)
	if presetCode != "" {
		code = strings.TrimSpace(strings.ToUpper(presetCode))
		confirmURL = appURL + "/auth/device?code=" + code
		fmt.Fprintf(out, "Polling for approval of code %s...\n", code)
	} else {
		req := deviceflow.MintRequest{
			Hostname:    hostname,
			OS:          deviceflow.GuessOS(),
			Arch:        deviceflow.GuessArch(),
			CLIVersion:  Version,
			DefaultUser: defaultUser,
			HostKind:    deviceflow.GuessHostKind(),
			KernelVersion: kernelVersionForUp(),
		}
		mintCtx, mintCancel := context.WithTimeout(ctx, 30*time.Second)
		minted, err := client.Mint(mintCtx, req)
		mintCancel()
		if err != nil {
			return fmt.Errorf("mint device code: %w", err)
		}
		code = minted.Code
		confirmURL = minted.ConfirmURL
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "To authorize this computer, open this URL in your browser:")
		fmt.Fprintln(out, "")
		fmt.Fprintf(out, "  %s\n", confirmURL)
		fmt.Fprintln(out, "")
		fmt.Fprintf(out, "Or paste this code at %s/auth/device:\n", appURL)
		fmt.Fprintf(out, "  %s\n", code)
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "Waiting for approval...")
	}

	var sp *progress.Spinner
	if f := cmdutil.FactoryFromCmd(cmd); f != nil {
		sp = f.NewSpinner("Waiting for approval")
	}
	sp.Start()
	view, result, err := client.Poll(ctx, code, 2*time.Second)
	sp.Stop()
	switch result {
	case deviceflow.PollApproved:
	case deviceflow.PollDenied:
		return errors.New("authorization denied by the user")
	case deviceflow.PollExpired:
		return errors.New("device code expired before approval — run `idapt up` again to get a fresh code")
	case deviceflow.PollNotFound:
		return errors.New("device code not found on the server (was it already used?)")
	case deviceflow.PollCanceled:
		if err != nil {
			return err
		}
		return errors.New("device-flow polling canceled")
	}
	if view == nil || view.ComputerID == "" || view.ComputerToken == "" {
		return errors.New("device flow approved but no credentials were returned (race) — run `idapt up` again")
	}

	if err := writeConfigFromCredentials(configPath, appURL, view.ComputerID, view.ComputerToken, view.Domain, defaultUser); err != nil {
		return err
	}
	fmt.Fprintln(out, "")
	fmt.Fprintf(out, "Linked: %s\n", view.Domain)
	return nil
}

func runTokenFlow(cmd *cobra.Command, appURL, token, configPath, hostname, defaultUser string) error {
	out := cmd.OutOrStdout()
	hostKind := "server"
	if deviceflow.GuessHostKind() == "desktop" {
		hostKind = "desktop"
	}
	body := map[string]any{
		"token":          token,
		"hostname":       hostname,
		"os":             runtime.GOOS,
		"arch":           runtime.GOARCH,
		"kernel_version": kernelVersionForUp(),
		"cli_version":    Version,
		"host_kind":      hostKind,
		"default_user":   defaultUser,
	}
	if existing, _ := config.Load(configPath); existing != nil && !existing.IsLocalMode() {
		resourceID := existing.ComputerResourceID
		if resourceID == "" {
			resourceID = existing.ComputerID
		}
		if resourceID != "" {
			body["existing_computer_resource_id"] = resourceID
		}
	}
	bodyBytes, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(
		cmd.Context(),
		http.MethodPost,
		appURL+"/api/v1/computers/pair",
		strings.NewReader(string(bodyBytes)),
	)
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
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		buf, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pair returned %d: %s", resp.StatusCode, strings.TrimSpace(string(buf)))
	}
	var result struct {
		Data struct {
			ComputerID    string `json:"computer_id"`
			ComputerToken string `json:"computer_token"`
			Domain        string `json:"domain"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if err := writeConfigFromCredentials(configPath, appURL, result.Data.ComputerID, result.Data.ComputerToken, result.Data.Domain, defaultUser); err != nil {
		return err
	}
	fmt.Fprintf(out, "Linked: %s\n", result.Data.Domain)
	return nil
}

func writeConfigFromCredentials(path, appURL, computerID, computerToken, domain, defaultUser string) error {
	cfg := map[string]any{
		"computerId":         computerID,
		"computerResourceId": computerID,
		"appUrl":             appURL,
		"domain":             domain,
		"jwksUrl":            appURL + "/api/cloud-computers/jwks",
		"computerToken":      computerToken,
		"defaultBackendPort": 80,
		"defaultUser":        defaultUser,
	}
	bytes, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	bytes = append(bytes, '\n')
	if err := os.WriteFile(path, bytes, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	return nil
}

func kernelVersionForUp() string {
	if runtime.GOOS == "linux" {
		out, err := exec.Command("uname", "-r").Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
	}
	return ""
}
