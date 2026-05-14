package cmd

import (
	"fmt"
	"net/url"
	"os"
	"strings"

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

var storeCmd = &cobra.Command{
	Use:     "store",
	Short:   "Browse and install from the store",
	PreRunE: requireHubFlag,
}

var storeSearchCmd = &cobra.Command{
	Use:     "search <query>",
	Short:   "Search the store for all resource types",
	Args:    cobra.ExactArgs(1),
	PreRunE: requireHubFlag,
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		q := url.Values{"q": {args[0]}}
		if cmd.Flags().Changed("type") {
			v, _ := cmd.Flags().GetString("type")
			q.Set("type", v)
		}

		var resp struct {
			Results []map[string]interface{} `json:"results"`
		}
		if err := client.Get(cmd.Context(), "/api/explore/search", q, &resp); err != nil {
			return err
		}

		formatter := f.Formatter()
		return formatter.WriteList(resp.Results, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "TYPE", Field: "type"},
			{Header: "NAME", Field: "name"},
			{Header: "DESCRIPTION", Field: "description", Width: 60},
			{Header: "AUTHOR", Field: "authorName"},
		})
	},
}
var storeSkillCmd = &cobra.Command{
	Use:     "skill",
	Short:   "Browse and install skills from the store",
	PreRunE: requireHubFlag,
}

var storeSkillSearchCmd = &cobra.Command{
	Use:     "search <query>",
	Short:   "Search skills in the store",
	Args:    cobra.ExactArgs(1),
	PreRunE: requireHubFlag,
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		q := url.Values{"q": {args[0]}}
		q = buildListQuery(cmd, q)

		var resp struct {
			Items []map[string]interface{} `json:"items"`
		}
		if err := client.Get(cmd.Context(), "/api/skill-store", q, &resp); err != nil {
			return err
		}

		formatter := f.Formatter()
		return formatter.WriteList(resp.Items, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "NAME", Field: "name"},
			{Header: "DESCRIPTION", Field: "description", Width: 60},
			{Header: "INSTALLS", Field: "installCount"},
		})
	},
}

var storeSkillInstallCmd = &cobra.Command{
	Use:     "install <resource-id>",
	Short:   "Install a skill from the store",
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

		body := map[string]interface{}{"projectId": projectID}
		var resp map[string]interface{}
		if err := client.Post(cmd.Context(), "/api/skill-store/"+args[0]+"/install", body, &resp); err != nil {
			return err
		}

		fmt.Fprintln(cmd.OutOrStdout(), "Skill installed.")
		return nil
	},
}
var storeScriptCmd = &cobra.Command{
	Use:     "script",
	Short:   "Browse and install scripts from the store",
	PreRunE: requireHubFlag,
}

var storeScriptSearchCmd = &cobra.Command{
	Use:     "search <query>",
	Short:   "Search scripts in the store",
	Args:    cobra.ExactArgs(1),
	PreRunE: requireHubFlag,
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		q := url.Values{"q": {args[0]}}
		q = buildListQuery(cmd, q)

		var resp struct {
			Items []map[string]interface{} `json:"items"`
		}
		if err := client.Get(cmd.Context(), "/api/script-store", q, &resp); err != nil {
			return err
		}

		formatter := f.Formatter()
		return formatter.WriteList(resp.Items, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "NAME", Field: "name"},
			{Header: "DESCRIPTION", Field: "description", Width: 60},
			{Header: "INSTALLS", Field: "installCount"},
		})
	},
}

var storeScriptInstallCmd = &cobra.Command{
	Use:     "install <resource-id>",
	Short:   "Install a script from the store",
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

		body := map[string]interface{}{"projectId": projectID}
		var resp map[string]interface{}
		if err := client.Post(cmd.Context(), "/api/script-store/"+args[0]+"/install", body, &resp); err != nil {
			return err
		}

		fmt.Fprintln(cmd.OutOrStdout(), "Script installed.")
		return nil
	},
}
var storeAgentCmd = &cobra.Command{
	Use:     "agent",
	Short:   "Browse and install agents from the store",
	PreRunE: requireHubFlag,
}

var storeAgentSearchCmd = &cobra.Command{
	Use:     "search <query>",
	Short:   "Search agents in the store",
	Args:    cobra.ExactArgs(1),
	PreRunE: requireHubFlag,
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		q := url.Values{"q": {args[0]}}
		q = buildListQuery(cmd, q)

		var resp struct {
			Items []map[string]interface{} `json:"items"`
		}
		if err := client.Get(cmd.Context(), "/api/agent-store", q, &resp); err != nil {
			return err
		}

		formatter := f.Formatter()
		return formatter.WriteList(resp.Items, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "NAME", Field: "name"},
			{Header: "DESCRIPTION", Field: "description", Width: 60},
			{Header: "INSTALLS", Field: "installCount"},
		})
	},
}

var storeAgentInstallCmd = &cobra.Command{
	Use:     "install <resource-id>",
	Short:   "Install an agent from the store",
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

		body := map[string]interface{}{"projectId": projectID}
		var resp map[string]interface{}
		if err := client.Post(cmd.Context(), "/api/agent-store/"+args[0]+"/install", body, &resp); err != nil {
			return err
		}

		fmt.Fprintln(cmd.OutOrStdout(), "Agent installed.")
		return nil
	},
}

func init() {
	storeSearchCmd.Flags().String("type", "", "Filter by resource type (skill, script, agent)")

	cmdutil.AddListFlags(storeSkillSearchCmd)
	cmdutil.AddListFlags(storeScriptSearchCmd)
	cmdutil.AddListFlags(storeAgentSearchCmd)

	storeSkillCmd.AddCommand(storeSkillSearchCmd)
	storeSkillCmd.AddCommand(storeSkillInstallCmd)

	storeScriptCmd.AddCommand(storeScriptSearchCmd)
	storeScriptCmd.AddCommand(storeScriptInstallCmd)

	storeAgentCmd.AddCommand(storeAgentSearchCmd)
	storeAgentCmd.AddCommand(storeAgentInstallCmd)

	storeCmd.AddCommand(storeSearchCmd)
	storeCmd.AddCommand(storeSkillCmd)
	storeCmd.AddCommand(storeScriptCmd)
	storeCmd.AddCommand(storeAgentCmd)

	origRootHelp := rootCmd.HelpFunc()
	rootCmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		applyStoreVisibility()
		origRootHelp(c, args)
	})
}
