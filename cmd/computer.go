package cmd

import (
	"github.com/spf13/cobra"
)

var computerRemoteCmd = &cobra.Command{
	Use:     "computer",
	Aliases: []string{"m"},
	Short:   "Manage remote computers",
	Annotations: map[string]string{
		"instructions": `# computer — instructions

Cloud computers run on Idapt Cloud, AWS, or Hetzner. Lifecycle: ` + "`create`" + ` →
` + "`start`" + ` / ` + "`sleep`" + ` / ` + "`stop`" + ` / ` + "`hibernate`" + ` →
` + "`delete`" + ` (zero-cost; data gone).

## State you usually want

- ` + "`sleep`" + ` — microVM only. Preserves running programs and memory for
  fast resume.
- ` + "`stop`" + ` — power off provisioned compute. Disk is preserved and
  ` + "`start`" + ` powers it back on.
- ` + "`hibernate`" + ` — unprovision compute and store restore state. Disk
  state is preserved, but running programs are not guaranteed to survive.
- ` + "`archive`" + ` — cosmetic only; hide from default list without
  changing billing or state. Reversible via ` + "`unarchive`" + `.
- ` + "`delete`" + ` — zero-cost but **irreversible**. Disk is
  destroyed; reserved IPs / DNS records released. Only use when you
  truly want the computer gone.

## Delete is the one to think about — read this first

- Stops all processes immediately.
- Destroys the disk. The data is gone.
- Releases any reserved IPs / DNS records associated with the computer.
- Confirm with the user before deleting any computer you didn't
  explicitly create yourself.
- Use ` + "`--confirm`" + ` to skip the interactive prompt.

If you want the cheapest reversible pause, use ` + "`hibernate`" + `.
If you need running processes preserved on Idapt Cloud, use ` + "`sleep`" + `.
` + "`delete`" + ` is the only ZERO-cost state, but it is also the only
one that loses data.

## Daemon commands and tmux

- ` + "`computer create`" + ` — mint a one-time daemon pairing token and
  install command.
- ` + "`computer exec`" + ` — one-shot daemon command. Stateless.
- ` + "`computer tmux`" + ` — re-attachable session for long-running
  shells (build watchers, REPLs). Survives across calls.`,
	},
}

func init() {
	computerRemoteCmd.AddCommand(computerListCmd)
	computerRemoteCmd.AddCommand(computerCreateCmd)
	computerRemoteCmd.AddCommand(computerGetCmd)
	computerRemoteCmd.AddCommand(computerEditCmd)
	computerRemoteCmd.AddCommand(computerDeleteCmd)
	computerRemoteCmd.AddCommand(computerArchiveCmd)
	computerRemoteCmd.AddCommand(computerUnarchiveCmd)
	computerRemoteCmd.AddCommand(computerStartCmd)
	computerRemoteCmd.AddCommand(computerSleepCmd)
	computerRemoteCmd.AddCommand(computerStopCmd)
	computerRemoteCmd.AddCommand(computerHibernateCmd)
	computerRemoteCmd.AddCommand(computerTestCmd)

	computerRemoteCmd.AddCommand(computerExecCmd)
	computerRemoteCmd.AddCommand(computerTmuxCmd)
	computerRemoteCmd.AddCommand(computerFileCmd)
	computerRemoteCmd.AddCommand(computerDirCmd)
	computerRemoteCmd.AddCommand(computerFwCmd)
	computerRemoteCmd.AddCommand(computerUserCmd)
	computerRemoteCmd.AddCommand(computerPortCmd)
	computerRemoteCmd.AddCommand(computerEnvVarCmd)
}
