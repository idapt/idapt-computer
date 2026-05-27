package cmd

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/input"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/spf13/cobra"
)

var computerAppCmd = &cobra.Command{
	Use:   "app",
	Short: "Manage Docker-backed Computer Apps on a computer",
	Long: `Manage Docker-backed Computer Apps on a computer.

Computer Apps run inside containers on the selected computer. The CLI talks to
the Idapt API, which dispatches the requested operation to the paired computer
daemon and keeps app state indexed in Idapt.`,
}

var computerAppRuntimeCmd = &cobra.Command{
	Use:     "status <computer-id-or-name>",
	Aliases: []string{"runtime"},
	Short:   "Show the Computer Apps runtime status",
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
		if err := client.Get(cmd.Context(), computerAppPath(id, "runtime"), nil, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteItem(resp.Data, []output.Column{
			{Header: "RUNTIME", Field: "runtime"},
			{Header: "STATUS", Field: "status"},
			{Header: "VERSION", Field: "version"},
			{Header: "COMPOSE", Field: "compose_version"},
			{Header: "AVAILABLE", Field: "available"},
			{Header: "MESSAGE", Field: "message", Width: 80},
			{Header: "REMEDIATION", Field: "remediation", Width: 80},
		})
	},
}

var computerAppSetupRuntimeCmd = &cobra.Command{
	Use:     "setup <computer-id-or-name>",
	Aliases: []string{"setup-runtime"},
	Short:   "Prepare Docker for Computer Apps on a computer",
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
		if err := client.WithTimeout(5*time.Minute).Post(cmd.Context(), computerAppPath(id, "runtime"), nil, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteItem(resp.Data, []output.Column{
			{Header: "RUNTIME", Field: "runtime"},
			{Header: "STATUS", Field: "status"},
			{Header: "CHANGED", Field: "changed"},
			{Header: "MESSAGE", Field: "message", Width: 80},
		})
	},
}

var computerAppListCmd = &cobra.Command{
	Use:   "list <computer-id-or-name>",
	Short: "List Computer Apps on a computer",
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
		var resp api.V1ListResponse
		if err := client.Get(cmd.Context(), computerAppPath(id), nil, &resp); err != nil {
			return err
		}
		return writeComputerAppList(f, resp.Data)
	},
}

var computerAppExternalCmd = &cobra.Command{
	Use:   "external <computer-id-or-name>",
	Short: "List non-Idapt Docker containers read-only",
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
		if err := client.Get(cmd.Context(), computerAppPath(id, "external"), nil, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteList(api.AsMapSlice(resp.Data["containers"]), []output.Column{
			{Header: "ID", Field: "runtime_container_id"},
			{Header: "NAME", Field: "runtime_name"},
			{Header: "IMAGE", Field: "image_ref"},
			{Header: "STATUS", Field: "status"},
			{Header: "COMPOSE_PROJECT", Field: "compose_project"},
			{Header: "COMPOSE_SERVICE", Field: "compose_service"},
		})
	},
}

var computerAppGetCmd = &cobra.Command{
	Use:   "get <computer-id-or-name> <app-id-or-slug>",
	Short: "Get Computer App details",
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
		var resp api.V1ItemResponse
		if err := client.Get(cmd.Context(), computerAppPath(id, url.PathEscape(args[1])), nil, &resp); err != nil {
			return err
		}
		return writeComputerAppItem(f, resp.Data)
	},
}

var computerAppCreateCmd = &cobra.Command{
	Use:   "create <computer-id-or-name>",
	Short: "Create a Computer App from a container image",
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
		body, err := buildComputerAppCreateBody(cmd, f)
		if err != nil {
			return err
		}
		var resp api.V1ItemResponse
		if err := client.WithTimeout(10*time.Minute).Post(cmd.Context(), computerAppPath(id), body, &resp); err != nil {
			return err
		}
		return writeComputerAppItem(f, resp.Data)
	},
}

var computerAppRunCmd = &cobra.Command{
	Use:   "run <computer-id-or-name> <path>",
	Short: "Build and run a Computer App from a Dockerfile directory",
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
		body, err := buildComputerAppRunBody(cmd, f, args[1])
		if err != nil {
			return err
		}
		var resp api.V1ItemResponse
		if err := client.WithTimeout(10*time.Minute).Post(cmd.Context(), computerAppPath(id, "run"), body, &resp); err != nil {
			return err
		}
		return writeComputerAppItem(f, resp.Data)
	},
}

var computerAppComposeUpCmd = &cobra.Command{
	Use:   "compose-up <computer-id-or-name> <compose-file>",
	Short: "Start a Compose project as a Computer App",
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
		body, err := buildComputerAppComposeBody(cmd, f, args[1])
		if err != nil {
			return err
		}
		var resp api.V1ItemResponse
		if err := client.WithTimeout(10*time.Minute).Post(cmd.Context(), computerAppPath(id, "compose-up"), body, &resp); err != nil {
			return err
		}
		return writeComputerAppItem(f, resp.Data)
	},
}

var computerAppStartCmd = computerAppLifecycleCommand("start", "Start a Computer App")
var computerAppStopCmd = computerAppLifecycleCommand("stop", "Stop a Computer App")
var computerAppRestartCmd = computerAppLifecycleCommand("restart", "Restart a Computer App")
var computerAppResetCmd = computerAppLifecycleCommand("reset", "Remove a Computer App's managed containers")

var computerAppDeleteCmd = &cobra.Command{
	Use:   "delete <computer-id-or-name> <app-id-or-slug>",
	Short: "Delete a Computer App and its managed containers",
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
		if !globalFlags.Confirm {
			if !cmdutil.ConfirmAction(f, fmt.Sprintf("Delete Computer App %s?", args[1])) {
				return fmt.Errorf("aborted")
			}
		}
		if err := client.WithTimeout(5*time.Minute).Delete(cmd.Context(), computerAppPath(id, url.PathEscape(args[1]))); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Computer App %s deleted.\n", args[1])
		return nil
	},
}

var computerAppLogsCmd = &cobra.Command{
	Use:   "logs <computer-id-or-name> <app-id-or-slug>",
	Short: "Read Computer App logs",
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
		q := url.Values{}
		lines, _ := cmd.Flags().GetInt("lines")
		q.Set("lines", strconv.Itoa(lines))
		follow, _ := cmd.Flags().GetBool("follow")
		if follow {
			q.Set("follow", "true")
		}
		if cmd.Flags().Changed("service") {
			service, _ := cmd.Flags().GetString("service")
			q.Set("service", service)
		}
		var resp api.V1ItemResponse
		path := computerAppPath(id, url.PathEscape(args[1]), "logs")
		if err := client.Get(cmd.Context(), path, q, &resp); err != nil {
			return err
		}
		if f.Format == output.FormatTable {
			if logs, ok := resp.Data["logs"].(string); ok {
				fmt.Fprint(cmd.OutOrStdout(), logs)
				return nil
			}
		}
		return f.Formatter().WriteItem(resp.Data, []output.Column{
			{Header: "LOGS", Field: "logs", Width: 120},
		})
	},
}

var computerAppExecCmd = &cobra.Command{
	Use:   "exec <computer-id-or-name> <app-id-or-slug> <command>",
	Short: "Execute a command inside a Computer App container",
	Args:  cobra.MinimumNArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		command := strings.Join(args[2:], " ")
		return runComputerAppExecRequest(cmd, args[0], args[1], command)
	},
}

var computerAppShellCmd = &cobra.Command{
	Use:   "shell <computer-id-or-name> <app-id-or-slug> [command]",
	Short: "Run a shell command inside a Computer App container",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		shell, _ := cmd.Flags().GetString("shell")
		command := shell
		if len(args) > 2 {
			command = shell + " -lc " + strconv.Quote(strings.Join(args[2:], " "))
		}
		return runComputerAppExecRequest(cmd, args[0], args[1], command)
	},
}

var computerAppPortsCmd = &cobra.Command{
	Use:   "ports <computer-id-or-name> <app-id-or-slug>",
	Short: "List detected ports for a Computer App",
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
		var resp api.V1ItemResponse
		if err := client.Get(cmd.Context(), computerAppPath(id, url.PathEscape(args[1]), "ports"), nil, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteList(api.AsMapSlice(resp.Data["ports"]), []output.Column{
			{Header: "PORT", Field: "port"},
			{Header: "PROTOCOL", Field: "protocol"},
			{Header: "SERVICE", Field: "service_name"},
			{Header: "CONTAINER", Field: "runtime_container_id"},
		})
	},
}

var computerAppExposeCmd = computerAppExposureCommand("expose", "Expose a Computer App port")
var computerAppUnexposeCmd = computerAppExposureCommand("unexpose", "Disable exposure for a Computer App port")

func init() {
	computerAppCreateCmd.Flags().String("name", "", "Computer App name")
	computerAppCreateCmd.Flags().String("image", "", "Container image reference")
	computerAppCreateCmd.Flags().String("description", "", "Computer App description")
	computerAppCreateCmd.Flags().String("command", "", "Shell command to run in the container")
	computerAppCreateCmd.Flags().StringArray("env", nil, "Environment variable as KEY=VALUE (repeatable)")
	computerAppCreateCmd.Flags().String("json", "", "Raw JSON request body or '-' for stdin")

	computerAppRunCmd.Flags().String("name", "", "Computer App name")
	computerAppRunCmd.Flags().String("dockerfile", "", "Dockerfile path relative to the build context")
	computerAppRunCmd.Flags().StringArray("env", nil, "Environment variable as KEY=VALUE (repeatable)")
	computerAppRunCmd.Flags().String("json", "", "Raw JSON request body or '-' for stdin")

	computerAppComposeUpCmd.Flags().String("name", "", "Computer App name")
	computerAppComposeUpCmd.Flags().String("project-directory", "", "Compose project directory")
	computerAppComposeUpCmd.Flags().Bool("accept-policy-warnings", false, "Accept warning-only Docker Compose policy findings")
	computerAppComposeUpCmd.Flags().StringArray("env", nil, "Environment variable as KEY=VALUE (repeatable)")
	computerAppComposeUpCmd.Flags().String("json", "", "Raw JSON request body or '-' for stdin")

	computerAppLogsCmd.Flags().Int("lines", 200, "Number of log lines to read")
	computerAppLogsCmd.Flags().Bool("follow", false, "Request follow mode when supported by the runtime")
	computerAppLogsCmd.Flags().String("service", "", "Compose service name")

	computerAppExecCmd.Flags().String("service", "", "Compose service name")
	computerAppExecCmd.Flags().Int("timeout", 60, "Timeout in seconds")
	computerAppShellCmd.Flags().String("service", "", "Compose service name")
	computerAppShellCmd.Flags().Int("timeout", 60, "Timeout in seconds")
	computerAppShellCmd.Flags().String("shell", "/bin/sh", "Shell executable inside the container")

	for _, cmd := range []*cobra.Command{computerAppExposeCmd, computerAppUnexposeCmd} {
		cmd.Flags().String("protocol", "tcp", "Port protocol (tcp, udp)")
		cmd.Flags().Bool("public", false, "Request public exposure instead of private preview")
		cmd.Flags().String("label", "", "Display label for the exposed port")
	}

	computerAppCmd.AddCommand(computerAppRuntimeCmd)
	computerAppCmd.AddCommand(computerAppSetupRuntimeCmd)
	computerAppCmd.AddCommand(computerAppListCmd)
	computerAppCmd.AddCommand(computerAppExternalCmd)
	computerAppCmd.AddCommand(computerAppGetCmd)
	computerAppCmd.AddCommand(computerAppCreateCmd)
	computerAppCmd.AddCommand(computerAppRunCmd)
	computerAppCmd.AddCommand(computerAppComposeUpCmd)
	computerAppCmd.AddCommand(computerAppStartCmd)
	computerAppCmd.AddCommand(computerAppStopCmd)
	computerAppCmd.AddCommand(computerAppRestartCmd)
	computerAppCmd.AddCommand(computerAppResetCmd)
	computerAppCmd.AddCommand(computerAppDeleteCmd)
	computerAppCmd.AddCommand(computerAppLogsCmd)
	computerAppCmd.AddCommand(computerAppExecCmd)
	computerAppCmd.AddCommand(computerAppShellCmd)
	computerAppCmd.AddCommand(computerAppPortsCmd)
	computerAppCmd.AddCommand(computerAppExposeCmd)
	computerAppCmd.AddCommand(computerAppUnexposeCmd)
}

func runComputerAppExecRequest(cmd *cobra.Command, computerArg, appArg, command string) error {
	f := cmdutil.FactoryFromCmd(cmd)
	client, err := f.APIClient()
	if err != nil {
		return err
	}
	id, err := resolveComputer(cmd, f, computerArg)
	if err != nil {
		return err
	}
	timeout, _ := cmd.Flags().GetInt("timeout")
	body := map[string]interface{}{"command": command}
	if timeout > 0 {
		body["timeout_ms"] = timeout * 1000
	}
	if cmd.Flags().Changed("service") {
		service, _ := cmd.Flags().GetString("service")
		body["service"] = service
	}
	var resp api.V1ItemResponse
	path := computerAppPath(id, url.PathEscape(appArg), "exec")
	if err := client.WithTimeout(time.Duration(max(timeout, 60))*time.Second).Post(cmd.Context(), path, body, &resp); err != nil {
		return err
	}
	if f.Format == output.FormatTable {
		if out, ok := resp.Data["stdout"].(string); ok {
			fmt.Fprint(cmd.OutOrStdout(), out)
			return nil
		}
	}
	return f.Formatter().WriteItem(resp.Data, []output.Column{
		{Header: "EXIT_CODE", Field: "exit_code"},
		{Header: "STDOUT", Field: "stdout", Width: 80},
		{Header: "STDERR", Field: "stderr", Width: 80},
		{Header: "TIMED_OUT", Field: "timed_out"},
	})
}

func computerAppPath(computerID string, parts ...string) string {
	path := "/api/v1/computers/" + url.PathEscape(computerID) + "/apps"
	for _, part := range parts {
		path += "/" + part
	}
	return path
}

func writeComputerAppList(f *cmdutil.Factory, apps []map[string]interface{}) error {
	return f.Formatter().WriteList(apps, []output.Column{
		{Header: "ID", Field: "resource_id"},
		{Header: "NAME", Field: "name"},
		{Header: "SLUG", Field: "slug"},
		{Header: "STATUS", Field: "status"},
		{Header: "KIND", Field: "kind"},
		{Header: "IMAGE", Field: "image_ref"},
		{Header: "UPDATED", Field: "updated_at"},
	})
}

func writeComputerAppItem(f *cmdutil.Factory, app map[string]interface{}) error {
	return f.Formatter().WriteItem(app, []output.Column{
		{Header: "ID", Field: "resource_id"},
		{Header: "NAME", Field: "name"},
		{Header: "SLUG", Field: "slug"},
		{Header: "STATUS", Field: "status"},
		{Header: "KIND", Field: "kind"},
		{Header: "RUNTIME", Field: "runtime"},
		{Header: "IMAGE", Field: "image_ref"},
		{Header: "SOURCE", Field: "source_path"},
		{Header: "COMPOSE", Field: "compose_path"},
		{Header: "LAST_ERROR", Field: "last_error_message", Width: 80},
	})
}

func computerAppLifecycleCommand(action, short string) *cobra.Command {
	return &cobra.Command{
		Use:   action + " <computer-id-or-name> <app-id-or-slug>",
		Short: short,
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
			var resp api.V1ItemResponse
			path := computerAppPath(id, url.PathEscape(args[1]), action)
			if err := client.WithTimeout(5*time.Minute).Post(cmd.Context(), path, nil, &resp); err != nil {
				return err
			}
			return writeComputerAppItem(f, resp.Data)
		},
	}
}

func computerAppExposureCommand(action, short string) *cobra.Command {
	return &cobra.Command{
		Use:   action + " <computer-id-or-name> <app-id-or-slug> <port>",
		Short: short,
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
			port, err := strconv.Atoi(args[2])
			if err != nil || port < 1 || port > 65535 {
				return fmt.Errorf("port must be an integer between 1 and 65535")
			}
			protocol, _ := cmd.Flags().GetString("protocol")
			body := map[string]interface{}{
				"port":     port,
				"protocol": protocol,
			}
			if action == "expose" {
				public, _ := cmd.Flags().GetBool("public")
				body["public"] = public
			}
			if cmd.Flags().Changed("label") {
				label, _ := cmd.Flags().GetString("label")
				body["label"] = label
			}
			var resp api.V1ItemResponse
			path := computerAppPath(id, url.PathEscape(args[1]), action)
			if err := client.Post(cmd.Context(), path, body, &resp); err != nil {
				return err
			}
			return f.Formatter().WriteList(api.AsMapSlice(resp.Data["ports"]), []output.Column{
				{Header: "PORT", Field: "port"},
				{Header: "PROTOCOL", Field: "protocol"},
				{Header: "STATE", Field: "exposure_state"},
				{Header: "PREVIEW_URL", Field: "preview_url", Width: 80},
				{Header: "PUBLIC_URL", Field: "public_url", Width: 80},
			})
		},
	}
}

func buildComputerAppCreateBody(cmd *cobra.Command, f *cmdutil.Factory) (map[string]interface{}, error) {
	body, err := jsonFlagBody(cmd, f)
	if err != nil {
		return nil, err
	}
	name, _ := cmd.Flags().GetString("name")
	image, _ := cmd.Flags().GetString("image")
	if name != "" {
		body["name"] = name
	}
	if image != "" {
		body["image"] = image
	}
	if cmd.Flags().Changed("description") {
		description, _ := cmd.Flags().GetString("description")
		body["description"] = description
	}
	if cmd.Flags().Changed("command") {
		command, _ := cmd.Flags().GetString("command")
		body["command"] = command
	}
	if err := addEnvFlags(cmd, body); err != nil {
		return nil, err
	}
	if body["name"] == nil || body["image"] == nil {
		return nil, fmt.Errorf("provide --name and --image")
	}
	return body, nil
}

func buildComputerAppRunBody(cmd *cobra.Command, f *cmdutil.Factory, path string) (map[string]interface{}, error) {
	body, err := jsonFlagBody(cmd, f)
	if err != nil {
		return nil, err
	}
	body["path"] = path
	if cmd.Flags().Changed("name") {
		name, _ := cmd.Flags().GetString("name")
		body["name"] = name
	}
	if cmd.Flags().Changed("dockerfile") {
		dockerfile, _ := cmd.Flags().GetString("dockerfile")
		body["dockerfile"] = dockerfile
	}
	if err := addEnvFlags(cmd, body); err != nil {
		return nil, err
	}
	return body, nil
}

func buildComputerAppComposeBody(cmd *cobra.Command, f *cmdutil.Factory, file string) (map[string]interface{}, error) {
	body, err := jsonFlagBody(cmd, f)
	if err != nil {
		return nil, err
	}
	body["file"] = file
	if cmd.Flags().Changed("name") {
		name, _ := cmd.Flags().GetString("name")
		body["name"] = name
	}
	if cmd.Flags().Changed("project-directory") {
		projectDirectory, _ := cmd.Flags().GetString("project-directory")
		body["project_directory"] = projectDirectory
	}
	if cmd.Flags().Changed("accept-policy-warnings") {
		accept, _ := cmd.Flags().GetBool("accept-policy-warnings")
		body["accept_policy_warnings"] = accept
	}
	if err := addEnvFlags(cmd, body); err != nil {
		return nil, err
	}
	return body, nil
}

func jsonFlagBody(cmd *cobra.Command, f *cmdutil.Factory) (map[string]interface{}, error) {
	if !cmd.Flags().Changed("json") {
		return map[string]interface{}{}, nil
	}
	raw, _ := cmd.Flags().GetString("json")
	parsed, err := input.ParseJSONFlag(raw, f.In)
	if err != nil {
		return nil, err
	}
	return parsed, nil
}

func addEnvFlags(cmd *cobra.Command, body map[string]interface{}) error {
	if !cmd.Flags().Changed("env") {
		return nil
	}
	values, _ := cmd.Flags().GetStringArray("env")
	env := map[string]string{}
	if existing, ok := body["env"].(map[string]interface{}); ok {
		for key, value := range existing {
			env[key] = fmt.Sprint(value)
		}
	}
	for _, item := range values {
		if item == "" || item == "[]" {
			continue
		}
		key, value, ok := strings.Cut(item, "=")
		if !ok || key == "" {
			return fmt.Errorf("--env values must use KEY=VALUE")
		}
		env[key] = value
	}
	body["env"] = env
	return nil
}
