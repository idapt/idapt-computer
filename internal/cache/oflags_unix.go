//go:build !windows

package cache

import "syscall"

const oNoFollow = syscall.O_NOFOLLOW
