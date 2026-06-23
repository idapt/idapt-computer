//go:build !windows

package commands

import "syscall"

const oNoFollow = syscall.O_NOFOLLOW
