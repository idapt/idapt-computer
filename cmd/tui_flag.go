package cmd

import (
	"errors"
	"os"
	"strings"

	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/credential"
	"github.com/idapt/idapt-cli/internal/features"
	"github.com/spf13/cobra"
)

var errTUIDisabled = errors.New(
	"the interactive TUI (and `chat ask` / `-p` / `chat send --stream`) " +
		"is not available for your account.\n\n" +
		"The Interactive CLI TUI feature (flag `tui`) is currently off. " +
		"Contact support or your admin to request access, or use the " +
		"existing `idapt chat send <chat-id> \"<message>\"` sync path.",
)

func requireTUIFlag(cmd *cobra.Command, _ []string) error {
	f := cmdutil.FactoryFromCmd(cmd)
	if f == nil {
		return nil
	}
	if f.Features().IsEnabled(features.FlagTUI) {
		return nil
	}
	return errTUIDisabled
}

func readTUICachedAPIKey() string {
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

func isTUIEnabledFromCache() bool {
	cachePath, err := features.DefaultCachePath()
	if err != nil || cachePath == "" {
		return false
	}
	cached := features.LoadFromCache(cachePath, readTUICachedAPIKey())
	if cached == nil {
		return false
	}
	return cached.IsEnabled(features.FlagTUI)
}

func applyTUIVisibility() {
	hide := !isTUIEnabledFromCache()
	tuiCmd.Hidden = hide
	chatAskCmd.Hidden = hide
}
