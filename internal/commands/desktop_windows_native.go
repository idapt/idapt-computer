//go:build windows

package commands
import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32              = windows.NewLazySystemDLL("user32.dll")
	gdi32               = windows.NewLazySystemDLL("gdi32.dll")
	procSendInput       = user32.NewProc("SendInput")
	procGetCursorPos    = user32.NewProc("GetCursorPos")
	procGetSystemMetric = user32.NewProc("GetSystemMetrics")
	procGetDC           = user32.NewProc("GetDC")
	procReleaseDC       = user32.NewProc("ReleaseDC")
	procCreateCompatDC  = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatBM  = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject    = gdi32.NewProc("SelectObject")
	procBitBlt          = gdi32.NewProc("BitBlt")
	procGetDIBits       = gdi32.NewProc("GetDIBits")
	procDeleteObject    = gdi32.NewProc("DeleteObject")
	procDeleteDC        = gdi32.NewProc("DeleteDC")
)

const (
	smXVirtualScreen  = 76
	smYVirtualScreen  = 77
	smCXVirtualScreen = 78
	smCYVirtualScreen = 79

	inputMouse    = 0
	inputKeyboard = 1

	mouseeventfMove        = 0x0001
	mouseeventfLeftDown    = 0x0002
	mouseeventfLeftUp      = 0x0004
	mouseeventfRightDown   = 0x0008
	mouseeventfRightUp     = 0x0010
	mouseeventfMiddleDown  = 0x0020
	mouseeventfMiddleUp    = 0x0040
	mouseeventfWheel       = 0x0800
	mouseeventfHWheel      = 0x1000
	mouseeventfAbsolute    = 0x8000
	mouseeventfVirtualDesk = 0x4000
	wheelDelta             = 120

	keyeventfKeyUp   = 0x0002
	keyeventfUnicode = 0x0004

	srccopy = 0x00CC0020
	biRGB   = 0
)

type input struct {
	typ uint32
	_   uint32
	mi  mouseInput
}

type mouseInput struct {
	dx          int32
	dy          int32
	mouseData   uint32
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

type keybdInput struct {
	wVk         uint16
	wScan       uint16
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

type winNativeBackend struct {
	env map[string]string
}

func (b *winNativeBackend) Name() string { return "windows-native" }

func (b *winNativeBackend) Probe() (bool, string) {
	if user32.Load() != nil || gdi32.Load() != nil {
		return false, "user32/gdi32 unavailable"
	}
	return true, ""
}

func getSystemMetric(i int) int {
	r, _, _ := procGetSystemMetric.Call(uintptr(i))
	return int(int32(r))
}

func (b *winNativeBackend) virtualScreen() (x, y, w, h int) {
	return getSystemMetric(smXVirtualScreen), getSystemMetric(smYVirtualScreen),
		getSystemMetric(smCXVirtualScreen), getSystemMetric(smCYVirtualScreen)
}

func sendInputs(inputs []input) error {
	if len(inputs) == 0 {
		return nil
	}
	n, _, err := procSendInput.Call(
		uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])),
		unsafe.Sizeof(inputs[0]),
	)
	if int(n) != len(inputs) {
		return fmt.Errorf("SendInput sent %d/%d events: %v", n, len(inputs), err)
	}
	return nil
}

func mouseEvent(flags uint32, dx, dy int32, data uint32) input {
	return input{typ: inputMouse, mi: mouseInput{dx: dx, dy: dy, mouseData: data, dwFlags: flags}}
}

func keyInput(vk uint16, scan uint16, flags uint32) input {
	in := input{typ: inputKeyboard}
	*(*keybdInput)(unsafe.Pointer(&in.mi)) = keybdInput{wVk: vk, wScan: scan, dwFlags: flags}
	return in
}

func (b *winNativeBackend) moveAbsolute(x, y int) input {
	vx, vy, vw, vh := b.virtualScreen()
	if vw <= 0 {
		vw = 1
	}
	if vh <= 0 {
		vh = 1
	}
	nx := int32(((x - vx) * 65535) / vw)
	ny := int32(((y - vy) * 65535) / vh)
	return mouseEvent(mouseeventfMove|mouseeventfAbsolute|mouseeventfVirtualDesk, nx, ny, 0)
}

func (b *winNativeBackend) CursorPosition(_ context.Context) (int, int, error) {
	var pt struct{ X, Y int32 }
	r, _, err := procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	if r == 0 {
		return 0, 0, fmt.Errorf("GetCursorPos: %v", err)
	}
	return int(pt.X), int(pt.Y), nil
}

func (b *winNativeBackend) Capture(_ context.Context, displayID string) (DesktopCapture, error) {
	if displayID == "" {
		vx, vy, vw, vh := b.virtualScreen()
		pngBytes, err := captureRect(vx, vy, vw, vh)
		if err != nil {
			return DesktopCapture{}, err
		}
		return DesktopCapture{PNG: pngBytes, Width: vw, Height: vh, ActiveWindowTitle: foregroundWindowTitle()}, nil
	}

	disp, ok := findDisplayByID(enumDisplays(), displayID)
	if !ok {
		return DesktopCapture{}, fmt.Errorf("display %q not found", displayID)
	}
	pngBytes, err := captureRect(disp.OffsetX, disp.OffsetY, disp.Width, disp.Height)
	if err != nil {
		return DesktopCapture{}, err
	}
	return DesktopCapture{
		PNG:               pngBytes,
		Width:             disp.Width,
		Height:            disp.Height,
		OffsetX:           disp.OffsetX,
		OffsetY:           disp.OffsetY,
		DisplayID:         displayID,
		ActiveWindowTitle: foregroundWindowTitle(),
	}, nil
}

func captureRect(srcX, srcY, w, h int) ([]byte, error) {
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("bad capture rect %dx%d", w, h)
	}
	screenDC, _, _ := procGetDC.Call(0)
	if screenDC == 0 {
		return nil, fmt.Errorf("GetDC failed")
	}
	defer procReleaseDC.Call(0, screenDC)
	memDC, _, _ := procCreateCompatDC.Call(screenDC)
	if memDC == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC failed")
	}
	defer procDeleteDC.Call(memDC)
	bmp, _, _ := procCreateCompatBM.Call(screenDC, uintptr(w), uintptr(h))
	if bmp == 0 {
		return nil, fmt.Errorf("CreateCompatibleBitmap failed")
	}
	defer procDeleteObject.Call(bmp)
	procSelectObject.Call(memDC, bmp)
	if r, _, _ := procBitBlt.Call(memDC, 0, 0, uintptr(w), uintptr(h), screenDC, uintptr(srcX), uintptr(srcY), srccopy); r == 0 {
		return nil, fmt.Errorf("BitBlt failed")
	}

	type bitmapInfoHeader struct {
		Size          uint32
		Width         int32
		Height        int32
		Planes        uint16
		BitCount      uint16
		Compression   uint32
		SizeImage     uint32
		XPelsPerMeter int32
		YPelsPerMeter int32
		ClrUsed       uint32
		ClrImportant  uint32
	}
	var bi struct {
		Header bitmapInfoHeader
		_      [3]uint32 // color table padding
	}
	bi.Header.Size = uint32(unsafe.Sizeof(bi.Header))
	bi.Header.Width = int32(w)
	bi.Header.Height = -int32(h) // negative → top-down
	bi.Header.Planes = 1
	bi.Header.BitCount = 32
	bi.Header.Compression = biRGB

	buf := make([]byte, w*h*4)
	if r, _, _ := procGetDIBits.Call(memDC, bmp, 0, uintptr(h),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&bi)), 0); r == 0 {
		return nil, fmt.Errorf("GetDIBits failed")
	}

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i+3 < len(buf); i += 4 {
		img.Pix[i+0] = buf[i+2] // R (DIB is BGRA)
		img.Pix[i+1] = buf[i+1] // G
		img.Pix[i+2] = buf[i+0] // B
		img.Pix[i+3] = 0xff
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func (b *winNativeBackend) Input(_ context.Context, p DesktopPayload) error {
	hasCoord := p.X != nil && p.Y != nil
	moveTo := func(x, y int) error { return sendInputs([]input{b.moveAbsolute(x, y)}) }
	clickPair := func(down, up uint32, repeat int) error {
		var ev []input
		if hasCoord {
			ev = append(ev, b.moveAbsolute(coord(p.X), coord(p.Y)))
		}
		for i := 0; i < repeat; i++ {
			ev = append(ev, mouseEvent(down, 0, 0, 0), mouseEvent(up, 0, 0, 0))
		}
		return sendInputs(ev)
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
		return clickPair(mouseeventfLeftDown, mouseeventfLeftUp, 1)
	case DesktopActionRightClick:
		return clickPair(mouseeventfRightDown, mouseeventfRightUp, 1)
	case DesktopActionMiddleClick:
		return clickPair(mouseeventfMiddleDown, mouseeventfMiddleUp, 1)
	case DesktopActionDoubleClick:
		return clickPair(mouseeventfLeftDown, mouseeventfLeftUp, 2)
	case DesktopActionTripleClick:
		return clickPair(mouseeventfLeftDown, mouseeventfLeftUp, 3)
	case DesktopActionLeftMouseDown:
		ev := []input{}
		if hasCoord {
			ev = append(ev, b.moveAbsolute(coord(p.X), coord(p.Y)))
		}
		return sendInputs(append(ev, mouseEvent(mouseeventfLeftDown, 0, 0, 0)))
	case DesktopActionLeftMouseUp:
		ev := []input{}
		if hasCoord {
			ev = append(ev, b.moveAbsolute(coord(p.X), coord(p.Y)))
		}
		return sendInputs(append(ev, mouseEvent(mouseeventfLeftUp, 0, 0, 0)))
	case DesktopActionLeftClickDrag:
		return sendInputs([]input{
			b.moveAbsolute(coord(p.StartX), coord(p.StartY)),
			mouseEvent(mouseeventfLeftDown, 0, 0, 0),
			b.moveAbsolute(coord(p.X), coord(p.Y)),
			mouseEvent(mouseeventfLeftUp, 0, 0, 0),
		})
	case DesktopActionScroll:
		amount := p.ScrollAmount
		if amount <= 0 {
			amount = 3
		}
		var ev []input
		if hasCoord {
			ev = append(ev, b.moveAbsolute(coord(p.X), coord(p.Y)))
		}
		for i := 0; i < amount; i++ {
			var flags uint32
			var delta int32
			switch p.ScrollDirection {
			case "up":
				flags, delta = mouseeventfWheel, wheelDelta
			case "down":
				flags, delta = mouseeventfWheel, -wheelDelta
			case "right":
				flags, delta = mouseeventfHWheel, wheelDelta
			case "left":
				flags, delta = mouseeventfHWheel, -wheelDelta
			default:
				return fmt.Errorf("invalid scroll direction %q", p.ScrollDirection)
			}
			ev = append(ev, mouseEvent(flags, 0, 0, uint32(delta)))
		}
		return sendInputs(ev)
	case DesktopActionType:
		return b.typeText(p.Text)
	case DesktopActionKey:
		return b.pressChord(p.Text, 0)
	case DesktopActionHoldKey:
		return b.pressChord(p.Text, clampDesktopDuration(p.DurationMs))
	}
	return fmt.Errorf("unsupported desktop input action: %s", p.Action)
}

func (b *winNativeBackend) typeText(text string) error {
	if text == "" {
		return fmt.Errorf("type action requires text")
	}
	var ev []input
	for _, r := range text {
		if r > 0xFFFF {
			continue // skip non-BMP (would need surrogate pairs)
		}
		ev = append(ev,
			keyInput(0, uint16(r), keyeventfUnicode),
			keyInput(0, uint16(r), keyeventfUnicode|keyeventfKeyUp),
		)
	}
	return sendInputs(ev)
}

func (b *winNativeBackend) pressChord(chord string, d time.Duration) error {
	mods, key, hasKey, err := resolveVKChord(chord)
	if err != nil {
		return err
	}
	var down []input
	for _, m := range mods {
		down = append(down, keyInput(m, 0, 0))
	}
	if hasKey {
		down = append(down, keyInput(key, 0, 0))
	}
	if err := sendInputs(down); err != nil {
		return err
	}
	if d > 0 {
		time.Sleep(d)
	}
	var up []input
	if hasKey {
		up = append(up, keyInput(key, 0, keyeventfKeyUp))
	}
	for i := len(mods) - 1; i >= 0; i-- {
		up = append(up, keyInput(mods[i], 0, keyeventfKeyUp))
	}
	return sendInputs(up)
}
