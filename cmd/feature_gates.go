package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/idapt/idapt-computer/internal/cmdutil"
	"github.com/idapt/idapt-computer/internal/credential"
	"github.com/idapt/idapt-computer/internal/features"
	"github.com/spf13/cobra"
)

type cliFeatureGate struct {
	flags       []string
	description string
}

func cliFeatureGateForCommand(cmd *cobra.Command) (cliFeatureGate, bool) {
	if cmd == nil {
		return cliFeatureGate{}, false
	}

	names := commandNameChain(cmd)
	if len(names) == 0 {
		return cliFeatureGate{}, false
	}

	if hasCommandPath(names, "drive", "mount") ||
		hasCommandPath(names, "drive", "unmount") ||
		hasCommandPath(names, "drive", "sync") {
		return cliFeatureGate{
			flags:       []string{features.FlagCLIFileMount},
			description: "Drive mount/sync",
		}, true
	}

	if hasCommandPathPrefix(names, "local-inference") ||
		hasCommandPathPrefix(names, "local") {
		return cliFeatureGate{
			flags: []string{
				features.FlagProviderEndpoints,
				features.FlagLocalInference,
			},
			description: "local inference",
		}, true
	}

	if hasCommandPathPrefix(names, "tunnel") ||
		hasCommandPathPrefix(names, "expose") ||
		hasCommandPathPrefix(names, "unexpose") ||
		hasCommandPathPrefix(names, "ssh") ||
		hasCommandPathPrefix(names, "ssh-proxy") {
		return cliFeatureGate{
			flags:       []string{features.FlagTunnels},
			description: "tunnels and SSH-over-tunnel",
		}, true
	}

	return cliFeatureGate{}, false
}

func enforceCLIFeatureGate(cmd *cobra.Command) error {
	gate, ok := cliFeatureGateForCommand(cmd)
	if !ok {
		return nil
	}
	f := cmdutil.FactoryFromCmd(cmd)
	if f == nil {
		return nil
	}
	resolved := f.Features()
	for _, flag := range gate.flags {
		if !resolved.IsEnabled(flag) {
			return fmt.Errorf(
				"the `idapt-computer %s` command is not available for your account.\n\n"+
					"%s is currently gated by feature flag %s.",
				cmd.CommandPath(),
				gate.description,
				flag,
			)
		}
	}
	return nil
}

func applyFeatureFlagVisibility(root *cobra.Command) {
	cachePath, _ := features.DefaultCachePath()
	apiKey := readCachedAPIKey()
	cached := features.LoadFromCache(cachePath, apiKey)

	flagsEnabled := func(required ...string) bool {
		if cached == nil {
			return false
		}
		for _, flag := range required {
			if !cached.IsEnabled(flag) {
				return false
			}
		}
		return true
	}

	hideDriveSync := !flagsEnabled(features.FlagCLIFileMount)
	setCommandHidden(root, hideDriveSync, "drive", "mount")
	setCommandHidden(root, hideDriveSync, "drive", "unmount")
	setCommandHidden(root, hideDriveSync, "drive", "sync")

	hideLocalInference := !flagsEnabled(
		features.FlagProviderEndpoints,
		features.FlagLocalInference,
	)
	setCommandHidden(root, hideLocalInference, "local-inference")

	hideTunnels := !flagsEnabled(features.FlagTunnels)
	setCommandHidden(root, hideTunnels, "expose")
	setCommandHidden(root, hideTunnels, "unexpose")
	setCommandHidden(root, hideTunnels, "tunnel")
	setCommandHidden(root, hideTunnels, "ssh")
	setCommandHidden(root, true, "ssh-proxy")
}

func commandNameChain(cmd *cobra.Command) []string {
	var reversed []string
	for c := cmd; c != nil; c = c.Parent() {
		if c.Name() != "" {
			reversed = append(reversed, c.Name())
		}
	}
	names := make([]string, 0, len(reversed))
	for i := len(reversed) - 1; i >= 0; i-- {
		names = append(names, reversed[i])
	}
	if len(names) > 0 && strings.Contains(names[0], "idapt") {
		return names[1:]
	}
	return names
}

func hasCommandPath(names []string, want ...string) bool {
	if len(names) != len(want) {
		return false
	}
	return hasCommandPathPrefix(names, want...)
}

func hasCommandPathPrefix(names []string, want ...string) bool {
	if len(names) < len(want) {
		return false
	}
	for i := range want {
		if names[i] != want[i] {
			return false
		}
	}
	return true
}

func setCommandHidden(root *cobra.Command, hidden bool, names ...string) {
	if c := findCommandByNames(root, names...); c != nil {
		c.Hidden = hidden
	}
}

func findCommandByNames(root *cobra.Command, names ...string) *cobra.Command {
	current := root
	for _, name := range names {
		var next *cobra.Command
		for _, child := range current.Commands() {
			if child.Name() == name {
				next = child
				break
			}
		}
		if next == nil {
			return nil
		}
		current = next
	}
	return current
}

func readCachedAPIKey() string {
	if k := os.Getenv("IDAPT_API_KEY"); k != "" && !strings.HasPrefix(k, "mk_") {
		return k
	}
	path, err := credential.DefaultPath()
	if err != nil {
		return ""
	}
	creds, err := credential.Load(path)
	if err != nil {
		return ""
	}
	return creds.APIKey
}
