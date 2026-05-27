package cmd

import (
	"fmt"
	"net/url"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/spf13/cobra"
)

var computerEnvVarCmd = &cobra.Command{
	Use:     "env",
	Aliases: []string{"env-var"},
	Short:   "Manage environment variables for computer users",
}

var computerEnvVarListCmd = &cobra.Command{
	Use:   "list <computer-id-or-name> <username>",
	Short: "List environment variables for a computer user (names only, no values)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		id, err := resolveComputer(cmd, f, args[0])
		if err != nil {
			return err
		}

		var resp api.V1ListResponse
		path := "/api/v1/computers/" + id + "/users/" + url.PathEscape(args[1]) + "/env"
		if err := client.Get(cmd.Context(), path, nil, &resp); err != nil {
			return err
		}

		formatter := f.Formatter()
		return formatter.WriteList(resp.Data, []output.Column{
			{Header: "NAME", Field: "name"},
			{Header: "FILE_ID", Field: "file_id"},
			{Header: "CREATED", Field: "created_at"},
		})
	},
}

var computerEnvVarSetCmd = &cobra.Command{
	Use:   "set <computer-id-or-name> <username>",
	Short: "Set an environment variable from a credential file or inline value",
	Long: `Set an environment variable for a computer user.

The value can come from:
  --file-id   Existing credential file resourceId (recommended for security)
  --name/--value  Inline name+value (creates a credential file automatically)

Examples:
  idapt computer env set my-server root --name DATABASE_URL --value "postgres://..."
  idapt computer env set my-server deploy --file-id abc-123-def`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		id, err := resolveComputer(cmd, f, args[0])
		if err != nil {
			return err
		}

		body := map[string]interface{}{}

		fileID, _ := cmd.Flags().GetString("file-id")
		name, _ := cmd.Flags().GetString("name")
		value, _ := cmd.Flags().GetString("value")

		if fileID != "" {
			body["file_id"] = fileID
		} else if name != "" && value != "" {
			body["name"] = name
			body["value"] = value
		} else {
			return fmt.Errorf("provide either --file-id or both --name and --value")
		}

		var resp api.V1ItemResponse
		path := "/api/v1/computers/" + id + "/users/" + url.PathEscape(args[1]) + "/env"
		if err := client.Post(cmd.Context(), path, body, &resp); err != nil {
			return err
		}

		if name != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Environment variable $%s set.\n", name)
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "Environment variable set from credential file.")
		}
		return nil
	},
}

var computerEnvVarDeleteCmd = &cobra.Command{
	Use:   "delete <computer-id-or-name> <username> <env-var-name>",
	Short: "Delete an environment variable binding (by env-var name)",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		id, err := resolveComputer(cmd, f, args[0])
		if err != nil {
			return err
		}

		if !globalFlags.Confirm {
			if !cmdutil.ConfirmAction(f, fmt.Sprintf("Delete environment variable %s?", args[2])) {
				return fmt.Errorf("aborted")
			}
		}

		path := "/api/v1/computers/" + id + "/users/" + url.PathEscape(args[1]) + "/env/" + url.PathEscape(args[2])
		if err := client.Delete(cmd.Context(), path); err != nil {
			return err
		}

		fmt.Fprintln(cmd.OutOrStdout(), "Environment variable removed.")
		return nil
	},
}

var computerEnvVarSetupCmd = &cobra.Command{
	Use:   "setup <computer-id-or-name> <username>",
	Short: "Setup shell integration for environment variables (~/.idapt-env + .bashrc)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		id, err := resolveComputer(cmd, f, args[0])
		if err != nil {
			return err
		}

		var resp map[string]interface{}
		path := "/api/v1/computers/" + id + "/users/" + url.PathEscape(args[1]) + "/env/setup"
		if err := client.Post(cmd.Context(), path, nil, &resp); err != nil {
			return err
		}

		fmt.Fprintln(cmd.OutOrStdout(), "Shell integration configured.")
		return nil
	},
}

var computerEnvVarSyncCmd = &cobra.Command{
	Use:   "sync <computer-id-or-name> <username>",
	Short: "Check or repair sync status between DB and ~/.idapt-env",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		id, err := resolveComputer(cmd, f, args[0])
		if err != nil {
			return err
		}

		repair, _ := cmd.Flags().GetBool("repair")
		path := "/api/v1/computers/" + id + "/users/" + url.PathEscape(args[1]) + "/env/sync"

		if repair {
			var resp map[string]interface{}
			if err := client.Post(cmd.Context(), path, nil, &resp); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Environment variables re-synced.")
			return nil
		}

		var resp api.V1ItemResponse
		if err := client.Get(cmd.Context(), path, nil, &resp); err != nil {
			return err
		}

		isInSync, _ := resp.Data["is_in_sync"].(bool)
		if isInSync {
			fmt.Fprintln(cmd.OutOrStdout(), "In sync.")
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "Out of sync. Use --repair to fix.")
			formatter := f.Formatter()
			return formatter.WriteList(api.AsMapSlice(resp.Data["entries"]), []output.Column{
				{Header: "NAME", Field: "name"},
				{Header: "ON DISK", Field: "present_on_disk"},
			})
		}
		return nil
	},
}

func init() {
	computerEnvVarSetCmd.Flags().String("file-id", "", "Credential file resourceId to bind")
	computerEnvVarSetCmd.Flags().String("name", "", "Environment variable name")
	computerEnvVarSetCmd.Flags().String("value", "", "Environment variable value (creates credential file)")

	computerEnvVarSyncCmd.Flags().Bool("repair", false, "Repair sync by rewriting ~/.idapt-env from DB state")

	computerEnvVarCmd.AddCommand(computerEnvVarListCmd)
	computerEnvVarCmd.AddCommand(computerEnvVarSetCmd)
	computerEnvVarCmd.AddCommand(computerEnvVarDeleteCmd)
	computerEnvVarCmd.AddCommand(computerEnvVarSetupCmd)
	computerEnvVarCmd.AddCommand(computerEnvVarSyncCmd)
}
