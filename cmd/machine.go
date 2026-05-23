package cmd

import (
	"github.com/spf13/cobra"
)

var machineRemoteCmd = &cobra.Command{
	Use:     "machine",
	Aliases: []string{"m"},
	Short:   "Manage remote machines",
	Annotations: map[string]string{
		"instructions": `# machine — instructions

Managed machines run on Idapt Cloud, AWS, or Hetzner. Lifecycle: ` + "`create`" + ` →
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
  truly want the machine gone.

## Delete is the one to think about — read this first

- Stops all processes immediately.
- Destroys the disk. The data is gone.
- Releases any reserved IPs / DNS records associated with the machine.
- Confirm with the user before deleting any machine you didn't
  explicitly create yourself.
- Use ` + "`--confirm`" + ` to skip the interactive prompt.

If you want the cheapest reversible pause, use ` + "`hibernate`" + `.
If you need running processes preserved on Idapt Cloud, use ` + "`sleep`" + `.
` + "`delete`" + ` is the only ZERO-cost state, but it is also the only
one that loses data.

## Daemon commands and tmux

- ` + "`machine create`" + ` — mint a one-time daemon pairing token and
  install command.
- ` + "`machine exec`" + ` — one-shot daemon command. Stateless.
- ` + "`machine tmux`" + ` — re-attachable session for long-running
  shells (build watchers, REPLs). Survives across calls.`,
	},
}

func init() {
	machineRemoteCmd.AddCommand(machineListCmd)
	machineRemoteCmd.AddCommand(machineCreateCmd)
	machineRemoteCmd.AddCommand(machineGetCmd)
	machineRemoteCmd.AddCommand(machineEditCmd)
	machineRemoteCmd.AddCommand(machineDeleteCmd)
	machineRemoteCmd.AddCommand(machineArchiveCmd)
	machineRemoteCmd.AddCommand(machineUnarchiveCmd)
	machineRemoteCmd.AddCommand(machineStartCmd)
	machineRemoteCmd.AddCommand(machineSleepCmd)
	machineRemoteCmd.AddCommand(machineStopCmd)
	machineRemoteCmd.AddCommand(machineHibernateCmd)
	machineRemoteCmd.AddCommand(machineTestCmd)

	machineRemoteCmd.AddCommand(machineExecCmd)
	machineRemoteCmd.AddCommand(machineTmuxCmd)
	machineRemoteCmd.AddCommand(machineFileCmd)
	machineRemoteCmd.AddCommand(machineDirCmd)
	machineRemoteCmd.AddCommand(machineFwCmd)
	machineRemoteCmd.AddCommand(machineUserCmd)
	machineRemoteCmd.AddCommand(machinePortCmd)
	machineRemoteCmd.AddCommand(machineEnvVarCmd)
}
