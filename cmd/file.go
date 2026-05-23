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

var fileCmd = &cobra.Command{
	Use:   "file",
	Short: "Manage files",
	Annotations: map[string]string{
		"instructions": `# file — instructions

## Reading vs searching

- ` + "`file read <id>`" + ` — known file id, want the contents.
- ` + "`file list`" + ` — browse a folder.
- For full-text search across many files, use ` + "`idapt file grep`" + ` or
  ` + "`idapt file search`" + ` — both project-scoped.

## Writing

- ` + "`file create`" + ` — new file. Errors if the path already exists.
- ` + "`file write`" + ` — overwrite. Auto-saves a version snapshot first.
- ` + "`file edit`" + ` — patch via str-replace. Cleaner version history than ` + "`write`" + `.

## Delete is destructive — read this before calling

- **Folders cascade.** Deleting a folder removes everything under it.
  Confirm scope before passing a folder id.
- **Versioned files keep history.** A future restore is possible via
  ` + "`idapt file restore-version`" + `.
- **Unversioned files are gone forever.** No undo. No tombstone.
- **Consider archive instead.** ` + "`idapt file move`" + ` into an
  ` + "`Archive/`" + ` folder is reversible; delete is not.
- **Active working dirs may lock.** A folder used by a code-run / git
  verb / CLI mount returns ` + "`resource_locked`" + ` on writes —
  let it free up before retrying.

Use ` + "`--confirm`" + ` to skip the interactive confirmation prompt.`,
	},
}

var fileListCmd = &cobra.Command{
	Use:   "list [parent-id]",
	Short: "List files in a folder",
	Long: `List files. Pass a parent folder resourceId to scope to that folder;
omit it to list the caller's root-level files.

Listing is scoped by folder, not by project: pass the parent-id argument
to scope to a folder, or use ` + "`file grep`" + ` / ` + "`file search`" + `
for project-wide lookups.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		q := url.Values{}
		if len(args) > 0 {
			q.Set("parent_id", args[0])
		}
		q = buildListQuery(cmd, q)

		var resp api.V1ListResponse
		if err := client.Get(cmd.Context(), "/api/v1/files", q, &resp); err != nil {
			return err
		}

		return f.Formatter().WriteList(resp.Data, []output.Column{
			{Header: "ID", Field: "id"},
			{Header: "NAME", Field: "name"},
			{Header: "MIME", Field: "mime_type"},
			{Header: "SIZE", Field: "file_size"},
			{Header: "PARENT_ID", Field: "parent_id"},
			{Header: "MODIFIED", Field: "updated_at"},
		})
	},
}

var fileReadCmd = &cobra.Command{
	Use:   "read <file-id>",
	Short: "Read file contents to stdout",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		result, err := client.Download(cmd.Context(), "/api/v1/files/"+args[0])
		if err != nil {
			return err
		}
		defer result.Body.Close()

		_, err = io.Copy(cmd.OutOrStdout(), result.Body)
		return err
	},
}

var fileWriteCmd = &cobra.Command{
	Use:   "write <file-id>",
	Short: "Write stdin to a file",
	Long:  "Reads content from stdin and writes it to the specified file. Use with pipes: echo 'content' | idapt file write <id>",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		content, err := io.ReadAll(f.In)
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}

		body := map[string]interface{}{"content": string(content)}
		if err := client.Patch(cmd.Context(), "/api/v1/files/"+args[0], body, nil); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "File updated.")
		return nil
	},
}

var fileCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new file",
	Args:  cobra.ExactArgs(1),
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

		body := map[string]interface{}{
			"project_id": projectID,
			"name":       args[0],
		}
		if cmd.Flags().Changed("parent") {
			v, _ := cmd.Flags().GetString("parent")
			body["parent_id"] = v
		}
		if cmd.Flags().Changed("content") {
			v, _ := cmd.Flags().GetString("content")
			body["content"] = v
		}

		var resp api.V1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/files", body, &resp); err != nil {
			return err
		}
		return writeFileItem(f, resp.Data)
	},
}

var fileEditCmd = &cobra.Command{
	Use:   "edit <file-id>",
	Short: "Edit a file (find and replace)",
	Long:  "Fetches the file, replaces old-text with new-text, and writes back.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}

		oldText, _ := cmd.Flags().GetString("old-text")
		newText, _ := cmd.Flags().GetString("new-text")
		if oldText == "" {
			return fmt.Errorf("--old-text is required")
		}

		result, err := client.Download(cmd.Context(), "/api/v1/files/"+args[0])
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

		body := map[string]interface{}{"content": updated}
		if err := client.Patch(cmd.Context(), "/api/v1/files/"+args[0], body, nil); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "File updated.")
		return nil
	},
}

var fileDeleteCmd = &cobra.Command{
	Use:   "delete <file-id>",
	Short: "Delete a file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		if !globalFlags.Confirm {
			if !cmdutil.ConfirmAction(f, fmt.Sprintf("Delete file %s?", args[0])) {
				return fmt.Errorf("aborted")
			}
		}
		if err := client.Delete(cmd.Context(), "/api/v1/files/"+args[0]); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "File deleted.")
		return nil
	},
}

var fileRenameCmd = &cobra.Command{
	Use:   "rename <file-id> <new-name>",
	Short: "Rename a file",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		body := map[string]interface{}{"name": args[1]}
		if err := client.Patch(cmd.Context(), "/api/v1/files/"+args[0], body, nil); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Renamed to %s.\n", args[1])
		return nil
	},
}

var fileMoveCmd = &cobra.Command{
	Use:   "move <file-id> <target-parent-id>",
	Short: "Move a file to a different folder",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		body := map[string]interface{}{"parent_id": args[1]}
		if err := client.Post(cmd.Context(), "/api/v1/files/"+args[0]+"/move", body, nil); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "File moved.")
		return nil
	},
}

var fileMkdirCmd = &cobra.Command{
	Use:   "mkdir <name>",
	Short: "Create a directory",
	Args:  cobra.ExactArgs(1),
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
		body := map[string]interface{}{
			"project_id": projectID,
			"name":       args[0],
		}
		if cmd.Flags().Changed("parent") {
			v, _ := cmd.Flags().GetString("parent")
			body["parent_id"] = v
		}
		var resp api.V1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/files/folders", body, &resp); err != nil {
			return err
		}
		return writeFileItem(f, resp.Data)
	},
}

var fileGrepCmd = &cobra.Command{
	Use:   "grep <pattern>",
	Short: "Search file contents for a pattern",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runFileSearch(cmd, args[0], "content")
	},
}

var fileGlobCmd = &cobra.Command{
	Use:   "glob <pattern>",
	Short: "Find files matching a glob pattern",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runFileSearch(cmd, args[0], "glob")
	},
}

var fileSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search files by name",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runFileSearch(cmd, args[0], "name")
	},
}

func runFileSearch(cmd *cobra.Command, query, searchType string) error {
	f := cmdutil.FactoryFromCmd(cmd)
	client, err := f.APIClient()
	if err != nil {
		return err
	}
	_, err = resolveProjectFlag(cmd, f)
	if err != nil {
		return err
	}
	q := url.Values{
		"q": {query},
	}
	if searchType != "" && searchType != "name" {
		q.Set("type", searchType)
	}
	var resp api.V1ListResponse
	if err := client.Get(cmd.Context(), "/api/v1/search", q, &resp); err != nil {
		return err
	}
	return f.Formatter().WriteList(resp.Data, []output.Column{
		{Header: "ID", Field: "id"},
		{Header: "NAME", Field: "name"},
		{Header: "TYPE", Field: "type"},
		{Header: "PATH", Field: "path"},
		{Header: "SNIPPET", Field: "snippet", Width: 80},
	})
}

var fileUploadCmd = &cobra.Command{
	Use:   "upload <local-path>",
	Short: "Upload a file",
	Args:  cobra.ExactArgs(1),
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
		fh, err := os.Open(args[0])
		if err != nil {
			return fmt.Errorf("opening file: %w", err)
		}
		defer fh.Close()
		fi, err := fh.Stat()
		if err != nil {
			return err
		}

		fields := map[string]string{"project_id": projectID}
		if cmd.Flags().Changed("parent") {
			v, _ := cmd.Flags().GetString("parent")
			fields["parent_id"] = v
		}

		resp, err := client.Upload(cmd.Context(), "/api/v1/files", fi.Name(), fh, fields)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		fmt.Fprintf(cmd.OutOrStdout(), "Uploaded %s (%d bytes).\n", fi.Name(), fi.Size())
		return nil
	},
}

var fileDownloadCmd = &cobra.Command{
	Use:   "download <file-id> [local-path]",
	Short: "Download a file",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		result, err := client.Download(cmd.Context(), "/api/v1/files/"+args[0])
		if err != nil {
			return err
		}
		defer result.Body.Close()

		outPath := result.Filename
		if len(args) > 1 {
			outPath = args[1]
		}
		if outPath == "" {
			outPath = args[0]
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

func writeFileItem(f *cmdutil.Factory, item map[string]interface{}) error {
	return f.Formatter().WriteItem(item, []output.Column{
		{Header: "ID", Field: "id"},
		{Header: "NAME", Field: "name"},
		{Header: "MIME", Field: "mime_type"},
		{Header: "SIZE", Field: "file_size"},
		{Header: "PARENT_ID", Field: "parent_id"},
		{Header: "PROJECT_ID", Field: "project_id"},
	})
}

func init() {
	cmdutil.AddListFlags(fileListCmd)

	fileCreateCmd.Flags().String("parent", "", "Parent folder ID")
	fileCreateCmd.Flags().String("content", "", "Initial file content")

	fileEditCmd.Flags().String("old-text", "", "Text to find")
	fileEditCmd.Flags().String("new-text", "", "Replacement text")

	fileMkdirCmd.Flags().String("parent", "", "Parent folder ID")
	fileUploadCmd.Flags().String("parent", "", "Parent folder ID")

	fileCmd.AddCommand(fileListCmd)
	fileCmd.AddCommand(fileReadCmd)
	fileCmd.AddCommand(fileWriteCmd)
	fileCmd.AddCommand(fileCreateCmd)
	fileCmd.AddCommand(fileEditCmd)
	fileCmd.AddCommand(fileDeleteCmd)
	fileCmd.AddCommand(fileRenameCmd)
	fileCmd.AddCommand(fileMoveCmd)
	fileCmd.AddCommand(fileMkdirCmd)
	fileCmd.AddCommand(fileGrepCmd)
	fileCmd.AddCommand(fileGlobCmd)
	fileCmd.AddCommand(fileSearchCmd)
	fileCmd.AddCommand(fileUploadCmd)
	fileCmd.AddCommand(fileDownloadCmd)
}
