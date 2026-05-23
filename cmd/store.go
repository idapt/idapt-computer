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
		"the `idapt store` command tree is not available for your account.\n\n" +
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

func applyStoreVisibility() {
	cachePath, _ := features.DefaultCachePath()
	apiKey := readStoreCachedAPIKey()
	hide := shouldHideStoreCommands(cachePath, apiKey)
	storeCmd.Hidden = hide
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

var storeCmd = &cobra.Command{
	Use:     "store",
	Short:   "Browse and install from the Hub store",
	PreRunE: requireHubFlag,
}

var storeSearchCmd = &cobra.Command{
	Use:     "search [query]",
	Short:   "Search the store (across skill / agent / machine / project items)",
	Args:    cobra.MaximumNArgs(1),
	PreRunE: requireHubFlag,
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		q := url.Values{}
		if len(args) > 0 {
			q.Set("q", args[0])
		}
		if cmd.Flags().Changed("type") {
			v, _ := cmd.Flags().GetString("type")
			q.Set("type", v)
		}
		if cmd.Flags().Changed("sort") {
			v, _ := cmd.Flags().GetString("sort")
			q.Set("sort", v)
		}
		q = buildListQuery(cmd, q)

		var resp api.V1ListResponse
		if err := client.Get(cmd.Context(), "/api/v1/store/search", q, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteList(resp.Data, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "TYPE", Field: "type"},
			{Header: "NAME", Field: "name"},
			{Header: "DESCRIPTION", Field: "description", Width: 60},
			{Header: "AUTHOR", Field: "authorName"},
			{Header: "INSTALLS", Field: "installCount"},
		})
	},
}

var storeInstallCmd = &cobra.Command{
	Use:     "install <resource-id>",
	Short:   "Install a store item into a project",
	Args:    cobra.ExactArgs(1),
	PreRunE: requireHubFlag,
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
		if cmd.Flags().Changed("folder-name") {
			v, _ := cmd.Flags().GetString("folder-name")
			body["folder_name"] = v
		}
		if cmd.Flags().Changed("target-parent") {
			v, _ := cmd.Flags().GetString("target-parent")
			body["target_parent_id"] = v
		}
		var resp api.V1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/store/"+args[0]+"/install", body, &resp); err != nil {
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
	cmdutil.AddListFlags(storeSearchCmd)
	storeSearchCmd.Flags().String("type", "all", "Filter type (skill | agent | machine | project | all)")
	storeSearchCmd.Flags().String("sort", "popular", "Sort order (popular | recent | updated)")

	storeInstallCmd.Flags().String("folder-name", "", "Override the installed folder name")
	storeInstallCmd.Flags().String("target-parent", "", "Parent folder resourceId for the install (defaults to project root)")

	storeCmd.AddCommand(storeSearchCmd)
	storeCmd.AddCommand(storeInstallCmd)

	origStoreHelp := storeCmd.HelpFunc()
	storeCmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		applyStoreVisibility()
		origStoreHelp(c, args)
	})
}
