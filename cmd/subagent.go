package cmd

import (
	"fmt"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/input"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/spf13/cobra"
)

var subagentCmd = &cobra.Command{
	Use:     "subagent",
	Aliases: []string{"ma"},
	Short:   "Subagent orchestration (alias over /v1/chats)",
}

var subagentChatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Manage subagent chats",
}

var subagentChatCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a subagent (child) chat",
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
		if cmd.Flags().Changed("agent") {
			v, _ := cmd.Flags().GetString("agent")
			body["agent_id"] = v
		}
		if cmd.Flags().Changed("parent-chat") {
			v, _ := cmd.Flags().GetString("parent-chat")
			body["parent_chat_id"] = v
		}
		if cmd.Flags().Changed("title") {
			v, _ := cmd.Flags().GetString("title")
			body["title"] = v
		}

		var resp api.V1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/chats", body, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteItem(resp.Data, []output.Column{
			{Header: "CHAT ID", Field: "id"},
			{Header: "AGENT", Field: "agent_id"},
			{Header: "TITLE", Field: "title"},
		})
	},
}

var subagentChatListCmd = &cobra.Command{
	Use:   "list <parent-chat-id>",
	Short: "List child chats for a parent chat",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		var resp api.V1ListResponse
		if err := client.Get(cmd.Context(), "/api/v1/chats", nil, &resp); err != nil {
			return err
		}
		children := []map[string]interface{}{}
		for _, row := range resp.Data {
			if parent, _ := row["parent_chat_id"].(string); parent == args[0] {
				children = append(children, row)
			}
		}
		return f.Formatter().WriteList(children, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "AGENT", Field: "agent_id"},
			{Header: "TITLE", Field: "title"},
			{Header: "CREATED", Field: "created_at"},
		})
	},
}

var subagentChatEditCmd = &cobra.Command{
	Use:   "edit <chat-id>",
	Short: "Edit a child chat",
	Args:  cobra.ExactArgs(1),
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
		if err := client.Patch(cmd.Context(), "/api/v1/chats/"+args[0], body, nil); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Child chat updated.")
		return nil
	},
}
var subagentMessageCmd = &cobra.Command{
	Use:   "message",
	Short: "Send and read subagent messages",
}

var subagentMessageSendCmd = &cobra.Command{
	Use:   "send <chat-id> <message>",
	Short: "Send a message to a subagent chat",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		noWait, _ := cmd.Flags().GetBool("no-wait")
		body := map[string]interface{}{
			"content": args[1],
			"wait":    !noWait,
		}
		var resp api.V1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/chats/"+args[0]+"/messages", body, &resp); err != nil {
			return err
		}
		if msg, ok := resp.Data["message"].(map[string]interface{}); ok {
			if content, ok := msg["content"].(string); ok {
				fmt.Fprintln(cmd.OutOrStdout(), content)
				return nil
			}
		}
		return f.Formatter().WriteItem(resp.Data, []output.Column{
			{Header: "STATUS", Field: "status"},
			{Header: "CHAT_ID", Field: "chat_id"},
			{Header: "PENDING TOKEN", Field: "pending_token"},
		})
	},
}

var subagentMessageListCmd = &cobra.Command{
	Use:   "list <chat-id>",
	Short: "List messages in a subagent chat",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		var resp api.V1ListResponse
		if err := client.Get(cmd.Context(), "/api/v1/chats/"+args[0]+"/messages", nil, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteList(resp.Data, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "ROLE", Field: "role"},
			{Header: "CONTENT", Field: "content", Width: 80},
			{Header: "MODEL", Field: "model_id"},
			{Header: "CREATED", Field: "created_at"},
		})
	},
}

var subagentMessageGetCmd = &cobra.Command{
	Use:   "get <chat-id> <message-id>",
	Short: "Get a specific message from a subagent chat",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		var resp api.V1ListResponse
		if err := client.Get(cmd.Context(), "/api/v1/chats/"+args[0]+"/messages", nil, &resp); err != nil {
			return err
		}
		for _, row := range resp.Data {
			if id, _ := row["id"].(string); id == args[1] {
				return f.Formatter().WriteItem(row, []output.Column{
					{Header: "ID", Field: "id"},
					{Header: "ROLE", Field: "role"},
					{Header: "CONTENT", Field: "content"},
					{Header: "MODEL", Field: "model_id"},
					{Header: "CREATED", Field: "created_at"},
				})
			}
		}
		return fmt.Errorf("message %s not found in chat %s", args[1], args[0])
	},
}

func init() {
	subagentChatCreateCmd.Flags().String("agent", "", "Agent resourceId for the child chat")
	subagentChatCreateCmd.Flags().String("parent-chat", "", "Parent chat resourceId")
	subagentChatCreateCmd.Flags().String("title", "", "Chat title")
	cmdutil.AddJSONInput(subagentChatCreateCmd)

	cmdutil.AddJSONInput(subagentChatEditCmd)

	subagentMessageSendCmd.Flags().Bool("no-wait", false, "Return immediately without waiting for the response")

	subagentChatCmd.AddCommand(subagentChatCreateCmd)
	subagentChatCmd.AddCommand(subagentChatListCmd)
	subagentChatCmd.AddCommand(subagentChatEditCmd)

	subagentMessageCmd.AddCommand(subagentMessageSendCmd)
	subagentMessageCmd.AddCommand(subagentMessageListCmd)
	subagentMessageCmd.AddCommand(subagentMessageGetCmd)

	subagentCmd.AddCommand(subagentChatCmd)
	subagentCmd.AddCommand(subagentMessageCmd)
}
