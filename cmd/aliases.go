package cmd

import "github.com/spf13/cobra"

func applyCommandAliases(root *cobra.Command) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			switch sub.Name() {
			case "list":
				addAliasIfFree(sub, "ls")
			case "delete":
				addAliasIfFree(sub, "rm")
				addAliasIfFree(sub, "del")
			}
			walk(sub)
		}
	}
	walk(root)
}

func addAliasIfFree(cmd *cobra.Command, alias string) {
	for _, a := range cmd.Aliases {
		if a == alias {
			return
		}
	}
	if parent := cmd.Parent(); parent != nil {
		for _, sib := range parent.Commands() {
			if sib == cmd {
				continue
			}
			if sib.Name() == alias {
				return
			}
			for _, a := range sib.Aliases {
				if a == alias {
					return
				}
			}
		}
	}
	cmd.Aliases = append(cmd.Aliases, alias)
}
