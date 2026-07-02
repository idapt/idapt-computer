//go:build windows

package cmd

import "os"

func notifyWinch(_ chan os.Signal) {}
