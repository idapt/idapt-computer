package cmd

import (
	"fmt"
	"net/url"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/input"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/idapt/idapt-cli/internal/resolve"
	"github.com/spf13/cobra"
)

func resolveComputer(cmd *cobra.Command, f *cmdutil.Factory, nameOrID string) (string, error) {
	if resolve.IsResourceId(nameOrID) {
		return nameOrID, nil
	}
	workspaceID, err := resolveWorkspaceFlag(cmd, f)
	if err != nil {
		return "", err
	}
	return resolveResource(cmd, f, "computer", nameOrID, workspaceID)
}

var computerListCmd = &cobra.Command{
	Use:   "list",
	Short: "List computers",
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		workspaceID, err := resolveWorkspaceFlag(cmd, f)
		if err != nil {
			return err
		}
		q := buildListQuery(cmd, url.Values{"workspace_id": {workspaceID}})
		if cmd.Flags().Changed("include-archived") {
			q.Set("include_archived", "true")
		}
		var resp api.V1ListResponse
		if err := client.Get(cmd.Context(), "/api/v1/computers", q, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteList(resp.Data, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "NAME", Field: "name"},
			{Header: "TYPE", Field: "type"},
			{Header: "STATE", Field: "state"},
			{Header: "HOSTNAME", Field: "hostname"},
			{Header: "PORT", Field: "port"},
			{Header: "WORKSPACE_ID", Field: "workspace_id"},
		})
	},
}

var computerCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a daemon pairing token for a user-cloud computer",
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		workspaceID, err := resolveWorkspaceFlag(cmd, f)
		if err != nil {
			return err
		}
		body := map[string]interface{}{"workspace_id": workspaceID}
		if cmd.Flags().Changed("json") {
			raw, _ := cmd.Flags().GetString("json")
			parsed, err := input.ParseJSONFlag(raw, f.In)
			if err != nil {
				return err
			}
			body = input.MergeFlags(parsed, map[string]interface{}{"workspace_id": workspaceID})
		}
		overrides := map[string]interface{}{}
		for _, p := range []struct{ flag, field string }{
			{"name", "intended_name"},
		} {
			if cmd.Flags().Changed(p.flag) {
				v, _ := cmd.Flags().GetString(p.flag)
				overrides[p.field] = v
			}
		}
		body = input.MergeFlags(body, overrides)

		var resp api.V1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/computers/pair-tokens", body, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteItem(resp.Data, []output.Column{
			{Header: "EXPIRES_AT", Field: "expires_at"},
			{Header: "INSTALL_COMMAND", Field: "install_command", Width: 100},
		})
	},
}

var computerGetCmd = &cobra.Command{
	Use:   "get <id-or-name>",
	Short: "Get computer details",
	Args:  cobra.ExactArgs(1),
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
		var resp api.V1ItemResponse
		if err := client.Get(cmd.Context(), "/api/v1/computers/"+id, nil, &resp); err != nil {
			return err
		}
		return writeComputerItem(f, resp.Data)
	},
}

var computerEditCmd = &cobra.Command{
	Use:   "edit <id-or-name>",
	Short: "Edit a computer",
	Args:  cobra.ExactArgs(1),
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
		if cmd.Flags().Changed("json") {
			raw, _ := cmd.Flags().GetString("json")
			parsed, err := input.ParseJSONFlag(raw, f.In)
			if err != nil {
				return err
			}
			body = parsed
		}
		for _, p := range []struct{ flag, field string }{
			{"name", "name"},
			{"description", "description"},
			{"icon", "icon"},
			{"logging-level", "logging_level"},
		} {
			if cmd.Flags().Changed(p.flag) {
				v, _ := cmd.Flags().GetString(p.flag)
				body[p.field] = v
			}
		}
		var resp api.V1ItemResponse
		if err := client.Patch(cmd.Context(), "/api/v1/computers/"+id, body, &resp); err != nil {
			return err
		}
		return writeComputerItem(f, resp.Data)
	},
}

var computerDeleteCmd = &cobra.Command{
	Use:   "delete <id-or-name>",
	Short: "Delete a computer",
	Args:  cobra.ExactArgs(1),
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
			if !cmdutil.ConfirmAction(f, fmt.Sprintf("Delete computer %s?", args[0])) {
				return fmt.Errorf("aborted")
			}
		}
		if err := client.Delete(cmd.Context(), "/api/v1/computers/"+id); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Computer %s deleted.\n", args[0])
		return nil
	},
}

var computerArchiveCmd = &cobra.Command{
	Use:   "archive <id-or-name>",
	Short: "Archive a computer (reversible — hidden from default list)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runComputerLifecycle(cmd, args[0], "archive", "archived")
	},
}

var computerUnarchiveCmd = &cobra.Command{
	Use:   "unarchive <id-or-name>",
	Short: "Restore a previously archived computer",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runComputerLifecycle(cmd, args[0], "unarchive", "unarchived")
	},
}

var computerStartCmd = &cobra.Command{
	Use:   "start <id-or-name>",
	Short: "Start or resume a cloud computer",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runComputerLifecycle(cmd, args[0], "start", "starting")
	},
}

var computerSleepCmd = &cobra.Command{
	Use:   "sleep <id-or-name>",
	Short: "Sleep a running microVM cloud computer",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runComputerLifecycle(cmd, args[0], "sleep", "sleeping")
	},
}

var computerStopCmd = &cobra.Command{
	Use:   "stop <id-or-name>",
	Short: "Power off a provisioned cloud computer",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runComputerLifecycle(cmd, args[0], "stop", "stopping")
	},
}

var computerHibernateCmd = &cobra.Command{
	Use:   "hibernate <id-or-name>",
	Short: "Snapshot, unprovision, and store a cloud computer",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runComputerLifecycle(cmd, args[0], "hibernate", "hibernating")
	},
}

var computerTestCmd = &cobra.Command{
	Use:     "test <id-or-name>",
	Aliases: []string{"test-connection"},
	Short:   "Run a daemon connectivity probe",
	Args:    cobra.ExactArgs(1),
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
		var resp api.V1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/computers/"+id+"/test-connection", nil, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteItem(resp.Data, []output.Column{
			{Header: "SUCCESS", Field: "success"},
			{Header: "DURATION (ms)", Field: "duration_ms"},
			{Header: "SERVER VERSION", Field: "server_info.version"},
			{Header: "UPTIME (s)", Field: "server_info.uptime_seconds"},
			{Header: "ERROR", Field: "error"},
		})
	},
}

func runComputerLifecycle(cmd *cobra.Command, idOrName, path, verbDone string) error {
	f := cmdutil.FactoryFromCmd(cmd)
	client, err := f.APIClient()
	if err != nil {
		return err
	}
	id, err := resolveComputer(cmd, f, idOrName)
	if err != nil {
		return err
	}
	var resp api.V1ItemResponse
	if err := client.Post(cmd.Context(), "/api/v1/computers/"+id+"/"+path, nil, &resp); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Computer %s %s.\n", idOrName, verbDone)
	return writeComputerItem(f, resp.Data)
}

func writeComputerItem(f *cmdutil.Factory, item map[string]interface{}) error {
	return f.Formatter().WriteItem(item, []output.Column{
		{Header: "ID", Field: "id"},
		{Header: "NAME", Field: "name"},
		{Header: "TYPE", Field: "type"},
		{Header: "STATE", Field: "state"},
		{Header: "HOSTNAME", Field: "hostname"},
		{Header: "PORT", Field: "port"},
		{Header: "LOGGING_LEVEL", Field: "logging_level"},
		{Header: "WORKSPACE_ID", Field: "workspace_id"},
		{Header: "ARCHIVED_AT", Field: "archived_at"},
		{Header: "DESCRIPTION", Field: "description", Width: 60},
	})
}

func init() {
	cmdutil.AddListFlags(computerListCmd)
	computerListCmd.Flags().Bool("include-archived", false, "Include archived computers")

	computerCreateCmd.Flags().String("name", "", "Computer name (subdomain-safe slug)")
	cmdutil.AddJSONInput(computerCreateCmd)

	computerEditCmd.Flags().String("name", "", "Computer name")
	computerEditCmd.Flags().String("description", "", "Description")
	computerEditCmd.Flags().String("icon", "", "Icon emoji")
	computerEditCmd.Flags().String("logging-level", "", "minimal | standard | full")
	cmdutil.AddJSONInput(computerEditCmd)
}
