
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion",
	Short: "Generate or install shell autocompletion scripts",
	Long: `Generate the autocompletion script for the idapt CLI.

Run ` + "`idapt completion install`" + ` to print a one-liner you can paste
into your shell's rc file (~/.bashrc, ~/.zshrc, ~/.config/fish/config.fish,
or PowerShell $PROFILE). The CLI does not touch the filesystem itself —
you remain in control of what gets written where.

Per-shell generators (` + "`idapt completion bash`" + ` etc.) are also
available for users who prefer to drop the script into a system-wide
completions directory (e.g. /etc/bash_completion.d/).`,
}

var completionBashCmd = &cobra.Command{
	Use:                   "bash",
	Short:                 "Print the bash autocompletion script",
	DisableFlagsInUseLine: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		noDesc, _ := cmd.Flags().GetBool("no-descriptions")
		if noDesc {
			return cmd.Root().GenBashCompletionV2(cmd.OutOrStdout(), false)
		}
		return cmd.Root().GenBashCompletionV2(cmd.OutOrStdout(), true)
	},
}

var completionZshCmd = &cobra.Command{
	Use:                   "zsh",
	Short:                 "Print the zsh autocompletion script",
	DisableFlagsInUseLine: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		noDesc, _ := cmd.Flags().GetBool("no-descriptions")
		if noDesc {
			return cmd.Root().GenZshCompletionNoDesc(cmd.OutOrStdout())
		}
		return cmd.Root().GenZshCompletion(cmd.OutOrStdout())
	},
}

var completionFishCmd = &cobra.Command{
	Use:                   "fish",
	Short:                 "Print the fish autocompletion script",
	DisableFlagsInUseLine: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		noDesc, _ := cmd.Flags().GetBool("no-descriptions")
		return cmd.Root().GenFishCompletion(cmd.OutOrStdout(), !noDesc)
	},
}

var completionPowershellCmd = &cobra.Command{
	Use:                   "powershell",
	Short:                 "Print the PowerShell autocompletion script",
	DisableFlagsInUseLine: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		noDesc, _ := cmd.Flags().GetBool("no-descriptions")
		if noDesc {
			return cmd.Root().GenPowerShellCompletion(cmd.OutOrStdout())
		}
		return cmd.Root().GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
	},
}

var completionInstallCmd = &cobra.Command{
	Use:   "install [shell]",
	Short: "Print a one-liner to enable autocompletion in your shell",
	Long: `Print a one-liner that loads idapt's autocompletion into your current shell.

The CLI does NOT modify your rc files — copy the printed line into the
file the message names (e.g. ~/.bashrc, ~/.zshrc) so it runs on every
new shell session.

With no argument, detects the shell from $SHELL. Pass an explicit shell
name (bash / zsh / fish / powershell) to override.`,
	Args:      cobra.MaximumNArgs(1),
	ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
	RunE: func(cmd *cobra.Command, args []string) error {
		shell := ""
		if len(args) == 1 {
			shell = strings.ToLower(args[0])
		} else {
			shell = detectShell()
		}
		return printCompletionInstallHint(cmd, shell)
	},
}

func detectShell() string {
	if s := os.Getenv("SHELL"); s != "" {
		base := filepath.Base(s)
		switch {
		case strings.HasPrefix(base, "bash"):
			return "bash"
		case strings.HasPrefix(base, "zsh"):
			return "zsh"
		case strings.HasPrefix(base, "fish"):
			return "fish"
		}
	}
	if os.Getenv("PSModulePath") != "" {
		return "powershell"
	}
	return ""
}

func printCompletionInstallHint(cmd *cobra.Command, shell string) error {
	out := cmd.OutOrStdout()
	switch shell {
	case "bash":
		fmt.Fprintln(out, "# Enable idapt completion for the current bash session:")
		fmt.Fprintln(out, `source <(idapt completion bash)`)
		fmt.Fprintln(out)
		fmt.Fprintln(out, "# To enable it for every future session, append this to ~/.bashrc:")
		fmt.Fprintln(out, `source <(idapt completion bash)`)
		fmt.Fprintln(out)
		fmt.Fprintln(out, "# (Optional) System-wide install — drop the script under bash-completion.d:")
		fmt.Fprintln(out, `sudo sh -c 'idapt completion bash > /etc/bash_completion.d/idapt'`)
		return nil
	case "zsh":
		fmt.Fprintln(out, "# Enable idapt completion for the current zsh session:")
		fmt.Fprintln(out, `source <(idapt completion zsh); compdef _idapt idapt`)
		fmt.Fprintln(out)
		fmt.Fprintln(out, "# To enable it for every future session, append this to ~/.zshrc:")
		fmt.Fprintln(out, `autoload -Uz compinit && compinit`)
		fmt.Fprintln(out, `source <(idapt completion zsh)`)
		fmt.Fprintln(out, `compdef _idapt idapt`)
		fmt.Fprintln(out)
		fmt.Fprintln(out, "# (Optional) System-wide install via the zsh function path:")
		fmt.Fprintln(out, `idapt completion zsh > "${fpath[1]}/_idapt"`)
		return nil
	case "fish":
		fmt.Fprintln(out, "# Enable idapt completion for the current fish session:")
		fmt.Fprintln(out, `idapt completion fish | source`)
		fmt.Fprintln(out)
		fmt.Fprintln(out, "# To enable it for every future session:")
		fmt.Fprintln(out, `idapt completion fish > ~/.config/fish/completions/idapt.fish`)
		return nil
	case "powershell":
		fmt.Fprintln(out, "# Enable idapt completion for the current PowerShell session:")
		fmt.Fprintln(out, `idapt completion powershell | Out-String | Invoke-Expression`)
		fmt.Fprintln(out)
		fmt.Fprintln(out, "# To enable it for every future session, append the same line to your $PROFILE:")
		fmt.Fprintln(out, `idapt completion powershell >> $PROFILE`)
		return nil
	}
	fmt.Fprintln(out, "Could not detect your shell from $SHELL.")
	fmt.Fprintln(out, "Pass an explicit shell name:")
	fmt.Fprintln(out, "  idapt completion install bash")
	fmt.Fprintln(out, "  idapt completion install zsh")
	fmt.Fprintln(out, "  idapt completion install fish")
	fmt.Fprintln(out, "  idapt completion install powershell")
	return nil
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	for _, sub := range []*cobra.Command{
		completionBashCmd, completionZshCmd, completionFishCmd, completionPowershellCmd,
	} {
		sub.Flags().Bool("no-descriptions", false, "Disable completion descriptions")
		completionCmd.AddCommand(sub)
	}
	completionCmd.AddCommand(completionInstallCmd)

	rootCmd.AddCommand(completionCmd)
}
