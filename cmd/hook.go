package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/spf13/cobra"
)
var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Manage hooks (condition->inject pairs that fire each chat turn)",
	Long: `Hooks generalise the closed set of system reminders into per-agent /
per-workspace condition->inject pairs evaluated each chat turn. Builtins
are platform-managed and can be toggled or overridden per-agent; user
hooks are created with explicit condition + effect specs.`,
	Annotations: map[string]string{
		"instructions": `# hook — instructions

Hooks fire on every chat turn. A poorly-tuned hook taxes every response
the agent makes, sometimes invisibly. Read this before ` + "`create`" + ` /
` + "`update`" + ` / ` + "`override`" + ` / ` + "`delete`" + `.

## When to add a hook

- The user asks for a recurring instruction ("remind me to write tests
  when I edit code"). Use ` + "`message-matches`" + ` or ` + "`tool-called`" + `.
- A workflow has a checkpoint every N turns. Use ` + "`every-n-turns`" + `.
- A piece of state matters once at chat start. Use ` + "`first-turn`" + `.

## When NOT to add a hook

- A one-off note. Just say it once in the conversation — don't burn
  per-turn budget on a single occurrence.
- Anything that could be a system prompt edit (` + "`agent.systemPrompt`" + `)
  instead. The system prompt is cached and cheaper per turn than a
  per-turn injection.

## Priority and budget

- User hooks default to priority 1000; builtins occupy 10..140. Lower
  priority = earlier in the block.
- The per-turn budget is 8 KB by default. Over-budget hooks are
  elided lowest-priority first (audit recorded in
  ` + "`agent_run.hooksFired`" + `).

## Toggle vs delete vs override

- ` + "`toggle`" + ` flips the per-agent on/off for a builtin/workspace
  hook. Reversible. Other agents are unaffected.
- ` + "`override`" + ` rewrites the template for ONE agent. Use when a
  builtin is mostly right but you want to tweak phrasing.
- ` + "`delete`" + ` is for user hooks you authored. Builtins can't be
  deleted. Prefer ` + "`toggle`" + ` for "stop this one for now".`,
	},
}

var hookListCmd = &cobra.Command{
	Use:   "list",
	Short: "List effective hooks for an agent",
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		agentID, _ := cmd.Flags().GetString("agent-id")
		if agentID == "" {
			return fmt.Errorf("--agent-id is required")
		}

		q := url.Values{}
		q.Set("agent_id", agentID)

		var resp struct {
			Hooks []map[string]interface{} `json:"hooks"`
		}
		if err := client.Get(cmd.Context(), "/api/hooks", q, &resp); err != nil {
			return err
		}

		formatter := f.Formatter()
		return formatter.WriteList(resp.Hooks, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "NAME", Field: "name"},
			{Header: "SCOPE", Field: "scope"},
			{Header: "ORIGIN", Field: "origin"},
			{Header: "ENABLED", Field: "enabled"},
			{Header: "PRIORITY", Field: "priority"},
		})
	},
}

var hookGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get a hook record by id",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		var resp struct {
			Hook map[string]interface{} `json:"hook"`
		}
		if err := client.Get(cmd.Context(), "/api/hooks/"+url.PathEscape(args[0]), nil, &resp); err != nil {
			return err
		}
		formatter := f.Formatter()
		return formatter.WriteItem(resp.Hook, []output.Column{{Header: "ID", Field: "id"}, {Header: "NAME", Field: "name"}, {Header: "SCOPE", Field: "scope"}, {Header: "PRIORITY", Field: "priority"}, {Header: "ENABLED", Field: "enabled"}})
	},
}

var hookCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a user hook (--name, --condition, --effect, --scope)",
	Long: `Create a new user hook. The condition and effect specs are JSON
strings matching the schemas in lib/hooks/validation/schemas.ts. Example:

  idapt hook create \
    --name "Remind about tests" \
    --scope agent \
    --agent-id <uuid> \
    --condition '{"kind":"message-matches","mode":"keyword","pattern":"refactor,edit"}' \
    --effect '{"placement":"prepend-user","template":"Remember to run tests for {agent.name}"}'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		name, _ := cmd.Flags().GetString("name")
		scope, _ := cmd.Flags().GetString("scope")
		agentID, _ := cmd.Flags().GetString("agent-id")
		workspaceID, _ := cmd.Flags().GetString("workspace-id")
		description, _ := cmd.Flags().GetString("description")
		conditionStr, _ := cmd.Flags().GetString("condition")
		effectStr, _ := cmd.Flags().GetString("effect")
		priority, _ := cmd.Flags().GetInt("priority")
		trialEligible, _ := cmd.Flags().GetBool("trial-eligible")

		if name == "" || scope == "" || conditionStr == "" || effectStr == "" {
			return fmt.Errorf("--name, --scope, --condition, --effect are required")
		}
		var condition, effect map[string]interface{}
		if err := json.Unmarshal([]byte(conditionStr), &condition); err != nil {
			return fmt.Errorf("invalid --condition JSON: %w", err)
		}
		if err := json.Unmarshal([]byte(effectStr), &effect); err != nil {
			return fmt.Errorf("invalid --effect JSON: %w", err)
		}

		body := map[string]interface{}{
			"scope":          scope,
			"name":           name,
			"condition":      condition,
			"effect":         effect,
			"trialEligible": trialEligible,
		}
		if description != "" {
			body["description"] = description
		}
		if agentID != "" {
			body["agentId"] = agentID
		}
		if workspaceID != "" {
			body["workspaceId"] = workspaceID
		}
		if priority > 0 {
			body["priority"] = priority
		}

		bodyBytes, _ := json.Marshal(body)
		var resp struct {
			Hook map[string]interface{} `json:"hook"`
		}
		if err := client.Post(cmd.Context(), "/api/hooks", bytes.NewReader(bodyBytes), &resp); err != nil {
			return err
		}
		formatter := f.Formatter()
		return formatter.WriteItem(resp.Hook, []output.Column{{Header: "ID", Field: "id"}, {Header: "NAME", Field: "name"}, {Header: "SCOPE", Field: "scope"}, {Header: "PRIORITY", Field: "priority"}, {Header: "ENABLED", Field: "enabled"}})
	},
}

var hookUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update a user hook (builtins are immutable; use override or toggle)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		body := map[string]interface{}{}
		if v, _ := cmd.Flags().GetString("name"); v != "" {
			body["name"] = v
		}
		if v, _ := cmd.Flags().GetString("description"); v != "" {
			body["description"] = v
		}
		if v, _ := cmd.Flags().GetInt("priority"); cmd.Flag("priority").Changed {
			body["priority"] = v
		}
		if cmd.Flag("enabled").Changed {
			v, _ := cmd.Flags().GetBool("enabled")
			body["enabled"] = v
		}
		bodyBytes, _ := json.Marshal(body)
		var resp struct {
			Hook map[string]interface{} `json:"hook"`
		}
		if err := client.Patch(cmd.Context(), "/api/hooks/"+url.PathEscape(args[0]), bytes.NewReader(bodyBytes), &resp); err != nil {
			return err
		}
		formatter := f.Formatter()
		return formatter.WriteItem(resp.Hook, []output.Column{{Header: "ID", Field: "id"}, {Header: "NAME", Field: "name"}, {Header: "SCOPE", Field: "scope"}, {Header: "PRIORITY", Field: "priority"}, {Header: "ENABLED", Field: "enabled"}})
	},
}

var hookToggleCmd = &cobra.Command{
	Use:   "toggle <id>",
	Short: "Enable/disable a builtin or workspace hook for a specific agent",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		agentID, _ := cmd.Flags().GetString("agent-id")
		if agentID == "" {
			return fmt.Errorf("--agent-id is required")
		}
		enabledStr, _ := cmd.Flags().GetString("enabled")
		enabled := strings.EqualFold(enabledStr, "true")

		body := map[string]interface{}{
			"agentId": agentID,
			"enabled":  enabled,
		}
		bodyBytes, _ := json.Marshal(body)
		var resp struct {
			State map[string]interface{} `json:"state"`
		}
		if err := client.Post(cmd.Context(), "/api/hooks/"+url.PathEscape(args[0])+"/toggle", bytes.NewReader(bodyBytes), &resp); err != nil {
			return err
		}
		formatter := f.Formatter()
		return formatter.WriteItem(resp.State, []output.Column{{Header: "HOOK_ID", Field: "hookId"}, {Header: "AGENT_ID", Field: "agentId"}, {Header: "ENABLED", Field: "enabled"}})
	},
}

var hookOverrideCmd = &cobra.Command{
	Use:   "override <id>",
	Short: "Set or clear a per-agent template override (template=null clears)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		agentID, _ := cmd.Flags().GetString("agent-id")
		if agentID == "" {
			return fmt.Errorf("--agent-id is required")
		}
		clear, _ := cmd.Flags().GetBool("clear")
		template, _ := cmd.Flags().GetString("template")

		var tplValue interface{} = template
		if clear {
			tplValue = nil
		}

		body := map[string]interface{}{
			"agentId": agentID,
			"template": tplValue,
		}
		bodyBytes, _ := json.Marshal(body)
		var resp struct {
			State map[string]interface{} `json:"state"`
		}
		if err := client.Post(cmd.Context(), "/api/hooks/"+url.PathEscape(args[0])+"/override", bytes.NewReader(bodyBytes), &resp); err != nil {
			return err
		}
		formatter := f.Formatter()
		return formatter.WriteItem(resp.State, []output.Column{{Header: "HOOK_ID", Field: "hookId"}, {Header: "AGENT_ID", Field: "agentId"}, {Header: "ENABLED", Field: "enabled"}})
	},
}

var hookDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a user hook (builtins cannot be deleted)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		if err := client.Delete(cmd.Context(), "/api/hooks/"+url.PathEscape(args[0])); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "deleted")
		return nil
	},
}

func init() {
	hookListCmd.Flags().String("agent-id", "", "Agent UUID (required)")

	hookCreateCmd.Flags().String("name", "", "Hook name (1-128 chars)")
	hookCreateCmd.Flags().String("scope", "", "agent | workspace")
	hookCreateCmd.Flags().String("agent-id", "", "Required when scope=agent")
	hookCreateCmd.Flags().String("workspace-id", "", "Required when scope=workspace")
	hookCreateCmd.Flags().String("description", "", "Optional description")
	hookCreateCmd.Flags().String("condition", "", "ConditionSpec as JSON")
	hookCreateCmd.Flags().String("effect", "", "EffectSpec as JSON")
	hookCreateCmd.Flags().Int("priority", 0, "Ordering (0..100000)")
	hookCreateCmd.Flags().Bool("trial-eligible", false, "Fire in trial-mode sessions")

	hookUpdateCmd.Flags().String("name", "", "New name")
	hookUpdateCmd.Flags().String("description", "", "New description")
	hookUpdateCmd.Flags().Int("priority", 0, "New priority")
	hookUpdateCmd.Flags().Bool("enabled", true, "Global on/off (agent-scoped only)")

	hookToggleCmd.Flags().String("agent-id", "", "Agent UUID (required)")
	hookToggleCmd.Flags().String("enabled", "true", "true|false")

	hookOverrideCmd.Flags().String("agent-id", "", "Agent UUID (required)")
	hookOverrideCmd.Flags().String("template", "", "New template text")
	hookOverrideCmd.Flags().Bool("clear", false, "Clear the override (revert to default)")

	hookCmd.AddCommand(hookListCmd)
	hookCmd.AddCommand(hookGetCmd)
	hookCmd.AddCommand(hookCreateCmd)
	hookCmd.AddCommand(hookUpdateCmd)
	hookCmd.AddCommand(hookToggleCmd)
	hookCmd.AddCommand(hookOverrideCmd)
	hookCmd.AddCommand(hookDeleteCmd)
}
