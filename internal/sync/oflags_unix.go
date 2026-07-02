//go:build !windows

package sync

import "syscall"

const oNoFollow = syscall.O_NOFOLLOW
