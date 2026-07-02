package cmd

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/idapt/idapt-computer/internal/api"
	"github.com/idapt/idapt-computer/internal/cmdutil"
	"github.com/idapt/idapt-computer/internal/credential"
	"github.com/idapt/idapt-computer/internal/output"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate with idapt",
	Long: `Manage CLI credentials.

Verbs:
  idapt-computer auth login    Sign in in your browser (or paste an API key)
  idapt-computer auth logout   Clear the stored credential
  idapt-computer auth status   Show the current identity (or print the recovery hint)

Credential precedence used by every other command:
  1. --api-key flag        (per-invocation)
  2. IDAPT_API_KEY env     (process-wide; useful under sudo and in CI)
  3. credentials.json file (written by ` + "`idapt-computer auth login`" + `)`,
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login to idapt",
	Long: `Sign in and persist the credential to the per-OS user config dir.

Three ways to authenticate:

  # Browser sign-in (device authorization, RFC 8628). The CLI prints a URL and
  # a code; you approve in your browser and the CLI receives a session token. No
  # password ever touches the CLI, and it works for Google / GitHub / SSO
  # accounts that have no password. Works headless / over SSH:
  idapt-computer auth login

  # Browser sign-in (OAuth authorization-code + PKCE, RFC 7636 / 8252). The CLI
  # opens your browser and catches the redirect on a local loopback port — no
  # code to copy. Best on a machine with its own browser:
  idapt-computer auth login --web

  # Or paste an existing API key (create one at <app>/settings/api-keys) — handy
  # for CI and headless machines:
  idapt-computer auth login --api-key uk_...
  printf '%s' "$KEY" | idapt-computer auth login --api-key-stdin

Avoid --api-key with an inline value: it lands in your shell history and the
process list. Prefer --api-key-stdin or the IDAPT_API_KEY env var.

Saved credentials live at ` + "`$XDG_CONFIG_HOME/idapt/credentials.json`" + ` on
Linux (mode 0600); ` + "`~/Library/Application Support/idapt/`" + ` on macOS;
` + "`%AppData%\\idapt\\`" + ` on Windows.`,
	RunE: runAuthLogin,
}

func runAuthLogin(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()

	apiKey, err := resolveAPIKeyInput(cmd)
	if err != nil {
		return err
	}
	if apiKey != "" {
		if !validAPIKeyPrefix(apiKey) {
			return fmt.Errorf("API key must start with uk_, ak_, or pk_")
		}
		if err := saveAPIKey(apiKey); err != nil {
			return err
		}
		fmt.Fprintf(out, "API key saved (%s).\n", maskKey(apiKey))
		printPostLoginNextSteps(cmd)
		return nil
	}

	f := cmdutil.FactoryFromCmd(cmd)
	client, err := f.APIClient()
	if err != nil {
		return err
	}
	baseURL := client.BaseURL()

	if web, _ := cmd.Flags().GetBool("web"); web {
		tokens, lerr := api.LoginAuthCode(cmd.Context(), baseURL, Version, out, launchBrowser)
		if lerr != nil {
			return mapOAuthLoginError(lerr)
		}
		if err := saveOAuthTokens(tokens); err != nil {
			return err
		}
		fmt.Fprintln(out, "\nLogin successful.")
		printPostLoginNextSteps(cmd)
		return nil
	}

	token, err := api.LoginDevice(cmd.Context(), baseURL, Version, out)
	if err != nil {
		return mapDeviceLoginError(err)
	}
	if err := saveAPIKey(token); err != nil {
		return err
	}
	fmt.Fprintln(out, "\nLogin successful.")
	printPostLoginNextSteps(cmd)
	return nil
}

func mapOAuthLoginError(err error) error {
	switch {
	case errors.Is(err, api.ErrOAuthAccessDenied):
		return errors.New("the login request was denied in the browser")
	case errors.Is(err, api.ErrOAuthTimedOut):
		return errors.New("timed out waiting for browser sign-in — run `idapt-computer auth login --web` again, or use `idapt-computer auth login` (device code) on headless machines")
	case errors.Is(err, api.ErrOAuthStateMismatch):
		return errors.New("the browser sign-in could not be verified — please try again")
	default:
		return err
	}
}

func resolveAPIKeyInput(cmd *cobra.Command) (string, error) {
	keyStdin, _ := cmd.Flags().GetBool("api-key-stdin")
	keyFlag, _ := cmd.Flags().GetString("api-key")
	if keyStdin {
		if keyFlag != "" {
			return "", errors.New("--api-key and --api-key-stdin are mutually exclusive")
		}
		return readSecretStdin(cmd.InOrStdin())
	}
	if keyFlag != "" {
		warnInsecureFlag(cmd.ErrOrStderr(), "--api-key",
			"Pipe it via --api-key-stdin or set the IDAPT_API_KEY env var instead")
		return keyFlag, nil
	}
	return "", nil
}

func mapDeviceLoginError(err error) error {
	switch {
	case errors.Is(err, api.ErrDeviceCodeExpired):
		return errors.New("the login code expired before approval — run `idapt-computer auth login` again")
	case errors.Is(err, api.ErrDeviceAccessDenied):
		return errors.New("the login request was denied in the browser")
	default:
		return err
	}
}

func validAPIKeyPrefix(key string) bool {
	return strings.HasPrefix(key, "uk_") ||
		strings.HasPrefix(key, "ak_") ||
		strings.HasPrefix(key, "pk_")
}

func maskKey(key string) string {
	if len(key) <= 12 {
		return "uk_…"
	}
	return key[:6] + "…" + key[len(key)-4:]
}

func saveAPIKey(key string) error {
	credPath, err := credential.DefaultPath()
	if err != nil {
		return fmt.Errorf("cannot determine credentials path: %w", err)
	}
	return credential.Save(credPath, credential.Credentials{APIKey: key})
}

func saveOAuthTokens(tokens *api.OAuthTokens) error {
	credPath, err := credential.DefaultPath()
	if err != nil {
		return fmt.Errorf("cannot determine credentials path: %w", err)
	}
	return credential.Save(credPath, credential.Credentials{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    time.Now().Unix() + int64(tokens.ExpiresIn),
	})
}

func printPostLoginNextSteps(cmd *cobra.Command) {
	fmt.Fprintln(cmd.OutOrStdout(), "")
	fmt.Fprintln(cmd.OutOrStdout(), "Next steps:")
	fmt.Fprintln(cmd.OutOrStdout(), "  idapt-computer auth status           # verify identity")
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Clear stored credentials",
	RunE: func(cmd *cobra.Command, args []string) error {
		credPath, err := credential.DefaultPath()
		if err != nil {
			return fmt.Errorf("cannot determine credentials path: %w", err)
		}
		if err := credential.Clear(credPath); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Logged out.")
		return nil
	},
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current authentication status",
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		if err := cmdutil.RequireAuth(f); err != nil {
			return err
		}
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		var resp struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := client.Get(cmd.Context(), "/api/v1/me", nil, &resp); err != nil {
			return cmdutil.WrapAPIError(err)
		}
		user := resp.Data

		formatter := f.Formatter()
		return formatter.WriteItem(user, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "Email", Field: "email"},
			{Header: "Name", Field: "name"},
			{Header: "Slug", Field: "slug"},
		})
	},
}

func init() {
	authLoginCmd.Flags().String("api-key", "", "API key (uk_, ak_, pk_) — INSECURE inline; prefer --api-key-stdin or IDAPT_API_KEY")
	authLoginCmd.Flags().Bool("api-key-stdin", false, "Read the API key from stdin")
	authLoginCmd.Flags().Bool("web", false, "Sign in via the browser using OAuth authorization-code + PKCE (loopback redirect) instead of the device-code flow")

	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authStatusCmd)
}
