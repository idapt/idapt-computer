package cmd

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/input"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/spf13/cobra"
)

var subagentCmd = &cobra.Command{
	Use:     "subagent",
	Aliases: []string{"ma"},
	Short:   "Subagent orchestration",
}

var subagentChatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Manage subagent chats",
}

var subagentChatCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a subagent chat (child chat)",
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
		if cmd.Flags().Changed("parent-chat") {
			v, _ := cmd.Flags().GetString("parent-chat")
			overrides["parentChatId"] = v
		}
		if cmd.Flags().Changed("agent") {
			v, _ := cmd.Flags().GetString("agent")
			overrides["agentId"] = v
		}
		if cmd.Flags().Changed("message") {
			v, _ := cmd.Flags().GetString("message")
			overrides["message"] = v
		}
		body = input.MergeFlags(body, overrides)

		var resp map[string]interface{}
		if err := client.Post(cmd.Context(), "/api/subagent/chat", body, &resp); err != nil {
			return err
		}

		formatter := f.Formatter()
		return formatter.WriteItem(resp, []output.Column{
			{Header: "CHAT ID", Field: "chatId"},
			{Header: "AGENT", Field: "agentId"},
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

		var resp struct {
			Chats []map[string]interface{} `json:"chats"`
		}
		if err := client.Get(cmd.Context(), "/api/subagent/chat/"+args[0]+"/children", nil, &resp); err != nil {
			return err
		}

		formatter := f.Formatter()
		return formatter.WriteList(resp.Chats, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "AGENT", Field: "agentId"},
			{Header: "STATUS", Field: "status"},
			{Header: "CREATED", Field: "createdAt"},
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

		var resp map[string]interface{}
		if err := client.Patch(cmd.Context(), "/api/subagent/chat/"+args[0], body, &resp); err != nil {
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
	Short: "Send a message to a child agent chat",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		body := map[string]interface{}{
			"content": map[string]interface{}{
				"text": args[1],
			},
		}

		if err := client.Post(cmd.Context(), "/api/subagent/chat/"+args[0]+"/message", body, nil); err != nil {
			return err
		}

		noStream, _ := cmd.Flags().GetBool("no-stream")
		if noStream {
			fmt.Fprintln(cmd.OutOrStdout(), "Message sent.")
			return nil
		}

		reader, err := client.StreamSSEGet(cmd.Context(), "/api/subagent/chat/"+args[0]+"/stream")
		if err != nil {
			return err
		}
		defer reader.Close()

		for {
			event, err := reader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}

			switch event.Event {
			case "text-delta":
				var delta struct {
					Text string `json:"text"`
				}
				if json.Unmarshal([]byte(event.Data), &delta) == nil {
					fmt.Fprint(cmd.OutOrStdout(), delta.Text)
				}
			case "done", "error":
				fmt.Fprintln(cmd.OutOrStdout())
				if event.Event == "error" {
					return fmt.Errorf("stream error: %s", event.Data)
				}
				return nil
			}
		}

		fmt.Fprintln(cmd.OutOrStdout())
		return nil
	},
}

var subagentMessageListCmd = &cobra.Command{
	Use:   "list <chat-id>",
	Short: "List messages in a child agent chat",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		var resp struct {
			Messages []map[string]interface{} `json:"messages"`
		}
		if err := client.Get(cmd.Context(), "/api/subagent/chat/"+args[0]+"/messages", nil, &resp); err != nil {
			return err
		}

		formatter := f.Formatter()
		return formatter.WriteList(resp.Messages, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "TYPE", Field: "type"},
			{Header: "TEXT", Field: "userText", Width: 80},
			{Header: "ASSISTANT", Field: "assistantText", Width: 80},
		})
	},
}

var subagentMessageGetCmd = &cobra.Command{
	Use:   "get <chat-id> <message-id>",
	Short: "Get a specific message from a child agent chat",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		var resp map[string]interface{}
		if err := client.Get(cmd.Context(), "/api/subagent/chat/"+args[0]+"/messages/"+args[1], nil, &resp); err != nil {
			return err
		}

		formatter := f.Formatter()
		return formatter.WriteItem(resp, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "TYPE", Field: "type"},
			{Header: "TEXT", Field: "userText"},
			{Header: "ASSISTANT", Field: "assistantText"},
			{Header: "CREATED", Field: "createdAt"},
		})
	},
}

func init() {
	subagentChatCreateCmd.Flags().String("parent-chat", "", "Parent chat ID")
	subagentChatCreateCmd.Flags().String("agent", "", "Agent ID for the child chat")
	subagentChatCreateCmd.Flags().String("message", "", "Initial message to send")
	cmdutil.AddJSONInput(subagentChatCreateCmd)

	cmdutil.AddJSONInput(subagentChatEditCmd)

	subagentMessageSendCmd.Flags().Bool("no-stream", false, "Don't stream the response")

	subagentChatCmd.AddCommand(subagentChatCreateCmd)
	subagentChatCmd.AddCommand(subagentChatListCmd)
	subagentChatCmd.AddCommand(subagentChatEditCmd)

	subagentMessageCmd.AddCommand(subagentMessageSendCmd)
	subagentMessageCmd.AddCommand(subagentMessageListCmd)
	subagentMessageCmd.AddCommand(subagentMessageGetCmd)

	subagentCmd.AddCommand(subagentChatCmd)
	subagentCmd.AddCommand(subagentMessageCmd)
}
