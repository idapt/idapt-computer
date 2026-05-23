//go:build windows

package cliconfig

func lockFileExclusive(path string) (release func(), err error) {
	return func() {}, nil
}
