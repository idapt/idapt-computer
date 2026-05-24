package cmd

import (
	"fmt"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/input"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/spf13/cobra"
)

var computerUserCmd = &cobra.Command{
	Use:   "user",
	Short: "Manage computer users",
}

var computerUserListCmd = &cobra.Command{
	Use:   "list <computer-id-or-name>",
	Short: "List users on a computer",
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
		if err := client.Get(cmd.Context(), "/api/v1/computers/"+id+"/users", nil, &resp); err != nil {
			return err
		}

		formatter := f.Formatter()
		return formatter.WriteList(resp.Data, []output.Column{
			{Header: "USERNAME", Field: "username"},
			{Header: "SHELL", Field: "shell"},
			{Header: "HOME", Field: "home"},
			{Header: "SUDO", Field: "sudo"},
		})
	},
}

var computerUserCreateCmd = &cobra.Command{
	Use:   "create <computer-id-or-name>",
	Short: "Create a user on a computer",
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

		overrides := map[string]interface{}{}
		if cmd.Flags().Changed("username") {
			v, _ := cmd.Flags().GetString("username")
			overrides["username"] = v
		}
		if cmd.Flags().Changed("password") {
			v, _ := cmd.Flags().GetString("password")
			overrides["password"] = v
		}
		if cmd.Flags().Changed("shell") {
			v, _ := cmd.Flags().GetString("shell")
			overrides["shell"] = v
		}
		if cmd.Flags().Changed("sudo") {
			v, _ := cmd.Flags().GetBool("sudo")
			overrides["sudo"] = v
		}
		body = input.MergeFlags(body, overrides)

		var resp api.V1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/computers/"+id+"/users", body, &resp); err != nil {
			return err
		}

		fmt.Fprintln(cmd.OutOrStdout(), "User created.")
		return f.Formatter().WriteItem(resp.Data, []output.Column{
			{Header: "USERNAME", Field: "username"},
			{Header: "SHELL", Field: "shell"},
			{Header: "HOME", Field: "home"},
			{Header: "SUDO", Field: "sudo"},
		})
	},
}

var computerUserDeleteCmd = &cobra.Command{
	Use:   "delete <computer-id-or-name> <username>",
	Short: "Delete a user from a computer",
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
			if !cmdutil.ConfirmAction(f, fmt.Sprintf("Delete user %s?", args[1])) {
				return fmt.Errorf("aborted")
			}
		}

		if err := client.Delete(cmd.Context(), "/api/v1/computers/"+id+"/users/"+args[1]); err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "User %s deleted.\n", args[1])
		return nil
	},
}

var computerUserEditCmd = &cobra.Command{
	Use:   "edit <computer-id-or-name> <username>",
	Short: "Edit a user on a computer",
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

		body := map[string]interface{}{}
		if cmd.Flags().Changed("password") {
			v, _ := cmd.Flags().GetString("password")
			body["password"] = v
		}
		if cmd.Flags().Changed("shell") {
			v, _ := cmd.Flags().GetString("shell")
			body["shell"] = v
		}
		if cmd.Flags().Changed("sudo") {
			v, _ := cmd.Flags().GetBool("sudo")
			body["sudo"] = v
		}

		var resp api.V1ItemResponse
		if err := client.Patch(cmd.Context(), "/api/v1/computers/"+id+"/users/"+args[1], body, &resp); err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "User %s updated.\n", args[1])
		return f.Formatter().WriteItem(resp.Data, []output.Column{
			{Header: "USERNAME", Field: "username"},
			{Header: "SHELL", Field: "shell"},
			{Header: "HOME", Field: "home"},
			{Header: "SUDO", Field: "sudo"},
		})
	},
}

func init() {
	computerUserCreateCmd.Flags().String("username", "", "Username")
	computerUserCreateCmd.Flags().String("password", "", "Password")
	computerUserCreateCmd.Flags().String("shell", "/bin/bash", "Login shell")
	computerUserCreateCmd.Flags().Bool("sudo", false, "Grant sudo access")
	cmdutil.AddJSONInput(computerUserCreateCmd)

	computerUserEditCmd.Flags().String("password", "", "New password")
	computerUserEditCmd.Flags().String("shell", "", "Login shell")
	computerUserEditCmd.Flags().Bool("sudo", false, "Grant sudo access")

	computerUserCmd.AddCommand(computerUserListCmd)
	computerUserCmd.AddCommand(computerUserCreateCmd)
	computerUserCmd.AddCommand(computerUserDeleteCmd)
	computerUserCmd.AddCommand(computerUserEditCmd)
}
