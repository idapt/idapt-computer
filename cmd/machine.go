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

Managed machines run on EC2 / Hetzner. Lifecycle: ` + "`create`" + ` →
` + "`start`" + ` / ` + "`stop`" + ` (preserves disk; still billed) →
` + "`terminate`" + ` (zero-cost; data gone).

## State you usually want

- ` + "`stop`" + ` — pause cleanly. Disk preserved. ` + "`start`" + `
  brings it back exactly where it was. Use this for "pause the
  expensive thing" — NOT ` + "`terminate`" + `.
- ` + "`terminate`" + ` — zero-cost but **irreversible**. Disk is
  destroyed; reserved IPs / DNS records released. Only use when you
  truly want the machine gone.

## Terminate is the one to think about — read this first

- Stops all processes immediately.
- Destroys the disk. The data is gone.
- Releases any reserved IPs / DNS records associated with the machine.
- Confirm with the user before terminating any machine you didn't
  explicitly create yourself.
- Use ` + "`--confirm`" + ` to skip the interactive prompt.

If you want a no-cost pause, ` + "`stop`" + ` — disk preserved (still
billed) and you can ` + "`start`" + ` again later. ` + "`terminate`" + `
is the only ZERO-cost state, but it's also the only one that loses
data.

## SSH and tmux

- ` + "`machine exec`" + ` — one-shot SSH command. Stateless.
- ` + "`machine tmux`" + ` — re-attachable session for long-running
  shells (build watchers, REPLs). Survives across calls.`,
	},
}

func init() {
	machineRemoteCmd.AddCommand(machineListCmd)
	machineRemoteCmd.AddCommand(machineCreateCmd)
	machineRemoteCmd.AddCommand(machineGetCmd)
	machineRemoteCmd.AddCommand(machineEditCmd)
	machineRemoteCmd.AddCommand(machineStartCmd)
	machineRemoteCmd.AddCommand(machineStopCmd)
	machineRemoteCmd.AddCommand(machineTerminateCmd)
	machineRemoteCmd.AddCommand(machineActivityCmd)
	machineRemoteCmd.AddCommand(machineExecCmd)
	machineRemoteCmd.AddCommand(machineTmuxCmd)
	machineRemoteCmd.AddCommand(machineFileCmd)
	machineRemoteCmd.AddCommand(machineDirCmd)
	machineRemoteCmd.AddCommand(machineFwCmd)
	machineRemoteCmd.AddCommand(machineUserCmd)
	machineRemoteCmd.AddCommand(machinePortCmd)
	machineRemoteCmd.AddCommand(machineEnvVarCmd)
}
