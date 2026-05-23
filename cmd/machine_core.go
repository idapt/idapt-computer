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

func resolveMachine(cmd *cobra.Command, f *cmdutil.Factory, nameOrID string) (string, error) {
	if resolve.IsResourceId(nameOrID) {
		return nameOrID, nil
	}
	projectID, err := resolveProjectFlag(cmd, f)
	if err != nil {
		return "", err
	}
	return resolveResource(cmd, f, "machine", nameOrID, projectID)
}

var machineListCmd = &cobra.Command{
	Use:   "list",
	Short: "List machines",
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		projectID, err := resolveProjectFlag(cmd, f)
		if err != nil {
			return err
		}
		q := buildListQuery(cmd, url.Values{"project_id": {projectID}})
		if cmd.Flags().Changed("include-archived") {
			q.Set("include_archived", "true")
		}
		var resp api.V1ListResponse
		if err := client.Get(cmd.Context(), "/api/v1/machines", q, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteList(resp.Data, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "NAME", Field: "name"},
			{Header: "TYPE", Field: "type"},
			{Header: "STATE", Field: "state"},
			{Header: "HOSTNAME", Field: "hostname"},
			{Header: "PORT", Field: "port"},
			{Header: "PROJECT_ID", Field: "project_id"},
		})
	},
}

var machineCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a daemon pairing token for a user-managed machine",
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		projectID, err := resolveProjectFlag(cmd, f)
		if err != nil {
			return err
		}
		body := map[string]interface{}{"project_id": projectID}
		if cmd.Flags().Changed("json") {
			raw, _ := cmd.Flags().GetString("json")
			parsed, err := input.ParseJSONFlag(raw, f.In)
			if err != nil {
				return err
			}
			body = input.MergeFlags(parsed, map[string]interface{}{"project_id": projectID})
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
		if err := client.Post(cmd.Context(), "/api/v1/machines/pair-tokens", body, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteItem(resp.Data, []output.Column{
			{Header: "EXPIRES_AT", Field: "expires_at"},
			{Header: "INSTALL_COMMAND", Field: "install_command", Width: 100},
		})
	},
}

var machineGetCmd = &cobra.Command{
	Use:   "get <id-or-name>",
	Short: "Get machine details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		id, err := resolveMachine(cmd, f, args[0])
		if err != nil {
			return err
		}
		var resp api.V1ItemResponse
		if err := client.Get(cmd.Context(), "/api/v1/machines/"+id, nil, &resp); err != nil {
			return err
		}
		return writeMachineItem(f, resp.Data)
	},
}

var machineEditCmd = &cobra.Command{
	Use:   "edit <id-or-name>",
	Short: "Edit a machine",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		id, err := resolveMachine(cmd, f, args[0])
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
		if err := client.Patch(cmd.Context(), "/api/v1/machines/"+id, body, &resp); err != nil {
			return err
		}
		return writeMachineItem(f, resp.Data)
	},
}

var machineDeleteCmd = &cobra.Command{
	Use:   "delete <id-or-name>",
	Short: "Delete a machine",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		id, err := resolveMachine(cmd, f, args[0])
		if err != nil {
			return err
		}
		if !globalFlags.Confirm {
			if !cmdutil.ConfirmAction(f, fmt.Sprintf("Delete machine %s?", args[0])) {
				return fmt.Errorf("aborted")
			}
		}
		if err := client.Delete(cmd.Context(), "/api/v1/machines/"+id); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Machine %s deleted.\n", args[0])
		return nil
	},
}

var machineArchiveCmd = &cobra.Command{
	Use:   "archive <id-or-name>",
	Short: "Archive a machine (reversible — hidden from default list)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMachineLifecycle(cmd, args[0], "archive", "archived")
	},
}

var machineUnarchiveCmd = &cobra.Command{
	Use:   "unarchive <id-or-name>",
	Short: "Restore a previously archived machine",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMachineLifecycle(cmd, args[0], "unarchive", "unarchived")
	},
}

var machineStartCmd = &cobra.Command{
	Use:   "start <id-or-name>",
	Short: "Start or resume a managed machine",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMachineLifecycle(cmd, args[0], "start", "starting")
	},
}

var machineSleepCmd = &cobra.Command{
	Use:   "sleep <id-or-name>",
	Short: "Sleep a running microVM managed machine",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMachineLifecycle(cmd, args[0], "sleep", "sleeping")
	},
}

var machineStopCmd = &cobra.Command{
	Use:   "stop <id-or-name>",
	Short: "Power off a provisioned managed machine",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMachineLifecycle(cmd, args[0], "stop", "stopping")
	},
}

var machineHibernateCmd = &cobra.Command{
	Use:   "hibernate <id-or-name>",
	Short: "Snapshot, unprovision, and store a managed machine",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMachineLifecycle(cmd, args[0], "hibernate", "hibernating")
	},
}

var machineTestCmd = &cobra.Command{
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
		id, err := resolveMachine(cmd, f, args[0])
		if err != nil {
			return err
		}
		var resp api.V1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/machines/"+id+"/test-connection", nil, &resp); err != nil {
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

func runMachineLifecycle(cmd *cobra.Command, idOrName, path, verbDone string) error {
	f := cmdutil.FactoryFromCmd(cmd)
	client, err := f.APIClient()
	if err != nil {
		return err
	}
	id, err := resolveMachine(cmd, f, idOrName)
	if err != nil {
		return err
	}
	var resp api.V1ItemResponse
	if err := client.Post(cmd.Context(), "/api/v1/machines/"+id+"/"+path, nil, &resp); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Machine %s %s.\n", idOrName, verbDone)
	return writeMachineItem(f, resp.Data)
}

func writeMachineItem(f *cmdutil.Factory, item map[string]interface{}) error {
	return f.Formatter().WriteItem(item, []output.Column{
		{Header: "ID", Field: "id"},
		{Header: "NAME", Field: "name"},
		{Header: "TYPE", Field: "type"},
		{Header: "STATE", Field: "state"},
		{Header: "HOSTNAME", Field: "hostname"},
		{Header: "PORT", Field: "port"},
		{Header: "LOGGING_LEVEL", Field: "logging_level"},
		{Header: "PROJECT_ID", Field: "project_id"},
		{Header: "ARCHIVED_AT", Field: "archived_at"},
		{Header: "DESCRIPTION", Field: "description", Width: 60},
	})
}

func init() {
	cmdutil.AddListFlags(machineListCmd)
	machineListCmd.Flags().Bool("include-archived", false, "Include archived machines")

	machineCreateCmd.Flags().String("name", "", "Machine name (subdomain-safe slug)")
	cmdutil.AddJSONInput(machineCreateCmd)

	machineEditCmd.Flags().String("name", "", "Machine name")
	machineEditCmd.Flags().String("description", "", "Description")
	machineEditCmd.Flags().String("icon", "", "Icon emoji")
	machineEditCmd.Flags().String("logging-level", "", "minimal | standard | full")
	cmdutil.AddJSONInput(machineEditCmd)
}
