package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/credential"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate with idapt",
	Long: `Manage CLI credentials.

Verbs:
  idapt auth login    Sign in with email+password or paste an API key
  idapt auth logout   Clear the stored credential
  idapt auth status   Show the current identity (or print the recovery hint)

Credential precedence used by every other command:
  1. --api-key flag        (per-invocation)
  2. IDAPT_API_KEY env     (process-wide; useful under sudo and in CI)
  3. credentials.json file (written by ` + "`idapt auth login`" + `)`,
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login to idapt",
	Long: `Sign in and persist the credential to the per-OS user config dir.

Two ways to authenticate:

  # Sign in with email+password. The CLI exchanges your credentials for a
  # long-lived, revocable API key and stores THAT (never your password).
  # Run it bare to be prompted interactively (password input is hidden):
  idapt auth login
  idapt auth login --email you@example.com           # prompts for the password
  printf '%s' "$PASS" | idapt auth login --email you@example.com --password-stdin

  # Or paste an existing API key (create one at <app>/settings/api-keys):
  idapt auth login --api-key uk_...
  printf '%s' "$KEY" | idapt auth login --api-key-stdin

Avoid --password / --api-key with an inline value: it lands in your shell
history and the process list. Prefer the prompt, the --*-stdin flags, or the
IDAPT_API_KEY env var.

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

	email, password, err := resolveEmailPassword(cmd)
	if err != nil {
		return err
	}

	f := cmdutil.FactoryFromCmd(cmd)
	client, err := f.APIClient()
	if err != nil {
		return err
	}
	baseURL := client.BaseURL()

	hostname, _ := os.Hostname()
	key, err := api.LoginEmailPassword(cmd.Context(), baseURL, Version, email, password, apiKeyName(hostname))
	if err != nil {
		return mapLoginError(err, baseURL)
	}
	if err := saveAPIKey(key); err != nil {
		return err
	}
	fmt.Fprintf(out, "Login successful — saved API key %s.\n", maskKey(key))
	printPostLoginNextSteps(cmd)
	return nil
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

func resolveEmailPassword(cmd *cobra.Command) (email, password string, err error) {
	emailFlag, _ := cmd.Flags().GetString("email")
	passwordFlag, _ := cmd.Flags().GetString("password")
	passwordStdin, _ := cmd.Flags().GetBool("password-stdin")

	if passwordStdin {
		if passwordFlag != "" {
			return "", "", errors.New("--password and --password-stdin are mutually exclusive")
		}
		if emailFlag == "" {
			return "", "", errors.New("--email is required when using --password-stdin")
		}
		pw, readErr := readSecretStdin(cmd.InOrStdin())
		if readErr != nil {
			return "", "", readErr
		}
		return emailFlag, pw, nil
	}

	email = emailFlag
	if email == "" {
		if stdinIsTTY() {
			email, err = promptLine(cmd.InOrStdin(), cmd.ErrOrStderr(), "Email: ")
			if err != nil {
				return "", "", err
			}
		}
		if email == "" {
			return "", "", errors.New("--email is required (or use --api-key)")
		}
	}

	if passwordFlag != "" {
		warnInsecureFlag(cmd.ErrOrStderr(), "--password",
			"Use --password-stdin or the interactive prompt instead")
		return email, passwordFlag, nil
	}
	if stdinIsTTY() {
		pw, promptErr := promptNoEcho("Password: ", cmd.ErrOrStderr())
		if promptErr != nil {
			return "", "", promptErr
		}
		if pw == "" {
			return "", "", errors.New("password was empty")
		}
		return email, pw, nil
	}
	return "", "", errors.New("no password provided — pass --password-stdin or run in an interactive terminal")
}

func mapLoginError(err error, baseURL string) error {
	base := strings.TrimRight(baseURL, "/")
	switch {
	case errors.Is(err, api.ErrAPIKeyPaidPlanRequired):
		return fmt.Errorf(
			"your account can't create CLI API keys on its current plan.\n"+
				"  • Upgrade at %s/settings, then re-run `idapt auth login`, or\n"+
				"  • Paste an existing key:  idapt auth login --api-key-stdin   (create one at %s/settings/api-keys)",
			base, base)
	case errors.Is(err, api.ErrLoginInvalidCredentials):
		return errors.New("sign-in failed: check your email and password (and that your email is verified)")
	default:
		return err
	}
}

func validAPIKeyPrefix(key string) bool {
	return strings.HasPrefix(key, "uk_") ||
		strings.HasPrefix(key, "ak_") ||
		strings.HasPrefix(key, "pk_")
}

func apiKeyName(hostname string) string {
	if hostname == "" {
		hostname = "unknown-host"
	}
	name := fmt.Sprintf("idapt CLI (%s)", hostname)
	if len(name) > 80 {
		name = name[:80]
	}
	return name
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

func printPostLoginNextSteps(cmd *cobra.Command) {
	fmt.Fprintln(cmd.OutOrStdout(), "")
	fmt.Fprintln(cmd.OutOrStdout(), "Next steps:")
	fmt.Fprintln(cmd.OutOrStdout(), "  idapt auth status           # verify identity")
	fmt.Fprintln(cmd.OutOrStdout(), "  idapt                        # open the interactive TUI")
	fmt.Fprintln(cmd.OutOrStdout(), "  idapt -p \"hello\"             # one-shot prompt")
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
	authLoginCmd.Flags().String("email", "", "Email address")
	authLoginCmd.Flags().String("password", "", "Password — INSECURE inline; prefer --password-stdin or the interactive prompt")
	authLoginCmd.Flags().Bool("password-stdin", false, "Read the password from stdin")

	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authStatusCmd)
}
