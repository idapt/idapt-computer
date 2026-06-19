package commands

import (
	"fmt"
	"path/filepath"
	"strings"
)

var privilegedGroupDenylist = map[string]struct{}{
	"docker":          {}, // docker.sock → root
	"sudo":            {}, // password-prompt-gated root, still host-privilege
	"wheel":           {}, // su/sudo on RH-family
	"root":            {}, // gid 0
	"adm":             {}, // log read across the host
	"shadow":          {}, // /etc/shadow read
	"disk":            {}, // raw block device access
	"lxd":             {}, // container manager → root (like docker)
	"lxc":             {},
	"kvm":             {}, // /dev/kvm
	"libvirt":         {}, // VM manager → host
	"libvirt-qemu":    {},
	"systemd-journal": {}, // full journal read
	"sys":             {},
	"staff":           {},
}

func validatePrivilegedGroups(groups []string) error {
	for _, g := range groups {
		if _, blocked := privilegedGroupDenylist[strings.ToLower(strings.TrimSpace(g))]; blocked {
			return fmt.Errorf("group %q is privileged and not allowed for managed users", g)
		}
	}
	return nil
}

func validateLoginShell(shell string) error {
	if shell == "" {
		return nil
	}
	if !filepath.IsAbs(shell) {
		return fmt.Errorf("shell must be an absolute path")
	}
	if filepath.Clean(shell) != shell {
		return fmt.Errorf("shell must be a clean absolute path")
	}
	for _, r := range shell {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '/' || r == '.' || r == '_' || r == '-' || r == '+':
		default:
			return fmt.Errorf("shell contains disallowed character %q", r)
		}
	}
	return nil
}
