package cmd

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/credential"
	"github.com/idapt/idapt-cli/internal/features"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/spf13/cobra"
)

func requireHubFlag(cmd *cobra.Command, _ []string) error {
	f := cmdutil.FactoryFromCmd(cmd)
	if f == nil {
		return nil
	}
	if f.Features().IsEnabled(features.FlagHub) {
		return nil
	}
	return fmt.Errorf(
		"the `idapt hub` command tree is not available for your account.\n\n" +
			"The Hub feature (flag `hub`) is currently off. Contact support " +
			"or your admin to request access.",
	)
}

func readStoreCachedAPIKey() string {
	if k := os.Getenv("IDAPT_API_KEY"); k != "" && !strings.HasPrefix(k, "mk_") {
		return k
	}
	path, err := credential.DefaultPath()
	if err != nil {
		return ""
	}
	creds, err := credential.Load(path)
	if err != nil {
		return ""
	}
	return creds.APIKey
}

func applyHubVisibility() {
	cachePath, _ := features.DefaultCachePath()
	apiKey := readStoreCachedAPIKey()
	hide := shouldHideStoreCommands(cachePath, apiKey)
	hubCmd.Hidden = hide
}

func shouldHideStoreCommands(cachePath, apiKey string) bool {
	if cachePath == "" {
		return true
	}
	cached := features.LoadFromCache(cachePath, apiKey)
	if cached == nil {
		return true
	}
	return !cached.IsEnabled(features.FlagHub)
}

var hubCmd = &cobra.Command{
	Use:     "hub",
	Short:   "Browse and install from the Hub store",
	PreRunE: requireHubFlag,
}

var hubSearchCmd = &cobra.Command{
	Use:     "search [query]",
	Short:   "Search the store (across skill / agent / computer / workspace items)",
	Args:    cobra.MaximumNArgs(1),
	PreRunE: requireHubFlag,
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		extra := url.Values{}
		if len(args) > 0 {
			extra.Set("q", args[0])
		}
		if cmd.Flags().Changed("type") {
			v, _ := cmd.Flags().GetString("type")
			extra.Set("type", v)
		}
		if cmd.Flags().Changed("sort") {
			v, _ := cmd.Flags().GetString("sort")
			extra.Set("sort", v)
		}

		rows, hasMore, err := f.FetchList(cmd.Context(), cmd, client, "/api/v1/hub/search", extra)
		if err != nil {
			return err
		}
		err = f.RenderList(cmd, rows, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "TYPE", Field: "type"},
			{Header: "NAME", Field: "name"},
			{Header: "DESCRIPTION", Field: "description", Width: 60},
			{Header: "AUTHOR", Field: "authorName"},
			{Header: "INSTALLS", Field: "installCount"},
		}, "No hub results found.")
		f.MaybeMoreHint(hasMore)
		return err
	},
}

var hubInstallCmd = &cobra.Command{
	Use:     "install <resource-id>",
	Short:   "Install a store item into a workspace",
	Args:    cobra.ExactArgs(1),
	PreRunE: requireHubFlag,
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
		if cmd.Flags().Changed("folder-name") {
			v, _ := cmd.Flags().GetString("folder-name")
			body["folder_name"] = v
		}
		if cmd.Flags().Changed("target-parent") {
			v, _ := cmd.Flags().GetString("target-parent")
			body["target_parent_id"] = v
		}
		var resp api.V1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/hub/"+args[0]+"/install", body, &resp); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Store item installed.")
		return f.Formatter().WriteItem(resp.Data, []output.Column{
			{Header: "INSTALLED RESOURCE ID", Field: "resourceId"},
			{Header: "FOLDER", Field: "folderName"},
			{Header: "FOLDER ID", Field: "folderId"},
		})
	},
}

func init() {
	hubSearchCmd.Flags().String("type", "all", "Filter type (skill | agent | computer | workspace | all)")
	hubSearchCmd.Flags().String("sort", "popular", "Sort order (popular | recent | updated)")
	cmdutil.AddListFlags(hubSearchCmd)
	cmdutil.AddAllFlag(hubSearchCmd)

	hubInstallCmd.Flags().String("folder-name", "", "Override the installed folder name")
	hubInstallCmd.Flags().String("target-parent", "", "Parent folder resourceId for the install (defaults to workspace root)")

	hubCmd.AddCommand(hubSearchCmd)
	hubCmd.AddCommand(hubInstallCmd)
	hubCmd.AddCommand(hubSubmitCmd)
	hubCmd.AddCommand(hubSubmissionsCmd)

	hubSubmitCmd.Flags().String("type", "", "Declared type (skill | agent | app)")
	_ = hubSubmitCmd.MarkFlagRequired("type")
	hubSubmitCmd.Flags().String("notes", "", "Optional notes for the reviewer")

	origHubHelp := hubCmd.HelpFunc()
	hubCmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		applyHubVisibility()
		origHubHelp(c, args)
	})
}

var hubSubmitCmd = &cobra.Command{
	Use:     "submit <repo-url>",
	Short:   "Submit a public GitHub repo for catalog inclusion",
	Args:    cobra.ExactArgs(1),
	PreRunE: requireHubFlag,
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		declaredType, _ := cmd.Flags().GetString("type")
		notes, _ := cmd.Flags().GetString("notes")

		body := map[string]any{
			"repoUrl":      args[0],
			"declaredType": declaredType,
		}
		if notes != "" {
			body["notes"] = notes
		}

		resp := map[string]any{}
		if err := client.Post(cmd.Context(), "/api/hub/submissions", body, &resp); err != nil {
			return err
		}

		return f.Formatter().WriteItem(resp, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "STATUS", Field: "submissionStatus"},
			{Header: "REPO", Field: "repoUrl"},
			{Header: "TYPE", Field: "declaredType"},
			{Header: "CREATED", Field: "createdAt"},
		})
	},
}

var hubSubmissionsCmd = &cobra.Command{
	Use:     "submissions",
	Short:   "List your hub submissions and their review status",
	Args:    cobra.NoArgs,
	PreRunE: requireHubFlag,
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		resp := struct {
			Items []map[string]any `json:"items"`
		}{}
		if err := client.Get(cmd.Context(), "/api/hub/submissions/mine", nil, &resp); err != nil {
			return err
		}

		return f.Formatter().WriteList(resp.Items, []output.Column{
			{Header: "STATUS", Field: "submissionStatus"},
			{Header: "TYPE", Field: "declaredType"},
			{Header: "REPO", Field: "repoUrl"},
			{Header: "CREATED", Field: "createdAt"},
		})
	},
}
