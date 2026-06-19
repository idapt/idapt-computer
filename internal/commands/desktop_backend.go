package commands
import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"
	"os"
)

type DesktopBackend interface {
	Name() string
	Probe() (bool, string)
	Capture(ctx context.Context) (png []byte, width, height int, err error)
	Input(ctx context.Context, p DesktopPayload) error
	CursorPosition(ctx context.Context) (x, y int, err error)
}

func desktopBackendOverride() string {
	return os.Getenv("IDAPT_DESKTOP_BACKEND")
}

type shellBackend struct {
	cfg   RunuserConfig
	runAs string
	env   map[string]string // session env (DISPLAY / WAYLAND_DISPLAY filled in)
}

func (b *shellBackend) Name() string { return "shell" }

func (b *shellBackend) Probe() (bool, string) { return desktopProbe(b.env) }

func (b *shellBackend) Capture(_ context.Context) ([]byte, int, int, error) {
	f, err := os.CreateTemp("", "idapt-desktop-*.png")
	if err != nil {
		return nil, 0, 0, err
	}
	path := f.Name()
	_ = f.Close()
	defer func() { _ = os.Remove(path) }()

	if err := desktopCapture(context.Background(), b.cfg, b.runAs, b.env, path, DesktopPayload{Action: DesktopActionScreenshot}); err != nil {
		return nil, 0, 0, fmt.Errorf("capture failed: %w", err)
	}
	raw, err := os.ReadFile(path) // #nosec G304 -- daemon-controlled temp path
	if err != nil {
		return nil, 0, 0, fmt.Errorf("read screenshot: %w", err)
	}
	if len(raw) == 0 {
		return nil, 0, 0, errors.New("screenshot produced no bytes")
	}
	w, h := pngDimensions(raw)
	return raw, w, h, nil
}

func pngDimensions(raw []byte) (int, int) {
	if cfg, err := png.DecodeConfig(bytes.NewReader(raw)); err == nil {
		return cfg.Width, cfg.Height
	}
	return 0, 0
}

func (b *shellBackend) Input(ctx context.Context, p DesktopPayload) error {
	return desktopInput(ctx, b.cfg, b.runAs, b.env, p)
}

func (b *shellBackend) CursorPosition(ctx context.Context) (int, int, error) {
	return desktopCursorPosition(ctx, b.cfg, b.runAs, b.env)
}
