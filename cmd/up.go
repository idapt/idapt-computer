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
	osuser "os/user"
	"runtime"
	"strings"
	"time"

	"github.com/idapt/idapt-computer/internal/cmdutil"
	"github.com/idapt/idapt-computer/internal/config"
	"github.com/idapt/idapt-computer/internal/deviceflow"
	"github.com/idapt/idapt-computer/internal/progress"
	"github.com/spf13/cobra"
)

var (
	upToken       string
	upTokenStdin  bool
	upCode        string
	upAppURL      string
	upConfigPath  string
	upNoService   bool
	upForce       bool
	upMode        string
	upUserMode    bool
	upSystemMode  bool
	upYes         bool
	upDefaultUser string
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

Workspace: a newly-paired computer lands in your PERSONAL workspace by
default. The browser confirmation page (device flow) pre-selects it and
lets you pick a different workspace before approving; a pre-minted
registration token (--token) carries the workspace chosen when it was minted. Share a
computer's local inference into other workspaces afterwards with
` + "`idapt computer link <computer> --workspace-id <ws>`" + ` (CLI / agent).

Examples:

  idapt-computer up
      Device flow — opens a URL the user confirms in a browser. Lands in
      your personal workspace unless you pick another at confirmation.

  idapt-computer up --code ABCD-2345
      Use a pre-minted code from the web UI ("Add computer" dialog).

  idapt-computer up --token pt_xxx
      Noninteractive registration token flow (CI / mass-provision / install-script).

  idapt-computer up --no-service
      Authorize but don't install the autostart unit. Useful in
      containers where systemd / launchd isn't available.

  idapt-computer up --force
      Allow overwriting an existing pairing (e.g. linking the same
      machine to a different account, which is otherwise refused).`,
	RunE: runUp,
}

func init() {
	upCmd.Flags().StringVar(&upToken, "token", "", "Registration token (CI / install flow) — INSECURE inline; prefer --token-stdin or IDAPT_TOKEN")
	upCmd.Flags().BoolVar(&upTokenStdin, "token-stdin", false, "Read the registration token from stdin")
	upCmd.Flags().StringVar(&upCode, "code", "", "Device code minted elsewhere (paste from the web UI)")
	upCmd.Flags().StringVar(&upAppURL, "app-url", "", "Idapt app URL (defaults to IDAPT_APP_URL or https://idapt.app)")
	upCmd.Flags().StringVar(&upConfigPath, "config", "", "Path to write daemon config (defaults to per-user XDG path; /etc/idapt in system mode)")
	upCmd.Flags().BoolVar(&upNoService, "no-service", false, "Skip installing/starting the autostart unit")
	upCmd.Flags().BoolVar(&upForce, "force", false, "Overwrite an existing pairing without confirmation")
	upCmd.Flags().StringVar(&upMode, "mode", "", "Install mode: user (rootless, recommended) or system (root, Linux-only)")
	upCmd.Flags().BoolVar(&upUserMode, "user", false, "Shorthand for --mode user (rootless per-user service)")
	upCmd.Flags().BoolVar(&upSystemMode, "system", false, "Shorthand for --mode system (root system service, Linux-only)")
	upCmd.Flags().BoolVar(&upYes, "non-interactive", false, "Assume defaults, never prompt (implies user mode unless --system; same as --yes)")
	upCmd.Flags().StringVar(&upDefaultUser, "default-user", "", "Default user the system-mode daemon acts as (defaults to $SUDO_USER)")
	rootCmd.AddCommand(upCmd)

	loginCmd := &cobra.Command{
		Use:    "login",
		Short:  "Alias for `idapt-computer up` (Tailscale-style device flow)",
		Hidden: false,
		RunE:   runUp,
	}
	loginCmd.Flags().AddFlagSet(upCmd.Flags())
	rootCmd.AddCommand(loginCmd)
}

func trustedAppHosts() map[string]struct{} {
	return map[string]struct{}{
		"idapt.app":     {},
		"www.idapt.app": {},
	}
}

func resolveAppURL() string {
	if upAppURL != "" {
		return strings.TrimRight(upAppURL, "/")
	}
	if v := os.Getenv("IDAPT_APP_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "https://idapt.app"
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
			"refusing to register against non-https app URL %q — the registration token and computer credential must never travel over http (set IDAPT_ALLOW_INSECURE_APP_URL=1 for a trusted self-hosted origin)",
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

func resolveUpConfigPath(mode string) (string, error) {
	return installModeConfigPath(mode, upConfigPath)
}

func upAssumeYes(cmd *cobra.Command) bool {
	if upYes {
		return true
	}
	if globalFlags != nil && globalFlags.Confirm {
		return true
	}
	if v, err := cmd.Flags().GetBool("yes"); err == nil && v {
		return true
	}
	return false
}

func runUp(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	appURL := resolveAppURL()

	installMode, err := resolveInstallMode(
		runtime.GOOS,
		upMode,
		upUserMode,
		upSystemMode,
		stdinInteractive(upAssumeYes(cmd)),
		func() (string, error) { return promptInstallMode(cmd.InOrStdin(), out) },
	)
	if err != nil {
		return err
	}

	configPath, err := resolveUpConfigPath(installMode)
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
				fmt.Fprintln(out, "Run `idapt-computer logout` to clear the pairing first,")
				fmt.Fprintln(out, "or pass --force to overwrite the existing pairing.")
				fmt.Fprintln(out, "")
				fmt.Fprintf(out, "Computer: %s\n", existing.Domain)
				fmt.Fprintf(out, "App URL:  %s\n", existing.AppURL)
				if !upNoService {
					fmt.Fprintln(out, "")
					fmt.Fprintln(out, "Daemon: run `idapt-computer service status` to check the daemon state,")
					fmt.Fprintln(out, "or `idapt-computer service up` to (re)install the autostart unit.")
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
	defaultUser := resolveDefaultUserForMode(installMode, upDefaultUser)

	if w := rootDefaultUserWarning(defaultUser); w != "" {
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, w)
	}

	token, err := resolvePairToken(cmd, upToken, upTokenStdin)
	if err != nil {
		return err
	}

	switch {
	case token != "":
		if err := runTokenFlow(cmd, appURL, token, configPath, hostname, defaultUser, installMode); err != nil {
			return err
		}
	default:
		if err := runDeviceFlow(cmd, appURL, upCode, configPath, hostname, defaultUser, installMode); err != nil {
			return err
		}
	}

	if upNoService {
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "Skipped service install (--no-service).")
		fmt.Fprintln(out, "Start the daemon later with: idapt-computer service up")
		return nil
	}

	fmt.Fprintln(out, "")
	if installMode == InstallModeSystem {
		fmt.Fprintln(out, "Installing daemon system service (root)...")
	} else {
		fmt.Fprintln(out, "Installing daemon autostart unit...")
	}
	if err := installServiceForMode(cmd, installMode, false); err != nil {
		fmt.Fprintf(out, "WARN: service install failed: %v\n", err)
		fmt.Fprintln(out, "Pairing succeeded, but the background daemon did not start automatically.")
		fmt.Fprintln(out, "Run `idapt-computer serve` to start it in the foreground,")
		fmt.Fprintln(out, "or run `idapt-computer service up` again after fixing the autostart issue.")
		return nil
	}
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "All set. Your computer is online.")
	return nil
}

func installServiceForMode(cmd *cobra.Command, mode string, reinstall bool) error {
	if mode == InstallModeSystem {
		return installSystemService(cmd, reinstall)
	}
	if err := serviceUp(cmd, reinstall); err != nil {
		return err
	}
	enableLingerForCurrentUser(cmd)
	return nil
}

func runDeviceFlow(cmd *cobra.Command, appURL, presetCode, configPath, hostname, defaultUser, installMode string) error {
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
			Hostname:      hostname,
			OS:            deviceflow.GuessOS(),
			Arch:          deviceflow.GuessArch(),
			CLIVersion:    Version,
			DefaultUser:   defaultUser,
			HostKind:      deviceflow.GuessHostKind(),
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
		return errors.New("device code expired before approval — run `idapt-computer up` again to get a fresh code")
	case deviceflow.PollNotFound:
		return errors.New("device code not found on the server (was it already used?)")
	case deviceflow.PollCanceled:
		if err != nil {
			return err
		}
		return errors.New("device-flow polling canceled")
	}
	if view == nil || view.ComputerID == "" || view.ComputerToken == "" {
		return errors.New("device flow approved but no credentials were returned (race) — run `idapt-computer up` again")
	}

	if err := writeConfigFromCredentials(configPath, appURL, view.ComputerID, view.ComputerToken, view.Domain, view.TunnelProxyURL, defaultUser); err != nil {
		return err
	}
	fmt.Fprintln(out, "")
	fmt.Fprintf(out, "Linked: %s\n", view.Domain)
	return nil
}

func rootDefaultUserWarning(defaultUser string) string {
	if defaultUser != "root" {
		return ""
	}
	return "Note: this user-mode daemon is running as root, so its default user is\n" +
		"'root' and the AI operates this computer as root. Local AI models work as-is.\n" +
		"For the AI to also run root shell/file commands, install a deliberate root\n" +
		"daemon with 'idapt-computer service elevate'. To keep the AI non-root, re-run\n" +
		"'idapt-computer up' as a normal user."
}

func runTokenFlow(cmd *cobra.Command, appURL, token, configPath, hostname, defaultUser, installMode string) error {
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
		"runs_as_root": os.Geteuid() == 0,
		"install_mode": installMode,
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
			ComputerID     string `json:"computer_id"`
			ComputerToken  string `json:"computer_token"`
			Domain         string `json:"domain"`
			TunnelProxyURL string `json:"tunnel_proxy_url"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if err := writeConfigFromCredentials(configPath, appURL, result.Data.ComputerID, result.Data.ComputerToken, result.Data.Domain, result.Data.TunnelProxyURL, defaultUser); err != nil {
		return err
	}
	fmt.Fprintf(out, "Linked: %s\n", result.Data.Domain)
	return nil
}

func writeConfigFromCredentials(path, appURL, computerID, computerToken, domain, tunnelProxyURL, defaultUser string) error {
	cfg := map[string]any{
		"computerId":         computerID,
		"computerResourceId": computerID,
		"appUrl":             appURL,
		"domain":             domain,
		"jwksUrl":            appURL + "/api/cloud-computers/jwks",
		"computerToken":      computerToken,
		"tunnelProxyUrl":     tunnelProxyURL,
		"defaultBackendPort": 80,
		"defaultUser":        defaultUser,
	}
	return writeStrictJSONFile(path, cfg)
}

func writeStrictJSONFile(path string, v any) error {
	bytes, err := json.MarshalIndent(v, "", "  ")
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

func detectDefaultUser() string {
	currentUsername := ""
	if current, err := osuser.Current(); err == nil && current != nil {
		currentUsername = current.Username
	}
	return defaultUserFromValues(
		os.Getenv("USER"),
		os.Getenv("USERNAME"),
		currentUsername,
	)
}

func defaultUserFromValues(userEnv, usernameEnv, currentUsername string) string {
	for _, candidate := range []string{userEnv, usernameEnv, currentUsername} {
		if normalized := normalizeDefaultUser(candidate); normalized != "" {
			return normalized
		}
	}
	return "user"
}

func normalizeDefaultUser(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if idx := strings.LastIndexAny(value, `\/`); idx >= 0 && idx < len(value)-1 {
		value = value[idx+1:]
	}
	return strings.TrimSpace(value)
}
