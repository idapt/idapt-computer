//go:build !linux

package commands

import "context"

func dropPrivilegesAvailable() bool { return false }

func runFsOpAsUser(_ context.Context, _ *runAsOwner, _ string, spec fsOpSpec) (fsOpResult, error) {
	return runFsOpInProcess(spec)
}
