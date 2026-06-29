package commands
import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"
	"os"
)

type DesktopCapture struct {
	PNG    []byte
	Width  int
	Height int
	OffsetX int
	OffsetY int
	DisplayID string
	ActiveWindowTitle string
}

var errDesktopActionUnsupported = errors.New(
	"action unsupported on this desktop backend — use the native backend (X11/Wayland/Windows/macOS)",
)

type DesktopBackend interface {
	Name() string
	Probe() (bool, string)
	Capture(ctx context.Context, displayID string) (DesktopCapture, error)
	Input(ctx context.Context, p DesktopPayload) error
	CursorPosition(ctx context.Context) (x, y int, err error)
	ListDisplays(ctx context.Context) ([]DesktopDisplay, error)
	Clipboard(ctx context.Context) (string, error)
	SetClipboard(ctx context.Context, text string) error
	ListWindows(ctx context.Context) ([]DesktopWindow, error)
	FocusWindow(ctx context.Context, windowID, titleMatch string) error
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

func (b *shellBackend) Capture(_ context.Context, _ string) (DesktopCapture, error) {
	f, err := os.CreateTemp("", "idapt-desktop-*.png")
	if err != nil {
		return DesktopCapture{}, err
	}
	path := f.Name()
	_ = f.Close()
	defer func() { _ = os.Remove(path) }()

	if err := desktopCapture(context.Background(), b.cfg, b.runAs, b.env, path, DesktopPayload{Action: DesktopActionScreenshot}); err != nil {
		return DesktopCapture{}, fmt.Errorf("capture failed: %w", err)
	}
	raw, err := os.ReadFile(path) // #nosec G304 -- daemon-controlled temp path
	if err != nil {
		return DesktopCapture{}, fmt.Errorf("read screenshot: %w", err)
	}
	if len(raw) == 0 {
		return DesktopCapture{}, errors.New("screenshot produced no bytes")
	}
	w, h := pngDimensions(raw)
	return DesktopCapture{PNG: raw, Width: w, Height: h}, nil
}

func (b *shellBackend) ListDisplays(ctx context.Context) ([]DesktopDisplay, error) {
	cap, err := b.Capture(ctx, "")
	if err != nil {
		return nil, err
	}
	return []DesktopDisplay{{
		ID:      "primary",
		Name:    "primary",
		Width:   cap.Width,
		Height:  cap.Height,
		OffsetX: 0,
		OffsetY: 0,
		Primary: true,
		Scale:   1,
	}}, nil
}

func (b *shellBackend) Clipboard(_ context.Context) (string, error) {
	return "", errDesktopActionUnsupported
}

func (b *shellBackend) SetClipboard(_ context.Context, _ string) error {
	return errDesktopActionUnsupported
}

func (b *shellBackend) ListWindows(_ context.Context) ([]DesktopWindow, error) {
	return nil, errDesktopActionUnsupported
}

func (b *shellBackend) FocusWindow(_ context.Context, _, _ string) error {
	return errDesktopActionUnsupported
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
