package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/idapt/idapt-cli/internal/cliconfig"
	"github.com/idapt/idapt-cli/internal/cmdutil"
)

func Run(ctx context.Context, f *cmdutil.Factory) (err error) {
	if f == nil {
		return errors.New("tui: factory is nil")
	}
	if err := cmdutil.RequireAuth(f); err != nil {
		return err
	}
	client, err := f.APIClient()
	if err != nil {
		return fmt.Errorf("tui: api client: %w", err)
	}
	cfgPath, _ := cliconfig.DefaultPath()
	m := NewModel(client, f.Config, cfgPath, f.Credentials, f.NoColor)

	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "\nidapt tui: panic — %v\n", r)
			if err == nil {
				err = fmt.Errorf("tui panic: %v", r)
			}
		}
	}()

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM)
	defer stop()

	var opts []tea.ProgramOption
	opts = append(opts,
		tea.WithContext(ctx),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	if in, ok := f.In.(io.Reader); ok && in != nil && in != os.Stdin {
		opts = append(opts, tea.WithInput(in))
	}
	if out, ok := f.Out.(io.Writer); ok && out != nil && out != os.Stdout {
		opts = append(opts, tea.WithOutput(out))
	}

	prog := tea.NewProgram(m, opts...)
	_, err = prog.Run()
	return err
}
