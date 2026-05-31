package main

import (
	"fmt"
	"os"

	"github.com/idapt/idapt-cli/cmd"
	"github.com/idapt/idapt-cli/internal/cmdutil"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(cmdutil.ExitCodeForError(err))
	}
}
