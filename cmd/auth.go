package cmd

import (
	"fmt"
	"strings"

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

  # Paste an existing API key (get one from https://idapt.ai/settings/api-keys)
  idapt auth login --api-key uk_...

  # Or sign in with email+password (creates a session-derived token)
  idapt auth login --email you@example.com --password ...

Saved credentials live at ` + "`$XDG_CONFIG_HOME/idapt/credentials.json`" + ` on
Linux (mode 0600); ` + "`~/Library/Application Support/idapt/`" + ` on macOS;
` + "`%AppData%\\idapt\\`" + ` on Windows.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		apiKey, _ := cmd.Flags().GetString("api-key")

		if apiKey != "" {
			if !strings.HasPrefix(apiKey, "uk_") && !strings.HasPrefix(apiKey, "ak_") && !strings.HasPrefix(apiKey, "pk_") {
				return fmt.Errorf("API key must start with uk_, ak_, or pk_")
			}
			credPath, err := credential.DefaultPath()
			if err != nil {
				return fmt.Errorf("cannot determine credentials path: %w", err)
			}
			creds := credential.Credentials{APIKey: apiKey}
			if err := credential.Save(credPath, creds); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "API key saved successfully.")
			printPostLoginNextSteps(cmd)
			return nil
		}

		email, _ := cmd.Flags().GetString("email")
		password, _ := cmd.Flags().GetString("password")
		if email == "" {
			return fmt.Errorf("--email is required (or use --api-key)")
		}
		if password == "" {
			return fmt.Errorf("--password is required")
		}

		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		var resp map[string]interface{}
		err = client.Post(cmd.Context(), "/api/auth/sign-in/email", map[string]string{
			"email":    email,
			"password": password,
		}, &resp)
		if err != nil {
			return err
		}

		fmt.Fprintln(cmd.OutOrStdout(), "Login successful.")
		printPostLoginNextSteps(cmd)
		return nil
	},
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
	authLoginCmd.Flags().String("api-key", "", "API key (uk_, ak_, pk_)")
	authLoginCmd.Flags().String("email", "", "Email address")
	authLoginCmd.Flags().String("password", "", "Password")

	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authStatusCmd)
}
