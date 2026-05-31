package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/resolve"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

var openCmd = &cobra.Command{
	Use:   "open [resource-id | <type> <name-or-id>]",
	Short: "Open the idapt web app (or a resource) in your browser",
	Long: `Open the idapt web app in your default browser.

  idapt open                        # the app home
  idapt open <resource-id>          # jump to any resource by id (/res/<id>)
  idapt open chat <name-or-id>      # resolve a name first, then open
  idapt open --print <resource-id>  # print the URL instead of launching

Every resource type shares one canonical web URL (/res/<id>). Resolvable
<type>s: agent, chat, computer, file, script, secret, trigger, workspace.

Honors $BROWSER. In a non-interactive shell (no TTY) it prints the URL
instead of trying to launch a browser, so it's pipe-friendly.`,
	Example: `  idapt open
  idapt open 3f8hnkm2vp7qsgt0yca4testpr
  idapt open chat my-chat
  idapt open --print agent my-agent`,
	Args: cobra.RangeArgs(0, 2),
	RunE: runOpen,
}

func init() {
	openCmd.Flags().Bool("print", false, "Print the URL instead of launching a browser")
	rootCmd.AddCommand(openCmd)
}

func runOpen(cmd *cobra.Command, args []string) error {
	f := cmdutil.FactoryFromCmd(cmd)
	base := webBaseURL(f)

	target := base
	switch len(args) {
	case 1:
		if !resolve.IsResourceId(args[0]) {
			return fmt.Errorf("%q is not a resource id; use `idapt open <type> <name>` to resolve a name (e.g. `idapt open chat my-chat`)", args[0])
		}
		target = base + "/res/" + args[0]
	case 2:
		id, err := resolveOpenTarget(cmd, f, args[0], args[1])
		if err != nil {
			return err
		}
		target = base + "/res/" + id
	}

	doPrint, _ := cmd.Flags().GetBool("print")
	if doPrint || !isatty.IsTerminal(os.Stdout.Fd()) {
		fmt.Fprintln(cmd.OutOrStdout(), target)
		return nil
	}
	if err := launchBrowser(target); err != nil {
		fmt.Fprintln(cmd.OutOrStdout(), target)
		return nil
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Opening %s\n", target)
	return nil
}

func webBaseURL(f *cmdutil.Factory) string {
	base := globalFlags.APIURL
	if base == "" && f != nil {
		base = f.Config.APIURL
	}
	if base == "" {
		base = "https://idapt.ai"
	}
	return strings.TrimRight(base, "/")
}

func resolveOpenTarget(cmd *cobra.Command, f *cmdutil.Factory, resType, nameOrID string) (string, error) {
	if resType == "workspace" {
		return resolveWorkspaceArg(cmd, f, nameOrID)
	}
	if resolve.IsResourceId(nameOrID) {
		return nameOrID, nil
	}
	workspaceID, err := resolveWorkspaceFlag(cmd, f)
	if err != nil {
		return "", err
	}
	return resolveResource(cmd, f, resType, nameOrID, workspaceID)
}

func launchBrowser(url string) error {
	bin, args := browserCommand(runtime.GOOS, os.Getenv("BROWSER"), url)
	if bin == "" {
		return fmt.Errorf("no browser opener for this platform")
	}
	return exec.Command(bin, args...).Start()
}

func browserCommand(goos, browserEnv, url string) (string, []string) {
	if fields := strings.Fields(browserEnv); len(fields) > 0 {
		return fields[0], append(fields[1:], url)
	}
	switch goos {
	case "darwin":
		return "open", []string{url}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		return "xdg-open", []string{url}
	}
}
