//go:build !windows

package cmd

func daemonSupervision() (mechanism string, supervised bool) {
	return "service-manager", true
}
