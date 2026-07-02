//go:build linux || darwin

package cmd

import (
	"fmt"
	"strings"

	"github.com/idapt/idapt-computer/internal/cmdutil"
	"github.com/idapt/idapt-computer/internal/features"
	ifuse "github.com/idapt/idapt-computer/internal/fuse"
	"github.com/spf13/cobra"
)

var filesMountCmd = &cobra.Command{
	Use:     "mount <workspace> <mountpoint>",
	Short:   "Mount your Drive as a local filesystem",
	Long:    "Mount your idapt Drive (cloud files) as a FUSE filesystem. Files are synced via OCC.",
	Args:    cobra.ExactArgs(2),
	PreRunE: requireCLIFileMountFlag,
	RunE:    runFilesMount,
}

var filesUnmountCmd = &cobra.Command{
	Use:     "unmount <mountpoint>",
	Short:   "Unmount a Drive FUSE filesystem",
	Args:    cobra.ExactArgs(1),
	PreRunE: requireCLIFileMountFlag,
	RunE:    runFilesUnmount,
}

var mountManager *ifuse.MountManager

func init() {
	filesMountCmd.Flags().StringSlice("exclude", nil, "Patterns to exclude from sync (comma-separated)")
	filesMountCmd.Flags().String("cache-dir", "", "Directory for local file cache")
	filesMountCmd.Flags().Int64("cache-size", 10*1024*1024*1024, "Maximum cache size in bytes (default 10GB)")

	fileCmd.AddCommand(filesMountCmd)
	fileCmd.AddCommand(filesUnmountCmd)

	origFileHelp := fileCmd.HelpFunc()
	fileCmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		applyFeatureFlagVisibility(rootCmd)
		origFileHelp(c, args)
	})
}

func requireCLIFileMountFlag(cmd *cobra.Command, _ []string) error {
	f := cmdutil.FactoryFromCmd(cmd)
	if f == nil {
		return nil
	}
	if f.Features().IsEnabled(features.FlagCLIFileMount) {
		return nil
	}
	return fmt.Errorf(
		"the `idapt-computer drive %s` command is not available for your account.\n\n"+
			"This is an experimental feature gated behind the `cli-file-mount` flag.\n"+
			"Contact support or your admin to request access.",
		cmd.Name(),
	)
}

func applyMountVisibility() {
	applyFeatureFlagVisibility(rootCmd)
}

func shouldHideMountCommands(cachePath, apiKey string) bool {
	if cachePath == "" {
		return true
	}
	cached := features.LoadFromCache(cachePath, apiKey)
	if cached == nil {
		return true
	}
	return !cached.IsEnabled(features.FlagCLIFileMount)
}

func getMountManager() *ifuse.MountManager {
	if mountManager == nil {
		mountManager = ifuse.NewMountManager()
	}
	return mountManager
}

func runFilesMount(cmd *cobra.Command, args []string) error {
	workspace := args[0]
	mountPoint := args[1]

	f := cmdutil.FactoryFromCmd(cmd)
	client, err := f.APIClient()
	if err != nil {
		return fmt.Errorf("API client: %w", err)
	}

	workspaceID, err := resolveWorkspaceID(cmd, client, workspace)
	if err != nil {
		if strings.Contains(err.Error(), "Missing permission") || strings.Contains(err.Error(), "not accessible") {
			return fmt.Errorf("resolve workspace: %w\n\nHint: Your API key may lack 'workspace:read' permission.\nIf using sudo, pass --api-key explicitly or use 'sudo -E' to preserve IDAPT_API_KEY.", err)
		}
		return fmt.Errorf("resolve workspace: %w", err)
	}

	excludePatterns, _ := cmd.Flags().GetStringSlice("exclude")
	cacheDir, _ := cmd.Flags().GetString("cache-dir")
	cacheSize, _ := cmd.Flags().GetInt64("cache-size")

	cfg := ifuse.MountConfig{
		WorkspaceID:     workspaceID,
		MountPoint:      mountPoint,
		CacheDir:        cacheDir,
		MaxCacheSize:    cacheSize,
		ExcludePatterns: excludePatterns,
	}

	fuseClient := ifuse.NewFuseAPIClient(client)
	mm := getMountManager()

	if err := mm.Mount(cmd.Context(), cfg, fuseClient); err != nil {
		if strings.Contains(err.Error(), "Transport endpoint") || strings.Contains(err.Error(), "fusermount") {
			return fmt.Errorf("mount: %w\n\nHint: A stale FUSE mount may exist. Run: fusermount3 -u %s", err, mountPoint)
		}
		return fmt.Errorf("mount: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Mounted %s at %s\n", workspace, mountPoint)
	fmt.Fprintf(cmd.OutOrStdout(), "Press Ctrl+C to unmount.\n")

	<-cmd.Context().Done()

	if err := mm.Unmount(mountPoint); err != nil {
		return fmt.Errorf("unmount: %w", err)
	}

	return nil
}

func runFilesUnmount(cmd *cobra.Command, args []string) error {
	mountPoint := args[0]
	mm := getMountManager()

	if err := mm.Unmount(mountPoint); err != nil {
		return fmt.Errorf("unmount: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Unmounted %s\n", mountPoint)
	return nil
}
