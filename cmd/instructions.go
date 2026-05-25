package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

const InstructionsAnnotationKey = "instructions"

var ErrInstructionsShortCircuit = errors.New("instructions short-circuit")

var instructionsCmd = &cobra.Command{
	Use:   "instructions [command...]",
	Short: "Show the instructions (when to use, anti-patterns) for a command",
	Long: `Show the instructions for a command — the opinionated "when to use" / "how to wield" / "anti-patterns" guidance.

Mirrors the LLM-side instructions surface: behavioral guidance separate from the mechanical contract you get from --help / idapt help. **One-tier — instructions are resource-scoped.**

Examples:
  idapt instructions               — instructions index
  idapt instructions drive         — instructions for the drive resource
  idapt instructions drive delete  — same body as 'instructions drive', plus a footer noting it's resource-scoped

Equivalent flag form on any command:
  idapt drive delete --instructions
  idapt computer terminate --instructions
`,
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		root := cmd.Root()
		if len(args) == 0 {
			return printInstructionsIndex(root)
		}
		target, _, err := root.Find(args)
		if err != nil || target == nil || target == root {
			return fmt.Errorf("unknown command %q for %q", strings.Join(args, " "), root.Name())
		}
		printCommandInstructions(target)
		return nil
	},
}

func printInstructionsIndex(root *cobra.Command) error {
	out := root.OutOrStdout()
	fmt.Fprintln(out, "# Idapt CLI — instructions index")
	fmt.Fprintln(out)
	fmt.Fprintln(out,
		"Each command has a CONTRACT (use --help / `idapt help <cmd>`) and INSTRUCTIONS (use --instructions / `idapt instructions <cmd>`).")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Instructions are RESOURCE-SCOPED — there is one instructions body per resource covering all its verbs.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "## Top-level commands")
	fmt.Fprintln(out)
	for _, sub := range root.Commands() {
		if sub.Hidden || sub.Name() == "help" || sub.Name() == "instructions" {
			continue
		}
		marker := " "
		if hasResourceInstructions(sub) {
			marker = "*"
		}
		fmt.Fprintf(out, "  %s %-20s %s\n", marker, sub.Name(), sub.Short)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Lines marked `*` have authored instructions. Others fall back to a stub built from the description.")
	return nil
}

func printCommandInstructions(cmd *cobra.Command) {
	out := cmd.OutOrStdout()
	resource := resourceCommand(cmd)
	body := lookupInstructions(resource)
	verbScoped := cmd != resource

	fmt.Fprintf(out, "# %s — instructions\n\n", resource.CommandPath())
	if body == "" {
		fmt.Fprintf(out, "_(no instructions authored for `%s`)_\n\n", resource.CommandPath())
		fmt.Fprintln(out, "## Description")
		fmt.Fprintln(out)
		if resource.Long != "" {
			fmt.Fprintln(out, resource.Long)
		} else if resource.Short != "" {
			fmt.Fprintln(out, resource.Short)
		}
		fmt.Fprintln(out)
		fmt.Fprintf(out, "For args, types, and examples: `idapt help %s`.\n", trimRoot(resource))
		return
	}
	fmt.Fprintln(out, body)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "---")
	fmt.Fprintf(out, "For args / flags: `idapt help %s`.\n", trimRoot(resource))
	if verbScoped {
		fmt.Fprintln(out)
		fmt.Fprintf(out,
			"_Instructions are resource-scoped — same body as `idapt instructions %s`._\n",
			trimRoot(resource))
	}
}

func resourceCommand(cmd *cobra.Command) *cobra.Command {
	root := cmd.Root()
	cur := cmd
	for cur != root && cur.Parent() != nil && cur.Parent() != root {
		cur = cur.Parent()
	}
	return cur
}

func trimRoot(cmd *cobra.Command) string {
	full := cmd.CommandPath()
	rootName := cmd.Root().Name()
	if strings.HasPrefix(full, rootName+" ") {
		return full[len(rootName)+1:]
	}
	return full
}

func lookupInstructions(cmd *cobra.Command) string {
	if cmd.Annotations == nil {
		return ""
	}
	return cmd.Annotations[InstructionsAnnotationKey]
}

func hasResourceInstructions(cmd *cobra.Command) bool {
	return lookupInstructions(cmd) != ""
}

func MaybePrintInstructions(cmd *cobra.Command) error {
	if globalFlags == nil || !globalFlags.Instructions {
		return nil
	}
	printCommandInstructions(cmd)
	return ErrInstructionsShortCircuit
}

func appendInstructionsFooter(cmd *cobra.Command) {
	root := cmd.Root()
	if cmd == root {
		return
	}
	resource := resourceCommand(cmd)
	if !hasResourceInstructions(resource) {
		return
	}
	out := cmd.OutOrStdout()
	fmt.Fprintln(out)
	fmt.Fprintf(out,
		"Run `idapt instructions %s` for usage guidance, anti-patterns, and the resource playbook.\n",
		trimRoot(resource),
	)
}

func init() {
	origRootHelp := rootCmd.HelpFunc()
	rootCmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		origRootHelp(c, args)
		appendInstructionsFooter(c)
	})
}
