//go:build darwin && cgo

package commands
import "C"

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
	"unsafe"
)

const (
	cgLeftMouseDown     = 1
	cgLeftMouseUp       = 2
	cgRightMouseDown    = 3
	cgRightMouseUp      = 4
	cgOtherMouseDown    = 25
	cgOtherMouseUp      = 26
	cgMouseButtonLeft   = 0
	cgMouseButtonRight  = 1
	cgMouseButtonCenter = 2
)

func darwinNativeBackend(_ map[string]string) DesktopBackend { return &macNativeBackend{} }

type macNativeBackend struct{}

func (b *macNativeBackend) Name() string { return "macos-native" }

func (b *macNativeBackend) Probe() (bool, string) {
	if _, err := exec.LookPath("screencapture"); err != nil {
		return false, "screencapture not found (expected on macOS)"
	}
	return true, ""
}

func (b *macNativeBackend) Capture(_ context.Context) ([]byte, int, int, error) {
	f, err := os.CreateTemp("", "idapt-desktop-*.png")
	if err != nil {
		return nil, 0, 0, err
	}
	path := f.Name()
	_ = f.Close()
	defer func() { _ = os.Remove(path) }()
	if out, err := exec.Command("screencapture", "-x", "-t", "png", path).CombinedOutput(); err != nil {
		return nil, 0, 0, fmt.Errorf("screencapture failed: %v: %s", err, string(out))
	}
	raw, err := os.ReadFile(path) // #nosec G304 -- daemon-controlled temp path
	if err != nil {
		return nil, 0, 0, err
	}
	if len(raw) == 0 {
		return nil, 0, 0, errors.New("screenshot produced no bytes")
	}
	w, h := pngDimensions(raw)
	return raw, w, h, nil
}

func (b *macNativeBackend) CursorPosition(_ context.Context) (int, int, error) {
	var x, y C.double
	C.macCursorPos(&x, &y)
	return int(x), int(y), nil
}

func (b *macNativeBackend) Input(_ context.Context, p DesktopPayload) error {
	hasCoord := p.X != nil && p.Y != nil
	move := func(x, y int) { C.macMouseMove(C.double(x), C.double(y)) }
	button := func(x, y, typ, btn int) { C.macMouseButton(C.double(x), C.double(y), C.int(typ), C.int(btn)) }
	clickAt := func(downType, upType, btn, repeat int) error {
		x, y := coord(p.X), coord(p.Y)
		if hasCoord {
			move(x, y)
		}
		for i := 0; i < repeat; i++ {
			button(x, y, downType, btn)
			button(x, y, upType, btn)
		}
		return nil
	}

	switch p.Action {
	case DesktopActionMouseMove:
		move(coord(p.X), coord(p.Y))
		return nil
	case DesktopActionLeftClick:
		return clickAt(cgLeftMouseDown, cgLeftMouseUp, cgMouseButtonLeft, 1)
	case DesktopActionRightClick:
		return clickAt(cgRightMouseDown, cgRightMouseUp, cgMouseButtonRight, 1)
	case DesktopActionMiddleClick:
		return clickAt(cgOtherMouseDown, cgOtherMouseUp, cgMouseButtonCenter, 1)
	case DesktopActionDoubleClick:
		return clickAt(cgLeftMouseDown, cgLeftMouseUp, cgMouseButtonLeft, 2)
	case DesktopActionTripleClick:
		return clickAt(cgLeftMouseDown, cgLeftMouseUp, cgMouseButtonLeft, 3)
	case DesktopActionLeftMouseDown:
		x, y := coord(p.X), coord(p.Y)
		if hasCoord {
			move(x, y)
		}
		button(x, y, cgLeftMouseDown, cgMouseButtonLeft)
		return nil
	case DesktopActionLeftMouseUp:
		x, y := coord(p.X), coord(p.Y)
		if hasCoord {
			move(x, y)
		}
		button(x, y, cgLeftMouseUp, cgMouseButtonLeft)
		return nil
	case DesktopActionLeftClickDrag:
		button(coord(p.StartX), coord(p.StartY), cgLeftMouseDown, cgMouseButtonLeft)
		C.macMouseButton(C.double(coord(p.X)), C.double(coord(p.Y)), C.int(6), C.int(cgMouseButtonLeft)) // LeftMouseDragged
		button(coord(p.X), coord(p.Y), cgLeftMouseUp, cgMouseButtonLeft)
		return nil
	case DesktopActionScroll:
		amount := p.ScrollAmount
		if amount <= 0 {
			amount = 3
		}
		if hasCoord {
			move(coord(p.X), coord(p.Y))
		}
		for i := 0; i < amount; i++ {
			switch p.ScrollDirection {
			case "up":
				C.macScroll(C.int32_t(3), 0)
			case "down":
				C.macScroll(C.int32_t(-3), 0)
			case "right":
				C.macScroll(0, C.int32_t(3))
			case "left":
				C.macScroll(0, C.int32_t(-3))
			default:
				return fmt.Errorf("invalid scroll direction %q", p.ScrollDirection)
			}
		}
		return nil
	case DesktopActionType:
		if p.Text == "" {
			return fmt.Errorf("type action requires text")
		}
		ctext := C.CString(p.Text)
		defer C.free(unsafe.Pointer(ctext))
		C.macTypeString(ctext)
		return nil
	case DesktopActionKey:
		return b.pressChord(p.Text, 0)
	case DesktopActionHoldKey:
		return b.pressChord(p.Text, clampDesktopDuration(p.DurationMs))
	}
	return fmt.Errorf("unsupported desktop input action: %s", p.Action)
}

func (b *macNativeBackend) pressChord(chord string, d time.Duration) error {
	flags, keycode, hasKey, err := resolveMacChord(chord)
	if err != nil {
		return err
	}
	if !hasKey {
		return nil
	}
	C.macKey(C.uint16_t(keycode), C.uint64_t(flags), C.int(1))
	if d > 0 {
		time.Sleep(d)
	}
	C.macKey(C.uint16_t(keycode), C.uint64_t(flags), C.int(0))
	return nil
}
