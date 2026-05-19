package cmd

import (
	"fmt"
	"net/url"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/input"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/spf13/cobra"
)

var scriptCmd = &cobra.Command{
	Use:   "script",
	Short: "Manage and run scripts (executable files)",
}

var scriptListCmd = &cobra.Command{
	Use:   "list",
	Short: "List scripts in the current project",
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
		var resp api.V1ListResponse
		if err := client.Get(cmd.Context(), "/api/v1/files", q, &resp); err != nil {
			return err
		}
		scripts := []map[string]interface{}{}
		for _, row := range resp.Data {
			name, _ := row["name"].(string)
			if hasScriptExt(name) {
				scripts = append(scripts, row)
			}
		}
		return f.Formatter().WriteList(scripts, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "NAME", Field: "name"},
			{Header: "MIME", Field: "mime_type"},
			{Header: "SIZE", Field: "file_size"},
			{Header: "UPDATED", Field: "updated_at"},
		})
	},
}

func hasScriptExt(name string) bool {
	for _, ext := range []string{".sh", ".bash", ".py", ".js", ".ts", ".mjs", ".rb", ".go", ".rs"} {
		if len(name) > len(ext) && name[len(name)-len(ext):] == ext {
			return true
		}
	}
	return false
}

var scriptCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a script (file with executable extension)",
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
		if cmd.Flags().Changed("name") {
			v, _ := cmd.Flags().GetString("name")
			overrides["name"] = v
		}
		if cmd.Flags().Changed("content") {
			v, _ := cmd.Flags().GetString("content")
			overrides["content"] = v
		}
		if cmd.Flags().Changed("content-file") {
			path, _ := cmd.Flags().GetString("content-file")
			content, err := input.ReadFileFlag(path)
			if err != nil {
				return err
			}
			overrides["content"] = content
		}
		body = input.MergeFlags(body, overrides)

		var resp api.V1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/files", body, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteItem(resp.Data, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "NAME", Field: "name"},
			{Header: "PROJECT_ID", Field: "project_id"},
		})
	},
}

var scriptGetCmd = &cobra.Command{
	Use:   "get <id-or-name>",
	Short: "Get script details",
	Args:  cobra.ExactArgs(1),
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
		id, err := resolveResource(cmd, f, "script", args[0], projectID)
		if err != nil {
			return err
		}
		var resp api.V1ItemResponse
		if err := client.Get(cmd.Context(), "/api/v1/files/"+id, nil, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteItem(resp.Data, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "NAME", Field: "name"},
			{Header: "MIME", Field: "mime_type"},
			{Header: "SIZE", Field: "file_size"},
			{Header: "CREATED", Field: "created_at"},
			{Header: "UPDATED", Field: "updated_at"},
		})
	},
}

var scriptEditCmd = &cobra.Command{
	Use:   "edit <id-or-name>",
	Short: "Edit a script",
	Args:  cobra.ExactArgs(1),
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
		id, err := resolveResource(cmd, f, "script", args[0], projectID)
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
		if cmd.Flags().Changed("name") {
			v, _ := cmd.Flags().GetString("name")
			body["name"] = v
		}
		if cmd.Flags().Changed("content") {
			v, _ := cmd.Flags().GetString("content")
			body["content"] = v
		}
		if cmd.Flags().Changed("content-file") {
			path, _ := cmd.Flags().GetString("content-file")
			content, err := input.ReadFileFlag(path)
			if err != nil {
				return err
			}
			body["content"] = content
		}
		if err := client.Patch(cmd.Context(), "/api/v1/files/"+id, body, nil); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Script updated.")
		return nil
	},
}

var scriptDeleteCmd = &cobra.Command{
	Use:   "delete <id-or-name>",
	Short: "Delete a script",
	Args:  cobra.ExactArgs(1),
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
		id, err := resolveResource(cmd, f, "script", args[0], projectID)
		if err != nil {
			return err
		}
		if !globalFlags.Confirm {
			if !cmdutil.ConfirmAction(f, fmt.Sprintf("Delete script %s?", args[0])) {
				return fmt.Errorf("aborted")
			}
		}
		if err := client.Delete(cmd.Context(), "/api/v1/files/"+id); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Script %s deleted.\n", args[0])
		return nil
	},
}

var scriptRunCmd = &cobra.Command{
	Use:   "run <id-or-name>",
	Short: "Execute a script",
	Args:  cobra.ExactArgs(1),
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
		id, err := resolveResource(cmd, f, "script", args[0], projectID)
		if err != nil {
			return err
		}
		body := map[string]interface{}{"file_id": id}
		if cmd.Flags().Changed("timeout") {
			v, _ := cmd.Flags().GetInt("timeout")
			body["timeout_seconds"] = v
		}
		var resp api.V1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/code-runs", body, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteItem(resp.Data, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "STATUS", Field: "status"},
			{Header: "EXIT CODE", Field: "exit_code"},
			{Header: "STDOUT", Field: "stdout", Width: 80},
			{Header: "STDERR", Field: "stderr", Width: 80},
		})
	},
}

var scriptRunsCmd = &cobra.Command{
	Use:   "runs <script-id-or-name>",
	Short: "List script execution runs",
	Args:  cobra.ExactArgs(1),
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
		id, err := resolveResource(cmd, f, "script", args[0], projectID)
		if err != nil {
			return err
		}
		q := buildListQuery(cmd, url.Values{"file_id": {id}})
		var resp api.V1ListResponse
		if err := client.Get(cmd.Context(), "/api/v1/code-runs", q, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteList(resp.Data, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "STATUS", Field: "status"},
			{Header: "BACKEND", Field: "backend"},
			{Header: "EXIT CODE", Field: "exit_code"},
			{Header: "STARTED", Field: "started_at"},
			{Header: "COMPLETED", Field: "completed_at"},
		})
	},
}

var scriptRunOutputCmd = &cobra.Command{
	Use:   "run-output <execution-id>",
	Short: "Get output of a script execution",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		var resp api.V1ItemResponse
		if err := client.Get(cmd.Context(), "/api/v1/code-runs/"+args[0], nil, &resp); err != nil {
			return err
		}
		if stdout, ok := resp.Data["stdout"].(string); ok && stdout != "" {
			fmt.Fprint(cmd.OutOrStdout(), stdout)
		}
		if stderr, ok := resp.Data["stderr"].(string); ok && stderr != "" {
			fmt.Fprint(cmd.ErrOrStderr(), stderr)
		}
		return nil
	},
}

var scriptInterruptCmd = &cobra.Command{
	Use:   "interrupt <execution-id>",
	Short: "Interrupt a running script execution",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		if err := client.Post(cmd.Context(), "/api/v1/code-runs/"+args[0]+"/interrupt", nil, nil); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Interrupt signal sent.")
		return nil
	},
}

func init() {
	cmdutil.AddListFlags(scriptListCmd)

	scriptCreateCmd.Flags().String("name", "", "Script file name (must include extension)")
	scriptCreateCmd.Flags().String("content", "", "Script content")
	scriptCreateCmd.Flags().String("content-file", "", "Path to script file")
	cmdutil.AddJSONInput(scriptCreateCmd)

	scriptEditCmd.Flags().String("name", "", "New name")
	scriptEditCmd.Flags().String("content", "", "New content")
	scriptEditCmd.Flags().String("content-file", "", "Path to file with new content")
	cmdutil.AddJSONInput(scriptEditCmd)

	scriptRunCmd.Flags().Int("timeout", 0, "Timeout in seconds (1–300)")

	cmdutil.AddListFlags(scriptRunsCmd)

	scriptCmd.AddCommand(scriptListCmd)
	scriptCmd.AddCommand(scriptCreateCmd)
	scriptCmd.AddCommand(scriptGetCmd)
	scriptCmd.AddCommand(scriptEditCmd)
	scriptCmd.AddCommand(scriptDeleteCmd)
	scriptCmd.AddCommand(scriptRunCmd)
	scriptCmd.AddCommand(scriptRunsCmd)
	scriptCmd.AddCommand(scriptRunOutputCmd)
	scriptCmd.AddCommand(scriptInterruptCmd)
}
