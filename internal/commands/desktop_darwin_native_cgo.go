//go:build darwin && cgo

package commands
import "C"

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"strconv"
	"strings"
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

func darwinNativeBackend(cfg RunuserConfig, runAs string, env map[string]string) DesktopBackend {
	return &macNativeBackend{cfg: cfg, runAs: runAs, env: env}
}

type macNativeBackend struct {
	cfg   RunuserConfig
	runAs string
	env   map[string]string
}

func (b *macNativeBackend) Name() string { return "macos-native" }

func (b *macNativeBackend) Probe() (bool, string) {
	if _, err := exec.LookPath("screencapture"); err != nil {
		return false, "screencapture not found (expected on macOS)"
	}
	return true, ""
}

func (b *macNativeBackend) Capture(_ context.Context, displayID string) (DesktopCapture, error) {
	if displayID == "" {
		return b.captureWhole()
	}
	id, err := strconv.ParseUint(displayID, 10, 32)
	if err != nil {
		return DesktopCapture{}, fmt.Errorf("invalid displayId %q: %w", displayID, err)
	}
	pngBytes, w, h, err := captureDisplay(uint32(id))
	if err != nil {
		return DesktopCapture{}, err
	}
	var cx, cy, cw, ch C.int
	C.macDisplayBounds(C.uint32_t(id), &cx, &cy, &cw, &ch)
	return DesktopCapture{
		PNG:       pngBytes,
		Width:     w,
		Height:    h,
		OffsetX:   int(cx),
		OffsetY:   int(cy),
		DisplayID: displayID,
	}, nil
}

func (b *macNativeBackend) captureWhole() (DesktopCapture, error) {
	f, err := os.CreateTemp("", "idapt-desktop-*.png")
	if err != nil {
		return DesktopCapture{}, err
	}
	path := f.Name()
	_ = f.Close()
	defer func() { _ = os.Remove(path) }()
	if out, err := exec.Command("screencapture", "-x", "-t", "png", path).CombinedOutput(); err != nil {
		return DesktopCapture{}, fmt.Errorf("screencapture failed: %v: %s", err, string(out))
	}
	raw, err := os.ReadFile(path) // #nosec G304 -- daemon-controlled temp path
	if err != nil {
		return DesktopCapture{}, err
	}
	if len(raw) == 0 {
		return DesktopCapture{}, errors.New("screenshot produced no bytes")
	}
	w, h := pngDimensions(raw)
	return DesktopCapture{PNG: raw, Width: w, Height: h}, nil
}

func captureDisplay(id uint32) ([]byte, int, int, error) {
	var cw, ch, cstride C.int
	ptr := C.macCaptureDisplay(C.uint32_t(id), &cw, &ch, &cstride)
	if ptr == nil {
		return nil, 0, 0, errors.New("CGDisplayCreateImage failed — grant Screen Recording (TCC) to the app")
	}
	defer C.free(unsafe.Pointer(ptr))
	w, h, stride := int(cw), int(ch), int(cstride)
	if w <= 0 || h <= 0 || stride < w*4 {
		return nil, 0, 0, fmt.Errorf("bad capture geometry %dx%d stride %d", w, h, stride)
	}
	src := C.GoBytes(unsafe.Pointer(ptr), C.int(stride*h))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		row := y * stride
		for x := 0; x < w; x++ {
			i := row + x*4
			d := img.PixOffset(x, y)
			img.Pix[d+0] = src[i+2] // R (CG buffer is B,G,R,A)
			img.Pix[d+1] = src[i+1] // G
			img.Pix[d+2] = src[i+0] // B
			img.Pix[d+3] = 0xff
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, 0, 0, err
	}
	return buf.Bytes(), w, h, nil
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
	case DesktopActionHover:
		move(coord(p.X), coord(p.Y))
		time.Sleep(hoverDwell)
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
func (b *macNativeBackend) ListDisplays(_ context.Context) ([]DesktopDisplay, error) {
	const maxD = 16
	var ids [maxD]C.uint32_t
	n := uint32(C.macActiveDisplays(&ids[0], C.uint32_t(maxD)))
	var displays []DesktopDisplay
	for i := uint32(0); i < n && i < maxD; i++ {
		id := uint32(ids[i])
		var cx, cy, cw, ch C.int
		C.macDisplayBounds(C.uint32_t(id), &cx, &cy, &cw, &ch)
		displays = append(displays, DesktopDisplay{
			ID:      strconv.FormatUint(uint64(id), 10),
			Name:    fmt.Sprintf("display-%d", id),
			Width:   int(cw),
			Height:  int(ch),
			OffsetX: int(cx),
			OffsetY: int(cy),
			Primary: C.macDisplayIsMain(C.uint32_t(id)) != 0,
			Scale:   1,
		})
	}
	if len(displays) == 0 {
		shot, err := b.captureWhole()
		if err != nil {
			return nil, err
		}
		displays = []DesktopDisplay{{ID: "0", Name: "primary", Width: shot.Width, Height: shot.Height, Primary: true, Scale: 1}}
	}
	ensurePrimary(displays)
	return displays, nil
}

func (b *macNativeBackend) Clipboard(ctx context.Context) (string, error) {
	if !binAvailable("pbpaste") {
		return "", errDesktopActionUnsupported
	}
	return runDesktopShell(ctx, b.cfg, b.runAs, "pbpaste", b.env)
}

func (b *macNativeBackend) SetClipboard(ctx context.Context, text string) error {
	if !binAvailable("pbcopy") {
		return errDesktopActionUnsupported
	}
	inner := "printf %s " + shellQuote(text) + " | pbcopy"
	_, err := runDesktopShell(ctx, b.cfg, b.runAs, inner, b.env)
	return err
}

func (b *macNativeBackend) ListWindows(ctx context.Context) ([]DesktopWindow, error) {
	if !binAvailable("osascript") {
		return nil, errDesktopActionUnsupported
	}
	script := "tell application \"System Events\"\n" +
		"set out to \"\"\n" +
		"repeat with p in (processes where background only is false)\n" +
		"set out to out & name of p & linefeed\n" +
		"end repeat\n" +
		"return out\n" +
		"end tell"
	out, err := runDesktopShell(ctx, b.cfg, b.runAs, "osascript -e "+shellQuote(script), b.env)
	if err != nil {
		return nil, err
	}
	front, _ := runDesktopShell(ctx, b.cfg, b.runAs,
		"osascript -e "+shellQuote("tell application \"System Events\" to get name of first process whose frontmost is true"), b.env)
	front = strings.TrimSpace(front)

	var wins []DesktopWindow
	for _, line := range strings.Split(out, "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		wins = append(wins, DesktopWindow{ID: name, App: name, Title: name, Focused: name == front})
	}
	return wins, nil
}

func (b *macNativeBackend) FocusWindow(ctx context.Context, windowID, titleMatch string) error {
	if !binAvailable("osascript") {
		return errDesktopActionUnsupported
	}
	target := windowID
	if target == "" {
		target = titleMatch
	}
	script := "tell application " + strconv.Quote(target) + " to activate"
	_, err := runDesktopShell(ctx, b.cfg, b.runAs, "osascript -e "+shellQuote(script), b.env)
	return err
}
