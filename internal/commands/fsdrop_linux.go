//go:build linux

package commands

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"
)

func runAsCredential(owner *runAsOwner, runAs string) (*syscall.Credential, error) {
	cred := &syscall.Credential{
		Uid: uint32(owner.UID),
		Gid: uint32(owner.GID),
	}
	if u, err := user.Lookup(runAs); err == nil {
		if gidStrs, gerr := u.GroupIds(); gerr == nil {
			groups := make([]uint32, 0, len(gidStrs))
			for _, gs := range gidStrs {
				if g, perr := strconv.Atoi(gs); perr == nil {
					groups = append(groups, uint32(g))
				}
			}
			cred.Groups = groups
		}
	}
	return cred, nil
}

func dropPrivilegesAvailable() bool {
	return os.Geteuid() == 0
}

func runFsOpAsUser(ctx context.Context, owner *runAsOwner, runAs string, spec fsOpSpec) (fsOpResult, error) {
	if !dropPrivilegesAvailable() {
		return runFsOpInProcess(spec)
	}
	self, err := os.Executable()
	if err != nil {
		return fsOpResult{}, fmt.Errorf("resolve daemon binary: %w", err)
	}
	cred, err := runAsCredential(owner, runAs)
	if err != nil {
		return fsOpResult{}, err
	}
	specJSON, err := encodeFsOpSpec(spec)
	if err != nil {
		return fsOpResult{}, err
	}

	cmd := exec.CommandContext(ctx, self, fsOpSubcommand, string(specJSON))
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: cred}
	cmd.Dir = "/"
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin"}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if stdout.Len() > 0 {
				if res, derr := decodeFsOpResult(stdout.Bytes()); derr == nil {
					return res, nil
				}
			}
			return fsOpResult{}, fmt.Errorf("fsop child failed: %s", stderr.String())
		}
		return fsOpResult{}, err
	}
	return decodeFsOpResult(stdout.Bytes())
}
