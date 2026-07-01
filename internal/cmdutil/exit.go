package cmdutil

import (
	"errors"
	"os"

	"github.com/idapt/idapt-computer/internal/api"
	"github.com/idapt/idapt-computer/internal/output"
)

func ExitWithError(f *Factory, err error) {
	if err == nil {
		return
	}

	output.WriteError(f.Format, f.ErrOut, err)

	os.Exit(ExitCodeForError(err))
}

func ExitCodeForError(err error) int {
	if err == nil {
		return api.ExitOK
	}
	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ExitCode()
	}
	return api.ExitError
}
