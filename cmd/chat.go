package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/input"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/spf13/cobra"
)

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Manage chats",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	},
	Annotations: map[string]string{
		"instructions": chatInstructions,
	},
}

const chatInstructions = `# chat — instructions

Every chat is a tree of messages, not a list. The CLI surface gives you
three ways to add to it:

  - ` + "`chat send <chat>`" + ` — append a NEW user turn at the active
    leaf, then the agent responds. Use this 99% of the time.
  - ` + "`chat send --branch-from <msg-id>`" + ` — branch from an
    earlier point. New user turn becomes a sibling of whatever was
    there. Old branch is preserved.
  - ` + "`chat reprompt <chat> --message <user-msg-id>`" + ` — regenerate
    the assistant's reply to an existing user message. Creates a sibling
    assistant message (e.g. to try a different model). Original is NOT
    overwritten.

## Auto model + effort speed-dial

The Auto model (` + "`--model auto`" + `) picks a concrete model per
message based on ` + "`--effort-level`" + `:

  fast    → Gemma-4-31b-it (free tier). Lowest latency, no reasoning.
  smart   → MiniMax M2.7 (mid-tier). Default tier.
  expert  → GPT-5.5 (frontier). Best quality, highest latency/cost.

` + "`--effort-level`" + ` controls BOTH the auto-router tier AND the
behavioural system prompt for concrete models. Set effort_level on a
non-auto model and the behavioural reminder still applies (e.g. "be
concise" for fast).

` + "`--reasoning-level`" + ` and ` + "`--cost-level`" + ` are the wire
decoupling for power users. The CLI doesn't expose them — pass them via
` + "`--json`" + ` if you need to.

## Branching: send vs reprompt vs --branch-from

  - Run the SAME prompt against another model: ` + "`reprompt --model X`" + `
  - Edit a user message and continue: ` + "`send --branch-from <prev>`" + `
  - Continue from an earlier turn: ` + "`send --branch-from <leaf>`" + `

Reprompt is the right tool for model bake-offs. Send-with-branch-from
is the right tool for "rewind and try again".

## Cleanup ladder — prefer reversible

  archive    Hides from default sidebar. FULLY reversible via unarchive.
             Use this for "out of sight" without committing to deletion.
  delete     Moves to trash. Reversible via ` + "`restore`" + `. Messages,
             runs, costs, shares all preserved.
  permanent-delete  IRREVERSIBLE. Only operates on already-trashed chats.
             Hard-drops messages, runs, costs, shares. No undo. No tombstone.

Default to archive. Reach for delete when you actually intend to remove
the chat from the active list. Reach for permanent-delete only when you
truly need the data gone (privacy, storage caps, post-test cleanup).

## Cost budget

Every chat carries a per-chat USD cap (` + "`costBudgetLimitUsd`" + `,
default $5). When the budget is exhausted, sends return rate_limit.
Inspect with ` + "`chat cost <chat>`" + `. Raise it with ` + "`chat edit`" + `
+ ` + "`--json '{\"costBudgetLimitUsd\": 25}'`" + ` (the public schema
doesn't expose this directly yet — escape hatch via --json).

## Stop a running generation

` + "`chat stop <chat>`" + ` sets ` + "`stopRequestedAt`" + ` on the
chat; the worker drains gracefully and the partial assistant message is
preserved. Don't repeatedly retry — give the worker a few seconds.

## Tier and feature-flag gating

Chat commands pass through the same access checks as the in-app
dispatcher — tier checks for paid models (` + "`validateModelAccess`" + `),
plan-mode tool restrictions, workspace membership, and any other
feature-flag gates apply transparently at the server layer. If a send
fails with ` + "`forbidden: Model access denied`" + ` you've hit a
subscription gate; if ` + "`forbidden: Sign in to use reprompt`" + ` you're
running anonymously and need to authenticate first.

The CLI itself doesn't add extra gates on top — every chat verb works
for any account that has the right subscription and workspace access.
`
var chatListCmd = &cobra.Command{
	Use:   "list",
	Short: "List chats",
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		q := url.Values{}
		if cmd.Flags().Changed("agent") {
			v, _ := cmd.Flags().GetString("agent")
			q.Set("agent_id", v)
		}
		if cmd.Flags().Changed("workspace") || globalFlags.Workspace != "" {
			workspaceID, err := resolveWorkspaceFlag(cmd, f)
			if err != nil {
				return err
			}
			q.Set("workspace_id", workspaceID)
		}
		q = buildListQuery(cmd, q)

		var resp v1ListResponse
		if err := client.Get(cmd.Context(), "/api/v1/chats", q, &resp); err != nil {
			return err
		}

		return f.Formatter().WriteList(resp.Data, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "TITLE", Field: "title", Width: 50},
			{Header: "AGENT_ID", Field: "agent_id"},
			{Header: "MODEL", Field: "default_model"},
			{Header: "CREATED", Field: "created_at"},
		})
	},
}
var chatCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new chat",
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
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
		if cmd.Flags().Changed("agent") {
			v, _ := cmd.Flags().GetString("agent")
			overrides["agent_id"] = v
		}
		if cmd.Flags().Changed("workspace") || globalFlags.Workspace != "" {
			workspaceID, err := resolveWorkspaceFlag(cmd, f)
			if err != nil {
				return err
			}
			overrides["workspace_id"] = workspaceID
		}
		if cmd.Flags().Changed("title") {
			v, _ := cmd.Flags().GetString("title")
			overrides["title"] = v
		}
		if cmd.Flags().Changed("model") {
			v, _ := cmd.Flags().GetString("model")
			overrides["default_model"] = v
		}
		body = input.MergeFlags(body, overrides)

		var resp v1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/chats", body, &resp); err != nil {
			return err
		}
		return writeChatItem(f, resp.Data)
	},
}
var chatGetCmd = &cobra.Command{
	Use:               "get <id>",
	Short:             "Get chat details",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeChatIDs,
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		var resp v1ItemResponse
		if err := client.Get(cmd.Context(), "/api/v1/chats/"+args[0], nil, &resp); err != nil {
			return err
		}
		return writeChatItem(f, resp.Data)
	},
}
var chatEditCmd = &cobra.Command{
	Use:               "edit <id>",
	Short:             "Edit a chat's title, icon, or default model",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeChatIDs,
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
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
		if cmd.Flags().Changed("title") {
			v, _ := cmd.Flags().GetString("title")
			overrides["title"] = v
		}
		if cmd.Flags().Changed("model") {
			v, _ := cmd.Flags().GetString("model")
			if v != "" {
				overrides["default_model"] = v
			}
		}
		body = input.MergeFlags(body, overrides)
		if cmd.Flags().Changed("model") {
			if v, _ := cmd.Flags().GetString("model"); v == "" {
				body["default_model"] = nil
			}
		}

		var resp v1ItemResponse
		if err := client.Patch(cmd.Context(), "/api/v1/chats/"+args[0], body, &resp); err != nil {
			return err
		}
		return writeChatItem(f, resp.Data)
	},
}
var chatDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Move chat to trash (reversible via 'chat restore')",
	Long: `Move a chat to the trash. This is REVERSIBLE — restore with ` + "`idapt chat restore <id>`" + `.

Prefer ` + "`chat archive`" + ` if you just want the chat out of the sidebar
without committing to deletion. Reach for ` + "`chat permanent-delete`" + ` only
after this command if you need the data hard-deleted.

See ` + "`idapt instructions chat`" + ` for the full cleanup ladder.`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeChatIDs,
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		if !globalFlags.Confirm {
			if !cmdutil.ConfirmAction(f, fmt.Sprintf("Move chat %s to trash? (reversible)", args[0])) {
				return fmt.Errorf("aborted")
			}
		}

		if err := client.Delete(cmd.Context(), "/api/v1/chats/"+args[0]); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Chat %s moved to trash.\n", args[0])
		return nil
	},
}
var chatSendCmd = &cobra.Command{
	Use:               "send <chat-id> <message>",
	Short:             "Send a message and wait for the AI response",
	ValidArgsFunction: completeChatIDs,
	Long: `Send a message to a chat. By default waits for the response and prints it.

Flags:
  --model <id>                   Override model for this message ("auto", "openai/gpt-5.5", ...).
  --effort-level fast|smart|expert
                                 Effort mode + Auto-router tier in one knob.
  --branch-from <message-id>     Append after this message instead of the active leaf.
                                 Use to branch the conversation at an earlier point.
  --no-wait                      Don't wait for the response. Returns the pending id.
  --timeout <seconds>            How long to wait for completion (default 120, max 300).`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		chatID := args[0]
		message := strings.Join(args[1:], " ")

		streamFlag := false
		if cmd.Flags().Lookup("stream") != nil {
			streamFlag, _ = cmd.Flags().GetBool("stream")
		}
		if streamFlag {
			if noWait, _ := cmd.Flags().GetBool("no-wait"); noWait {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: --no-wait ignored when --stream is set")
			}
			model, _ := cmd.Flags().GetString("model")
			effort, _ := cmd.Flags().GetString("effort-level")
			if effort != "" && !validEffortLevel(effort) {
				return fmt.Errorf("invalid --effort-level %q", effort)
			}
			branch, _ := cmd.Flags().GetString("branch-from")
			req := ChatRunRequest{
				ChatID:      chatID,
				Text:        message,
				ModelID:     model,
				EffortLevel: effort,
				BranchFrom:  branch,
				StreamMode:  true,
				JSONMode:    f.Format == "json" || f.Format == "jsonl",
				NoColor:     f.NoColor,
				Out:         cmd.OutOrStdout(),
			}
			_, err := RunChat(cmd.Context(), client, req)
			return err
		}

		body := map[string]interface{}{
			"content": message,
		}
		if v, _ := cmd.Flags().GetString("model"); v != "" {
			body["model_id"] = v
		}
		if v, _ := cmd.Flags().GetString("effort-level"); v != "" {
			if !validEffortLevel(v) {
				return fmt.Errorf("invalid --effort-level %q (must be fast, smart, or expert)", v)
			}
			body["effort_level"] = v
		}
		if v, _ := cmd.Flags().GetString("branch-from"); v != "" {
			body["branch_from"] = v
		}

		noWait, _ := cmd.Flags().GetBool("no-wait")
		if noWait {
			body["wait"] = false
		}
		if cmd.Flags().Changed("timeout") {
			v, _ := cmd.Flags().GetInt("timeout")
			body["timeout"] = v
		}

		var resp v1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/chats/"+chatID+"/messages", body, &resp); err != nil {
			return err
		}

		switch f.Format {
		case output.FormatJSON, output.FormatJSONL, output.FormatQuiet:
			return f.Formatter().WriteItem(resp.Data, []output.Column{
				{Header: "ID", Field: "id"},
			})
		}

		out := cmd.OutOrStdout()
		status, _ := resp.Data["status"].(string)
		if status == "pending" {
			pendingToken, _ := resp.Data["pending_token"].(string)
			fmt.Fprintf(out, "Queued (pending %s). Use `idapt chat messages %s --last 1` to poll.\n", pendingToken, chatID)
			return nil
		}

		message_, _ := resp.Data["message"].(map[string]interface{})
		if message_ == nil {
			fmt.Fprintln(out, "Completed but produced no assistant response.")
			fmt.Fprintf(out, "Inspect: `idapt chat runs %s --state failed`\n", chatID)
			return nil
		}

		if modelID, _ := resp.Data["model_id"].(string); modelID != "" {
			fmt.Fprintf(out, "[Model: %s]\n", modelID)
		}
		if content, _ := message_["content"].(string); content != "" {
			fmt.Fprintln(out, content)
		}
		return nil
	},
}
var chatRepromptCmd = &cobra.Command{
	Use:               "reprompt <chat-id>",
	ValidArgsFunction: completeChatIDs,
	Short: "Regenerate the assistant's reply to a user message (creates a sibling)",
	Long: `Regenerate the assistant response to an existing user message. The new
response is a SIBLING of the original — the old reply is not overwritten.

Useful for model bake-offs: ` + "`chat reprompt <chat> --message <user-msg> --model claude-opus-4.7`" + `

Flags:
  --message <user-msg-id>   The user message to regenerate against (required).
  --model <id>              Override model for the regeneration ("auto", concrete, ...).
  --effort-level fast|smart|expert
                            Effort mode + Auto-router tier.
  --no-wait                 Don't wait for completion. Returns immediately.
  --timeout <seconds>       How long to wait for completion (default 120, max 300).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		chatID := args[0]
		messageID, _ := cmd.Flags().GetString("message")
		if messageID == "" {
			return fmt.Errorf("--message <user-message-id> is required")
		}

		body := map[string]interface{}{}
		if v, _ := cmd.Flags().GetString("model"); v != "" {
			body["model_id"] = v
		}
		if v, _ := cmd.Flags().GetString("effort-level"); v != "" {
			if !validEffortLevel(v) {
				return fmt.Errorf("invalid --effort-level %q (must be fast, smart, or expert)", v)
			}
			body["effort_level"] = v
		}
		if noWait, _ := cmd.Flags().GetBool("no-wait"); noWait {
			body["wait"] = false
		}
		if cmd.Flags().Changed("timeout") {
			v, _ := cmd.Flags().GetInt("timeout")
			body["timeout"] = v
		}

		path := fmt.Sprintf("/api/v1/chats/%s/messages/%s/reprompt", chatID, messageID)
		var resp v1ItemResponse
		if err := client.Post(cmd.Context(), path, body, &resp); err != nil {
			return err
		}

		switch f.Format {
		case output.FormatJSON, output.FormatJSONL, output.FormatQuiet:
			return f.Formatter().WriteItem(resp.Data, []output.Column{
				{Header: "REPROMPTED MESSAGE ID", Field: "reprompted_message_id"},
			})
		}

		out := cmd.OutOrStdout()
		status, _ := resp.Data["status"].(string)
		if status == "pending" {
			fmt.Fprintf(out, "Queued. Use `idapt chat messages %s --last 5` to see the new sibling.\n", chatID)
			return nil
		}

		message, _ := resp.Data["message"].(map[string]interface{})
		if message == nil {
			fmt.Fprintln(out, "Completed but produced no new assistant response.")
			fmt.Fprintf(out, "Inspect: `idapt chat runs %s --state failed`\n", chatID)
			return nil
		}
		if modelID, _ := resp.Data["model_id"].(string); modelID != "" {
			fmt.Fprintf(out, "[Model: %s]\n", modelID)
		}
		if content, _ := message["content"].(string); content != "" {
			fmt.Fprintln(out, content)
		}
		return nil
	},
}
var chatMessagesCmd = &cobra.Command{
	Use:               "messages <chat-id>",
	Short:             "List messages in a chat",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeChatIDs,
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		q := buildListQuery(cmd, nil)
		if cmd.Flags().Changed("last") {
			n, _ := cmd.Flags().GetInt("last")
			q.Set("limit", fmt.Sprintf("%d", n))
			q.Del("cursor")
		}

		var resp v1ListResponse
		if err := client.Get(cmd.Context(), "/api/v1/chats/"+args[0]+"/messages", q, &resp); err != nil {
			return err
		}

		return f.Formatter().WriteList(resp.Data, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "ROLE", Field: "role"},
			{Header: "MODEL", Field: "model_id"},
			{Header: "CONTENT", Field: "content", Width: 80},
			{Header: "CREATED", Field: "created_at"},
		})
	},
}
var chatRunsCmd = &cobra.Command{
	Use:               "runs <chat-id>",
	Short:             "List agent runs for a chat (cost, tokens, model, error)",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeChatIDs,
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		q := buildListQuery(cmd, nil)
		if v, _ := cmd.Flags().GetString("state"); v != "" {
			q.Set("state", v)
		}

		var resp v1ListResponse
		if err := client.Get(cmd.Context(), "/api/v1/chats/"+args[0]+"/runs", q, &resp); err != nil {
			return err
		}

		return f.Formatter().WriteList(resp.Data, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "STATE", Field: "state"},
			{Header: "MODEL", Field: "model_id"},
			{Header: "IN_TOK", Field: "total_input_tokens"},
			{Header: "OUT_TOK", Field: "total_output_tokens"},
			{Header: "COST_USD", Field: "total_cost_usd"},
			{Header: "DURATION_S", Field: "duration_seconds"},
			{Header: "ERROR", Field: "error", Width: 40},
		})
	},
}
var chatCostCmd = &cobra.Command{
	Use:               "cost <chat-id>",
	Short:             "Show aggregate cost and token data for a chat",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeChatIDs,
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		var resp v1ItemResponse
		if err := client.Get(cmd.Context(), "/api/v1/chats/"+args[0]+"/cost", nil, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteItem(resp.Data, []output.Column{
			{Header: "TOTAL_COST_USD", Field: "total_cost_usd"},
			{Header: "INPUT_TOKENS", Field: "total_input_tokens"},
			{Header: "OUTPUT_TOKENS", Field: "total_output_tokens"},
			{Header: "CACHED_TOKENS", Field: "total_cached_tokens"},
			{Header: "BUDGET_LIMIT_USD", Field: "cost_budget_limit_usd"},
			{Header: "BUDGET_SPENT_USD", Field: "cost_budget_spent_usd"},
			{Header: "BUDGET_MODE", Field: "cost_budget_mode"},
		})
	},
}
var chatArchiveCmd = &cobra.Command{
	Use:               "archive <chat-id>",
	Short:             "Hide chat from default sidebar (reversible via 'chat unarchive')",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeChatIDs,
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		var resp v1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/chats/"+args[0]+"/archive", nil, &resp); err != nil {
			return err
		}
		return writeChatItem(f, resp.Data)
	},
}
var chatUnarchiveCmd = &cobra.Command{
	Use:               "unarchive <chat-id>",
	Short:             "Restore chat to the default sidebar",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeChatIDs,
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		var resp v1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/chats/"+args[0]+"/unarchive", nil, &resp); err != nil {
			return err
		}
		return writeChatItem(f, resp.Data)
	},
}
var chatRestoreCmd = &cobra.Command{
	Use:               "restore <chat-id>",
	Short:             "Restore a trashed chat to the active list",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeChatIDs,
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		var resp v1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/chats/"+args[0]+"/restore", nil, &resp); err != nil {
			return err
		}
		return writeChatItem(f, resp.Data)
	},
}
var chatPermanentDeleteCmd = &cobra.Command{
	Use:   "permanent-delete <chat-id>",
	Short: "Permanently delete a trashed chat (IRREVERSIBLE)",
	Long: `Hard-delete a chat that is already in the trash. This is IRREVERSIBLE —
messages, agent runs, costs, and shares are all dropped.

Workflow:
  1. ` + "`chat delete <id>`" + ` moves to trash (reversible via ` + "`chat restore`" + `).
  2. ` + "`chat permanent-delete <id>`" + ` hard-deletes from trash (NO undo).

If you only want the chat out of the sidebar, use ` + "`chat archive`" + ` —
it's reversible and keeps everything.`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeChatIDs,
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		if !globalFlags.Confirm {
			if !cmdutil.ConfirmAction(f, fmt.Sprintf("Permanently delete chat %s? This CANNOT be undone.", args[0])) {
				return fmt.Errorf("aborted")
			}
		}

		path := "/api/v1/chats/" + args[0] + "/permanent-delete"
		if err := client.Delete(cmd.Context(), path); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Chat %s permanently deleted.\n", args[0])
		return nil
	},
}
var chatExportCmd = &cobra.Command{
	Use:               "export <chat-id>",
	Short:             "Export a chat as markdown (or text/json/pdf via --format)",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeChatIDs,
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		path := "/api/v1/chats/" + args[0] + "/export"
		var opts []api.RequestOption
		if format, _ := cmd.Flags().GetString("format"); format != "" {
			opts = append(opts, api.WithQuery(url.Values{"format": {format}}))
		}
		result, err := client.Download(cmd.Context(), path, opts...)
		if err != nil {
			return err
		}
		defer result.Body.Close()
		_, err = io.Copy(cmd.OutOrStdout(), result.Body)
		return err
	},
}
var chatStopCmd = &cobra.Command{
	Use:               "stop <chat-id>",
	Short:             "Stop an active chat generation (graceful drain)",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeChatIDs,
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		body := map[string]interface{}{}
		if mode, _ := cmd.Flags().GetString("mode"); mode != "" {
			body["mode"] = mode
		}
		var resp v1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/chats/"+args[0]+"/stop", body, &resp); err != nil {
			return err
		}
		switch f.Format {
		case output.FormatJSON, output.FormatJSONL, output.FormatQuiet:
			return f.Formatter().WriteItem(resp.Data, []output.Column{
				{Header: "RUN_ACTIVE", Field: "run_active"},
			})
		}
		if active, ok := resp.Data["run_active"].(bool); ok && !active {
			fmt.Fprintln(cmd.OutOrStdout(), "No active run — nothing to stop.")
			return nil
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Stop requested.")
		return nil
	},
}
type v1ItemResponse = api.V1ItemResponse
type v1ListResponse = api.V1ListResponse

func writeChatItem(f *cmdutil.Factory, item map[string]interface{}) error {
	return f.Formatter().WriteItem(item, []output.Column{
		{Header: "ID", Field: "id"},
		{Header: "TITLE", Field: "title"},
		{Header: "AGENT_ID", Field: "agent_id"},
		{Header: "WORKSPACE_ID", Field: "workspace_id"},
		{Header: "MODEL", Field: "default_model"},
		{Header: "IS_TRASHED", Field: "is_trashed"},
		{Header: "ARCHIVED_AT", Field: "archived_at"},
		{Header: "CREATED", Field: "created_at"},
	})
}

func validEffortLevel(s string) bool {
	switch s {
	case "fast", "smart", "expert":
		return true
	}
	return false
}

var _ = json.RawMessage(nil)
func init() {
	chatListCmd.Flags().String("agent", "", "Filter by agent ID")
	cmdutil.AddListFlags(chatListCmd)

	chatCreateCmd.Flags().String("agent", "", "Agent resource id or name")
	chatCreateCmd.Flags().String("title", "", "Chat title")
	chatCreateCmd.Flags().String("model", "", "Default model id ('auto' or concrete)")
	cmdutil.AddJSONInput(chatCreateCmd)

	chatEditCmd.Flags().String("title", "", "Chat title")
	chatEditCmd.Flags().String("model", "", "Default model id ('' clears the override)")
	cmdutil.AddJSONInput(chatEditCmd)

	chatSendCmd.Flags().String("model", "", "Override model for this message ('auto', 'openai/gpt-5.5', ...)")
	chatSendCmd.Flags().String("effort-level", "", "Effort mode + Auto-router tier: fast | smart | expert")
	chatSendCmd.Flags().String("branch-from", "", "Append after this message id instead of the active leaf")
	chatSendCmd.Flags().Bool("no-wait", false, "Return immediately; don't wait for the assistant response")
	chatSendCmd.Flags().Int("timeout", 0, "Seconds to wait for completion (server default 120, max 300)")

	chatRepromptCmd.Flags().String("message", "", "User message id to regenerate against (required)")
	chatRepromptCmd.Flags().String("model", "", "Override model for the regeneration")
	chatRepromptCmd.Flags().String("effort-level", "", "Effort mode + Auto-router tier: fast | smart | expert")
	chatRepromptCmd.Flags().Bool("no-wait", false, "Return immediately; don't wait for completion")
	chatRepromptCmd.Flags().Int("timeout", 0, "Seconds to wait for completion (server default 120, max 300)")
	_ = chatRepromptCmd.MarkFlagRequired("message")

	cmdutil.AddListFlags(chatMessagesCmd)
	chatMessagesCmd.Flags().Int("last", 0, "Return only the last N messages (sugar for --limit)")

	chatRunsCmd.Flags().String("state", "", "Filter by run state: generating | streaming | completed | failed | stopped | paused")
	cmdutil.AddListFlags(chatRunsCmd)

	chatExportCmd.Flags().String("format", "", "Export format: markdown (default) | text | json | pdf")

	chatStopCmd.Flags().String("mode", "", "Stop mode: soft (default) | hard")

	_ = chatCreateCmd.RegisterFlagCompletionFunc("agent", completeAgentIDs)
	_ = chatCreateCmd.RegisterFlagCompletionFunc("model", completeModelIDs)
	_ = chatListCmd.RegisterFlagCompletionFunc("agent", completeAgentIDs)
	_ = chatEditCmd.RegisterFlagCompletionFunc("model", completeModelIDs)
	_ = chatSendCmd.RegisterFlagCompletionFunc("model", completeModelIDs)
	_ = chatSendCmd.RegisterFlagCompletionFunc("effort-level", completeEffortLevel)
	_ = chatRepromptCmd.RegisterFlagCompletionFunc("model", completeModelIDs)
	_ = chatRepromptCmd.RegisterFlagCompletionFunc("effort-level", completeEffortLevel)

	chatCmd.AddCommand(chatListCmd)
	chatCmd.AddCommand(chatCreateCmd)
	chatCmd.AddCommand(chatGetCmd)
	chatCmd.AddCommand(chatEditCmd)
	chatCmd.AddCommand(chatDeleteCmd)
	chatCmd.AddCommand(chatSendCmd)
	chatCmd.AddCommand(chatRepromptCmd)
	chatCmd.AddCommand(chatMessagesCmd)
	chatCmd.AddCommand(chatRunsCmd)
	chatCmd.AddCommand(chatCostCmd)
	chatCmd.AddCommand(chatArchiveCmd)
	chatCmd.AddCommand(chatUnarchiveCmd)
	chatCmd.AddCommand(chatRestoreCmd)
	chatCmd.AddCommand(chatPermanentDeleteCmd)
	chatCmd.AddCommand(chatExportCmd)
	chatCmd.AddCommand(chatStopCmd)
}
