//go:build linux

package commands
import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	vk "github.com/bnema/wayland-virtual-input-go/virtual_keyboard"
	vp "github.com/bnema/wayland-virtual-input-go/virtual_pointer"
)

var waylandEnvMu sync.Mutex

type waylandNativeBackend struct {
	env map[string]string // session env (WAYLAND_DISPLAY + XDG_RUNTIME_DIR)
	cfg   RunuserConfig
	runAs string
}

func (b *waylandNativeBackend) Name() string { return "wayland-native" }

func (b *waylandNativeBackend) withWaylandEnv(fn func() error) error {
	waylandEnvMu.Lock()
	defer waylandEnvMu.Unlock()
	for _, k := range []string{"WAYLAND_DISPLAY", "XDG_RUNTIME_DIR"} {
		if v := b.env[k]; v != "" {
			old, had := os.LookupEnv(k)
			_ = os.Setenv(k, v)
			defer func(k, old string, had bool) {
				if had {
					_ = os.Setenv(k, old)
				} else {
					_ = os.Unsetenv(k)
				}
			}(k, old, had)
		}
	}
	return fn()
}

func (b *waylandNativeBackend) Probe() (bool, string) {
	if _, err := exec.LookPath("grim"); err != nil {
		return false, "grim not found — install grim for Wayland screenshots (or use an X11 session)"
	}
	err := b.withWaylandEnv(func() error {
		mgr, err := vp.NewVirtualPointerManager(context.Background())
		if err != nil {
			return err
		}
		return mgr.Close()
	})
	if err != nil {
		return false, fmt.Sprintf(
			"Wayland compositor does not expose the virtual-pointer protocol (%v) — computer-use needs a wlroots compositor (Sway/Hyprland/river/labwc); GNOME/KDE Wayland are not supported, use an X11 session",
			err)
	}
	return true, ""
}

func (b *waylandNativeBackend) Capture(_ context.Context, displayID string) (DesktopCapture, error) {
	f, err := os.CreateTemp("", "idapt-desktop-*.png")
	if err != nil {
		return DesktopCapture{}, err
	}
	path := f.Name()
	_ = f.Close()
	defer func() { _ = os.Remove(path) }()

	args := []string{}
	if displayID != "" {
		args = append(args, "-o", displayID)
	}
	args = append(args, path)
	cmd := exec.Command("grim", args...)
	cmd.Env = append(os.Environ(),
		"WAYLAND_DISPLAY="+b.env["WAYLAND_DISPLAY"],
		"XDG_RUNTIME_DIR="+b.env["XDG_RUNTIME_DIR"],
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return DesktopCapture{}, fmt.Errorf("grim capture failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
	raw, err := os.ReadFile(path) // #nosec G304 -- daemon-controlled temp path
	if err != nil {
		return DesktopCapture{}, fmt.Errorf("read screenshot: %w", err)
	}
	w, h := pngDimensions(raw)
	return DesktopCapture{PNG: raw, Width: w, Height: h, DisplayID: displayID}, nil
}

func (b *waylandNativeBackend) CursorPosition(_ context.Context) (int, int, error) {
	return 0, 0, fmt.Errorf("cursor-position is not available on Wayland — take a screenshot instead")
}

func (b *waylandNativeBackend) Input(_ context.Context, p DesktopPayload) error {
	switch p.Action {
	case DesktopActionType, DesktopActionKey, DesktopActionHoldKey:
		return b.keyboardInput(p)
	default:
		return b.pointerInput(p)
	}
}

func (b *waylandNativeBackend) pointerInput(p DesktopPayload) error {
	w, h := b.outputExtent()
	return b.withWaylandEnv(func() error {
		mgr, err := vp.NewVirtualPointerManager(context.Background())
		if err != nil {
			return err
		}
		defer mgr.Close()
		ptr, err := mgr.CreatePointer()
		if err != nil {
			return err
		}
		defer ptr.Close()

		now := time.Now()
		moveTo := func(x, y int) error {
			if err := ptr.MotionAbsolute(now, uint32(x), uint32(y), uint32(w), uint32(h)); err != nil {
				return err
			}
			return ptr.Frame()
		}
		hasCoord := p.X != nil && p.Y != nil
		press := func(btn uint32) error {
			if err := ptr.Button(now, btn, vp.ButtonStatePressed); err != nil {
				return err
			}
			return ptr.Frame()
		}
		release := func(btn uint32) error {
			if err := ptr.Button(now, btn, vp.ButtonStateReleased); err != nil {
				return err
			}
			return ptr.Frame()
		}
		click := func(btn uint32, repeat int) error {
			if hasCoord {
				if err := moveTo(coord(p.X), coord(p.Y)); err != nil {
					return err
				}
			}
			for i := 0; i < repeat; i++ {
				if err := press(btn); err != nil {
					return err
				}
				if err := release(btn); err != nil {
					return err
				}
			}
			return nil
		}

		switch p.Action {
		case DesktopActionMouseMove:
			return moveTo(coord(p.X), coord(p.Y))
		case DesktopActionHover:
			if err := moveTo(coord(p.X), coord(p.Y)); err != nil {
				return err
			}
			time.Sleep(hoverDwell)
			return nil
		case DesktopActionLeftClick:
			return click(vp.BTN_LEFT, 1)
		case DesktopActionRightClick:
			return click(vp.BTN_RIGHT, 1)
		case DesktopActionMiddleClick:
			return click(vp.BTN_MIDDLE, 1)
		case DesktopActionDoubleClick:
			return click(vp.BTN_LEFT, 2)
		case DesktopActionTripleClick:
			return click(vp.BTN_LEFT, 3)
		case DesktopActionLeftMouseDown:
			if hasCoord {
				if err := moveTo(coord(p.X), coord(p.Y)); err != nil {
					return err
				}
			}
			return press(vp.BTN_LEFT)
		case DesktopActionLeftMouseUp:
			if hasCoord {
				if err := moveTo(coord(p.X), coord(p.Y)); err != nil {
					return err
				}
			}
			return release(vp.BTN_LEFT)
		case DesktopActionLeftClickDrag:
			if err := moveTo(coord(p.StartX), coord(p.StartY)); err != nil {
				return err
			}
			if err := press(vp.BTN_LEFT); err != nil {
				return err
			}
			if err := moveTo(coord(p.X), coord(p.Y)); err != nil {
				return err
			}
			return release(vp.BTN_LEFT)
		case DesktopActionScroll:
			amount := p.ScrollAmount
			if amount <= 0 {
				amount = 3
			}
			if hasCoord {
				if err := moveTo(coord(p.X), coord(p.Y)); err != nil {
					return err
				}
			}
			step := 15.0
			var dv, dh float64
			switch p.ScrollDirection {
			case "up":
				dv = -step
			case "down":
				dv = step
			case "left":
				dh = -step
			case "right":
				dh = step
			default:
				return fmt.Errorf("invalid scroll direction %q", p.ScrollDirection)
			}
			for i := 0; i < amount; i++ {
				if dv != 0 {
					if err := ptr.ScrollVertical(dv); err != nil {
						return err
					}
				}
				if dh != 0 {
					if err := ptr.ScrollHorizontal(dh); err != nil {
						return err
					}
				}
			}
			return nil
		}
		return fmt.Errorf("unsupported pointer action: %s", p.Action)
	})
}

func (b *waylandNativeBackend) keyboardInput(p DesktopPayload) error {
	return b.withWaylandEnv(func() error {
		mgr, err := vk.NewVirtualKeyboardManager(context.Background())
		if err != nil {
			return err
		}
		defer mgr.Close()
		kb, err := mgr.CreateKeyboard()
		if err != nil {
			return err
		}
		defer kb.Close()
		time.Sleep(20 * time.Millisecond)

		switch p.Action {
		case DesktopActionType:
			if p.Text == "" {
				return fmt.Errorf("type action requires text")
			}
			return typeWaylandText(kb, p.Text)

		case DesktopActionKey:
			if strings.TrimSpace(p.Text) == "" {
				return fmt.Errorf("key action requires text (the key chord)")
			}
			mods, key, hasKey, err := resolveEvdevChord(p.Text)
			if err != nil {
				return err
			}
			return pressEvdevChord(kb, mods, key, hasKey)

		case DesktopActionHoldKey:
			if strings.TrimSpace(p.Text) == "" {
				return fmt.Errorf("hold-key action requires text (the key chord)")
			}
			mods, key, hasKey, err := resolveEvdevChord(p.Text)
			if err != nil {
				return err
			}
			for _, m := range mods {
				if err := kb.PressKey(m); err != nil {
					return err
				}
			}
			if hasKey {
				if key.shift {
					if err := kb.PressKey(evKeyLeftShift); err != nil {
						return err
					}
				}
				if err := kb.PressKey(key.code); err != nil {
					return err
				}
			}
			d := clampDesktopDuration(p.DurationMs)
			if d <= 0 {
				d = time.Second
			}
			time.Sleep(d)
			if hasKey {
				if err := kb.ReleaseKey(key.code); err != nil {
					return err
				}
				if key.shift {
					if err := kb.ReleaseKey(evKeyLeftShift); err != nil {
						return err
					}
				}
			}
			for i := len(mods) - 1; i >= 0; i-- {
				if err := kb.ReleaseKey(mods[i]); err != nil {
					return err
				}
			}
			return nil
		}
		return fmt.Errorf("unsupported keyboard action: %s", p.Action)
	})
}

func pressEvdevChord(kb *vk.VirtualKeyboard, mods []uint32, key evdevKey, hasKey bool) error {
	for _, m := range mods {
		if err := kb.PressKey(m); err != nil {
			return err
		}
	}
	if hasKey {
		if key.shift {
			if err := kb.PressKey(evKeyLeftShift); err != nil {
				return err
			}
		}
		if err := kb.TypeKey(key.code); err != nil {
			return err
		}
		if key.shift {
			if err := kb.ReleaseKey(evKeyLeftShift); err != nil {
				return err
			}
		}
	}
	for i := len(mods) - 1; i >= 0; i-- {
		if err := kb.ReleaseKey(mods[i]); err != nil {
			return err
		}
	}
	return nil
}

func typeWaylandText(kb *vk.VirtualKeyboard, text string) error {
	for _, r := range text {
		key, ok := evdevForRune(r)
		if !ok {
			continue // skip glyphs not on the US layout
		}
		if key.shift {
			if err := kb.PressKey(evKeyLeftShift); err != nil {
				return err
			}
		}
		if err := kb.TypeKey(key.code); err != nil {
			return err
		}
		if key.shift {
			if err := kb.ReleaseKey(evKeyLeftShift); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *waylandNativeBackend) outputExtent() (int, int) {
	shot, err := b.Capture(context.Background(), "")
	if err == nil && shot.Width > 0 && shot.Height > 0 {
		return shot.Width, shot.Height
	}
	return 1920, 1080
}
func (b *waylandNativeBackend) ListDisplays(_ context.Context) ([]DesktopDisplay, error) {
	w, h := b.outputExtent()
	return []DesktopDisplay{{
		ID:      "0",
		Name:    "primary",
		Width:   w,
		Height:  h,
		Primary: true,
		Scale:   1,
	}}, nil
}

func (b *waylandNativeBackend) Clipboard(ctx context.Context) (string, error) {
	if !binAvailable("wl-paste") {
		return "", errDesktopActionUnsupported
	}
	return runDesktopShell(ctx, b.cfg, b.runAs, waylandPrefix(b.env)+"wl-paste -n", b.env)
}

func (b *waylandNativeBackend) SetClipboard(ctx context.Context, text string) error {
	if !binAvailable("wl-copy") {
		return errDesktopActionUnsupported
	}
	inner := waylandPrefix(b.env) + "printf %s " + shellQuote(text) + " | wl-copy >/dev/null 2>&1"
	_, err := runDesktopShell(ctx, b.cfg, b.runAs, inner, b.env)
	return err
}

func (b *waylandNativeBackend) ListWindows(_ context.Context) ([]DesktopWindow, error) {
	return nil, errDesktopActionUnsupported
}

func (b *waylandNativeBackend) FocusWindow(_ context.Context, _, _ string) error {
	return errDesktopActionUnsupported
}
