package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/idapt/idapt-computer/internal/config"
	"github.com/mattn/go-isatty"
)
const (
	InstallModeUser   = "user"
	InstallModeSystem = "system"
)

const installModePromptCopy = `How should the idapt daemon be installed?

  1) Install as current user (recommended — most users should choose this)
       The daemon runs as you. Local models, code execution and file tasks all
       work. It can only ever act as your user. You can upgrade to a system
       service later with ` + "`idapt-computer service elevate`" + `.

  2) Install as root (system service, Linux only)
       Installs a system service: the daemon can run commands as any user
       including root, and starts at boot. Choose this only on a non-sensitive
       machine where you accept the AI having root. Day-to-day commands still
       run as your user — root is used only when a command explicitly requests
       it.
`

func resolveInstallMode(
	goos, flagMode string,
	userFlag, systemFlag bool,
	interactive bool,
	prompt func() (string, error),
) (string, error) {
	requested, err := normalizeModeFlags(flagMode, userFlag, systemFlag)
	if err != nil {
		return "", err
	}

	if goos != "linux" {
		if requested == InstallModeSystem {
			return "", fmt.Errorf("system mode is Linux-only (this is %s) — omit --system / --mode system", goos)
		}
		return InstallModeUser, nil
	}

	if requested != "" {
		return requested, nil
	}

	if interactive && prompt != nil {
		choice, perr := prompt()
		if perr != nil {
			return "", perr
		}
		switch choice {
		case InstallModeUser, InstallModeSystem:
			return choice, nil
		default:
			return "", fmt.Errorf("unrecognised install-mode choice %q", choice)
		}
	}
	return InstallModeUser, nil
}

func normalizeModeFlags(flagMode string, userFlag, systemFlag bool) (string, error) {
	if userFlag && systemFlag {
		return "", fmt.Errorf("--user and --system are mutually exclusive")
	}
	flagMode = strings.ToLower(strings.TrimSpace(flagMode))
	switch flagMode {
	case "", InstallModeUser, InstallModeSystem:
	default:
		return "", fmt.Errorf("invalid --mode %q (want %q or %q)", flagMode, InstallModeUser, InstallModeSystem)
	}
	if flagMode != "" {
		if userFlag && flagMode != InstallModeUser {
			return "", fmt.Errorf("--user conflicts with --mode %s", flagMode)
		}
		if systemFlag && flagMode != InstallModeSystem {
			return "", fmt.Errorf("--system conflicts with --mode %s", flagMode)
		}
		return flagMode, nil
	}
	if userFlag {
		return InstallModeUser, nil
	}
	if systemFlag {
		return InstallModeSystem, nil
	}
	return "", nil
}

func systemDefaultUser(sudoUser, defaultUserFlag string) string {
	if u := normalizeDefaultUser(sudoUser); u != "" && u != "root" {
		return u
	}
	if u := normalizeDefaultUser(defaultUserFlag); u != "" {
		return u
	}
	return detectDefaultUser()
}

func resolveDefaultUserForMode(mode, defaultUserFlag string) string {
	if mode == InstallModeSystem {
		return systemDefaultUser(os.Getenv("SUDO_USER"), defaultUserFlag)
	}
	if u := normalizeDefaultUser(defaultUserFlag); u != "" {
		return u
	}
	return detectDefaultUser()
}

func stdinInteractive(assumeYes bool) bool {
	if assumeYes {
		return false
	}
	return isatty.IsTerminal(os.Stdin.Fd())
}

func promptInstallMode(in io.Reader, out io.Writer) (string, error) {
	fmt.Fprint(out, installModePromptCopy)
	choice, err := promptLine(in, out, "Choose [1] user / [2] system (default 1): ")
	if err != nil {
		return "", err
	}
	switch strings.TrimSpace(choice) {
	case "", "1", InstallModeUser:
		return InstallModeUser, nil
	case "2", InstallModeSystem:
		return InstallModeSystem, nil
	default:
		return "", fmt.Errorf("please answer 1 or 2 (got %q)", choice)
	}
}

func installModeForConfigPath(path string) string {
	if filepath.Clean(path) == filepath.Clean(config.SystemConfigPath) {
		return InstallModeSystem
	}
	return InstallModeUser
}

func installModeConfigPath(mode, explicitPath string) (string, error) {
	if explicitPath != "" {
		return explicitPath, nil
	}
	if mode == InstallModeSystem {
		return config.SystemConfigPath, nil
	}
	return config.EnsureUserConfigPath()
}

func systemModeSupported() bool {
	return runtime.GOOS == "linux"
}
