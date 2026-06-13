package cmd

import "github.com/spf13/cobra"

const (
	groupWorkspace = "workspace"
	groupAccount   = "account"
	groupSystem    = "system"
)

func registerCommandGroups(root *cobra.Command) {
	root.AddGroup(
		&cobra.Group{ID: groupWorkspace, Title: "Workspace & Files Commands:"},
		&cobra.Group{ID: groupAccount, Title: "Account & Config Commands:"},
		&cobra.Group{ID: groupSystem, Title: "System Commands:"},
	)
	root.SetHelpCommandGroupID(groupSystem)

	assign := func(c *cobra.Command, id string) {
		if c != nil {
			c.GroupID = id
		}
	}

	assign(fileCmd, groupWorkspace) // registers as `drive`

	assign(authCmd, groupAccount)
	assign(configCliCmd, groupAccount)

	assign(updateCmd, groupSystem)
	assign(versionCmd, groupSystem)
	assign(selftestCmd, groupSystem)
	assign(uninstallCmd, groupSystem)
	assign(upCmd, groupSystem)
	assign(downCmd, groupSystem)
	assign(logoutCmd, groupSystem)
	assign(pairCmd, groupSystem)
	assign(desktopCmd, groupSystem)
	assign(serviceCmd, groupSystem)
	assign(instructionsCmd, groupSystem)
	assign(completionCmd, groupSystem)
	assign(openCmd, groupSystem)
}
