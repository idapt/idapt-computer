//go:build !linux

package firewall

import (
	"fmt"
	"runtime"
)
func errUnsupportedFirewall() error {
	return fmt.Errorf("firewall management is only supported on Linux computers (this computer runs %s)", runtime.GOOS)
}

func ApplyRules(_ []Rule) error { return errUnsupportedFirewall() }

func ClearRules() error { return errUnsupportedFirewall() }

func ReadRules() ([]Rule, error) { return nil, errUnsupportedFirewall() }
