package cmd

import "github.com/spf13/cobra"

const (
	groupCore       = "core"
	groupCompute    = "compute"
	groupWorkspace  = "workspace"
	groupAutomation = "automation"
	groupAccount    = "account"
	groupSystem     = "system"
)

func registerCommandGroups(root *cobra.Command) {
	root.AddGroup(
		&cobra.Group{ID: groupCore, Title: "Core Commands:"},
		&cobra.Group{ID: groupCompute, Title: "Compute Commands:"},
		&cobra.Group{ID: groupWorkspace, Title: "Workspace & Files Commands:"},
		&cobra.Group{ID: groupAutomation, Title: "Automation Commands:"},
		&cobra.Group{ID: groupAccount, Title: "Account & Config Commands:"},
		&cobra.Group{ID: groupSystem, Title: "System Commands:"},
	)
	root.SetHelpCommandGroupID(groupSystem)

	assign := func(c *cobra.Command, id string) {
		if c != nil {
			c.GroupID = id
		}
	}

	assign(chatCmd, groupCore)
	assign(agentCmd, groupCore)
	assign(modelCmd, groupCore)
	assign(mediaCmd, groupCore)

	assign(computerRemoteCmd, groupCompute)
	assign(computerUseCmd, groupCompute)
	assign(execCmd, groupCompute)
	assign(webCmd, groupCompute)

	assign(workspaceCmd, groupWorkspace)
	assign(fileCmd, groupWorkspace) // registers as `drive`
	assign(secretCmd, groupWorkspace)
	assign(shareCmd, groupWorkspace)
	assign(hubCmd, groupWorkspace)

	assign(scriptCmd, groupAutomation)
	assign(triggerCmd, groupAutomation)
	assign(hookCmd, groupAutomation)
	assign(subagentCmd, groupAutomation)

	assign(authCmd, groupAccount)
	assign(configCliCmd, groupAccount)
	assign(subscriptionCmd, groupAccount)
	assign(apikeyCmd, groupAccount)
	assign(profileCmd, groupAccount)
	assign(settingsCmd, groupAccount)
	assign(notificationCmd, groupAccount)

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
