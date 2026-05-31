package cmd

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/spf13/cobra"
)

var computerFileCmd = &cobra.Command{
	Use:   "file",
	Short: "Manage files on a computer via SFTP",
}

var computerFileListCmd = &cobra.Command{
	Use:   "list <computer-id-or-name> <path>",
	Short: "List files on a computer",
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

		body := map[string]interface{}{
			"op":   "list",
			"path": args[1],
		}
		var resp api.V1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/computers/"+id+"/sftp", body, &resp); err != nil {
			return err
		}

		formatter := f.Formatter()
		return formatter.WriteList(api.AsMapSlice(resp.Data["entries"]), []output.Column{
			{Header: "NAME", Field: "name"},
			{Header: "PATH", Field: "path"},
			{Header: "DIR", Field: "is_directory"},
			{Header: "SIZE", Field: "size"},
			{Header: "PERMS", Field: "permissions"},
			{Header: "MODIFIED_MS", Field: "modified_at_ms"},
		})
	},
}

var computerFileReadCmd = &cobra.Command{
	Use:   "read <computer-id-or-name> <path>",
	Short: "Read a file from a computer",
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

		q := url.Values{"path": {args[1]}}
		result, err := client.Download(cmd.Context(), "/api/v1/computers/"+id+"/sftp/download?"+q.Encode())
		if err != nil {
			return err
		}
		defer result.Body.Close()

		_, err = io.Copy(cmd.OutOrStdout(), result.Body)
		return err
	},
}

var computerFileWriteCmd = &cobra.Command{
	Use:   "write <computer-id-or-name> <path>",
	Short: "Write stdin to a file on a computer",
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

		body := map[string]interface{}{
			"op":   "write",
			"path": args[1],
		}

		content, err := io.ReadAll(f.In)
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
		body["content"] = string(content)

		if err := client.Post(cmd.Context(), "/api/v1/computers/"+id+"/sftp", body, nil); err != nil {
			return err
		}

		fmt.Fprintln(cmd.OutOrStdout(), "File written.")
		return nil
	},
}

var computerFileEditCmd = &cobra.Command{
	Use:   "edit <computer-id-or-name> <path>",
	Short: "Edit a file on a computer (find and replace)",
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

		oldText, _ := cmd.Flags().GetString("old-text")
		newText, _ := cmd.Flags().GetString("new-text")
		if oldText == "" {
			return fmt.Errorf("--old-text is required")
		}

		q := url.Values{"path": {args[1]}}
		result, err := client.Download(cmd.Context(), "/api/v1/computers/"+id+"/sftp/download?"+q.Encode())
		if err != nil {
			return err
		}
		defer result.Body.Close()

		data, err := io.ReadAll(result.Body)
		if err != nil {
			return err
		}

		content := string(data)
		if !strings.Contains(content, oldText) {
			return fmt.Errorf("old-text not found in file")
		}
		updated := strings.Replace(content, oldText, newText, 1)

		body := map[string]interface{}{
			"op":      "write",
			"path":    args[1],
			"content": updated,
		}
		if err := client.Post(cmd.Context(), "/api/v1/computers/"+id+"/sftp", body, nil); err != nil {
			return err
		}

		fmt.Fprintln(cmd.OutOrStdout(), "File updated.")
		return nil
	},
}

var computerFileDeleteCmd = &cobra.Command{
	Use:   "delete <computer-id-or-name> <path>",
	Short: "Delete a file on a computer",
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
			if !cmdutil.ConfirmAction(f, fmt.Sprintf("Delete %s?", args[1])) {
				return fmt.Errorf("aborted")
			}
		}

		body := map[string]interface{}{
			"op":   "delete",
			"path": args[1],
		}
		if err := client.Post(cmd.Context(), "/api/v1/computers/"+id+"/sftp", body, nil); err != nil {
			return err
		}

		fmt.Fprintln(cmd.OutOrStdout(), "File deleted.")
		return nil
	},
}

var computerFileMkdirCmd = &cobra.Command{
	Use:   "mkdir <computer-id-or-name> <path>",
	Short: "Create a directory on a computer",
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

		body := map[string]interface{}{
			"op":   "mkdir",
			"path": args[1],
		}
		if err := client.Post(cmd.Context(), "/api/v1/computers/"+id+"/sftp", body, nil); err != nil {
			return err
		}

		fmt.Fprintln(cmd.OutOrStdout(), "Directory created.")
		return nil
	},
}

var computerFileMoveCmd = &cobra.Command{
	Use:   "move <computer-id-or-name> <src> <dst>",
	Short: "Move/rename a file on a computer",
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

		body := map[string]interface{}{
			"op":          "rename",
			"path":        args[1],
			"destination": args[2],
		}
		if err := client.Post(cmd.Context(), "/api/v1/computers/"+id+"/sftp", body, nil); err != nil {
			return err
		}

		fmt.Fprintln(cmd.OutOrStdout(), "File moved.")
		return nil
	},
}

var computerFileStatCmd = &cobra.Command{
	Use:   "stat <computer-id-or-name> <path>",
	Short: "Get file info on a computer",
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

		body := map[string]interface{}{
			"op":   "stat",
			"path": args[1],
		}
		var resp api.V1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/computers/"+id+"/sftp", body, &resp); err != nil {
			return err
		}

		entry, _ := resp.Data["entry"].(map[string]interface{})
		formatter := f.Formatter()
		return formatter.WriteItem(entry, []output.Column{
			{Header: "NAME", Field: "name"},
			{Header: "PATH", Field: "path"},
			{Header: "DIR", Field: "is_directory"},
			{Header: "SIZE", Field: "size"},
			{Header: "PERMS", Field: "permissions"},
			{Header: "OWNER", Field: "owner"},
			{Header: "MODIFIED_MS", Field: "modified_at_ms"},
		})
	},
}

var computerFileGrepCmd = &cobra.Command{
	Use:   "grep <computer-id-or-name> <pattern> [path]",
	Short: "Search file contents on a computer",
	Args:  cobra.RangeArgs(2, 3),
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

		grepPath := "."
		if len(args) > 2 {
			grepPath = args[2]
		}

		grepCmd := fmt.Sprintf("grep -rn %q %s", args[1], grepPath)
		body := map[string]interface{}{
			"command": grepCmd,
		}

		var resp api.V1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/computers/"+id+"/exec", body, &resp); err != nil {
			return err
		}

		if out, ok := resp.Data["stdout"].(string); ok {
			fmt.Fprint(cmd.OutOrStdout(), out)
		}
		return nil
	},
}

var computerFileFindCmd = &cobra.Command{
	Use:   "find <computer-id-or-name> <pattern> [path]",
	Short: "Find files on a computer",
	Args:  cobra.RangeArgs(2, 3),
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

		findPath := "."
		if len(args) > 2 {
			findPath = args[2]
		}

		findCmd := fmt.Sprintf("find %s -name %q", findPath, args[1])
		body := map[string]interface{}{
			"command": findCmd,
		}

		var resp api.V1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/computers/"+id+"/exec", body, &resp); err != nil {
			return err
		}

		if out, ok := resp.Data["stdout"].(string); ok {
			fmt.Fprint(cmd.OutOrStdout(), out)
		}
		return nil
	},
}

var computerFileUploadCmd = &cobra.Command{
	Use:   "upload <computer-id-or-name> <local-path> <remote-path>",
	Short: "Upload a file to a computer",
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

		file, err := os.Open(args[1])
		if err != nil {
			return fmt.Errorf("opening file: %w", err)
		}
		defer file.Close()

		fi, err := file.Stat()
		if err != nil {
			return err
		}

		fields := map[string]string{
			"path": args[2],
		}

		resp, err := client.Upload(cmd.Context(), "/api/v1/computers/"+id+"/sftp/upload", fi.Name(), file, fields)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		fmt.Fprintf(cmd.OutOrStdout(), "Uploaded %s to %s.\n", args[1], args[2])
		return nil
	},
}

var computerFileDownloadCmd = &cobra.Command{
	Use:   "download <computer-id-or-name> <remote-path> [local-path]",
	Short: "Download a file from a computer",
	Args:  cobra.RangeArgs(2, 3),
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

		q := url.Values{"path": {args[1]}}
		result, err := client.Download(cmd.Context(), "/api/v1/computers/"+id+"/sftp/download?"+q.Encode())
		if err != nil {
			return err
		}
		defer result.Body.Close()

		outPath := args[1]
		if len(args) > 2 {
			outPath = args[2]
		}
		if result.Filename != "" && len(args) <= 2 {
			outPath = result.Filename
		}

		out, err := os.Create(outPath)
		if err != nil {
			return err
		}
		defer out.Close()

		n, err := io.Copy(out, result.Body)
		if err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Downloaded %s (%d bytes).\n", outPath, n)
		return nil
	},
}

func init() {
	computerFileEditCmd.Flags().String("old-text", "", "Text to find")
	computerFileEditCmd.Flags().String("new-text", "", "Replacement text")

	computerFileCmd.AddCommand(computerFileListCmd)
	computerFileCmd.AddCommand(computerFileReadCmd)
	computerFileCmd.AddCommand(computerFileWriteCmd)
	computerFileCmd.AddCommand(computerFileEditCmd)
	computerFileCmd.AddCommand(computerFileDeleteCmd)
	computerFileCmd.AddCommand(computerFileMkdirCmd)
	computerFileCmd.AddCommand(computerFileMoveCmd)
	computerFileCmd.AddCommand(computerFileStatCmd)
	computerFileCmd.AddCommand(computerFileGrepCmd)
	computerFileCmd.AddCommand(computerFileFindCmd)
	computerFileCmd.AddCommand(computerFileUploadCmd)
	computerFileCmd.AddCommand(computerFileDownloadCmd)
}
