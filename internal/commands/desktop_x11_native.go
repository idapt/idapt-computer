//go:build linux

package commands
import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"
)

const (
	xEvKeyPress      = 2
	xEvKeyRelease    = 3
	xEvButtonPress   = 4
	xEvButtonRelease = 5
	xEvMotionNotify  = 6
)

var x11ConnectMu sync.Mutex

type x11NativeBackend struct {
	display    string
	xauthority string
}

func (b *x11NativeBackend) Name() string { return "x11-native" }

func (b *x11NativeBackend) connect() (*xgb.Conn, error) {
	x11ConnectMu.Lock()
	defer x11ConnectMu.Unlock()
	if b.xauthority != "" {
		old, had := os.LookupEnv("XAUTHORITY")
		_ = os.Setenv("XAUTHORITY", b.xauthority)
		defer func() {
			if had {
				_ = os.Setenv("XAUTHORITY", old)
			} else {
				_ = os.Unsetenv("XAUTHORITY")
			}
		}()
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(50 * time.Millisecond)
		}
		conn, err := xgb.NewConnDisplay(b.display)
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("x11 connect (DISPLAY=%q): %w", b.display, lastErr)
}

func (b *x11NativeBackend) Probe() (bool, string) {
	conn, err := b.connect()
	if err != nil {
		return false, fmt.Sprintf(
			"cannot reach X server on DISPLAY=%q (%v) — ensure an X session is running and the daemon has its XAUTHORITY cookie",
			b.display, err)
	}
	conn.Close()
	return true, ""
}

func (b *x11NativeBackend) Capture(_ context.Context) ([]byte, int, int, error) {
	conn, err := b.connect()
	if err != nil {
		return nil, 0, 0, err
	}
	defer conn.Close()

	screen := xproto.Setup(conn).DefaultScreen(conn)
	w, h := int(screen.WidthInPixels), int(screen.HeightInPixels)
	reply, err := xproto.GetImage(
		conn, xproto.ImageFormatZPixmap, xproto.Drawable(screen.Root),
		0, 0, screen.WidthInPixels, screen.HeightInPixels, 0xffffffff,
	).Reply()
	if err != nil {
		return nil, 0, 0, fmt.Errorf("x11 GetImage: %w", err)
	}

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	src := reply.Data
	n := w * h * 4
	if len(src) < n {
		n = len(src) - (len(src) % 4)
	}
	for i := 0; i < n; i += 4 {
		img.Pix[i+0] = src[i+2] // R
		img.Pix[i+1] = src[i+1] // G
		img.Pix[i+2] = src[i+0] // B
		img.Pix[i+3] = 0xff
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, 0, 0, fmt.Errorf("png encode: %w", err)
	}
	return buf.Bytes(), w, h, nil
}

func (b *x11NativeBackend) CursorPosition(_ context.Context) (int, int, error) {
	conn, err := b.connect()
	if err != nil {
		return 0, 0, err
	}
	defer conn.Close()
	root := xproto.Setup(conn).DefaultScreen(conn).Root
	reply, err := xproto.QueryPointer(conn, root).Reply()
	if err != nil {
		return 0, 0, fmt.Errorf("x11 QueryPointer: %w", err)
	}
	return int(reply.RootX), int(reply.RootY), nil
}

func (b *x11NativeBackend) Input(_ context.Context, p DesktopPayload) error {
	conn, err := b.connect()
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := xtest.Init(conn); err != nil {
		return fmt.Errorf("x11 XTEST init: %w", err)
	}
	root := xproto.Setup(conn).DefaultScreen(conn).Root
	km, err := loadX11Keymap(conn)
	if err != nil {
		return err
	}
	hasCoord := p.X != nil && p.Y != nil

	switch p.Action {
	case DesktopActionMouseMove:
		return fakeMotion(conn, root, coord(p.X), coord(p.Y))

	case DesktopActionLeftClick:
		return clickWithMods(conn, root, km, 1, 1, hasCoord, p)
	case DesktopActionRightClick:
		return clickWithMods(conn, root, km, 3, 1, hasCoord, p)
	case DesktopActionMiddleClick:
		return clickWithMods(conn, root, km, 2, 1, hasCoord, p)
	case DesktopActionDoubleClick:
		return clickWithMods(conn, root, km, 1, 2, hasCoord, p)
	case DesktopActionTripleClick:
		return clickWithMods(conn, root, km, 1, 3, hasCoord, p)

	case DesktopActionLeftMouseDown:
		if hasCoord {
			if err := fakeMotion(conn, root, coord(p.X), coord(p.Y)); err != nil {
				return err
			}
		}
		return fakeButton(conn, root, 1, true)
	case DesktopActionLeftMouseUp:
		if hasCoord {
			if err := fakeMotion(conn, root, coord(p.X), coord(p.Y)); err != nil {
				return err
			}
		}
		return fakeButton(conn, root, 1, false)

	case DesktopActionLeftClickDrag:
		if err := fakeMotion(conn, root, coord(p.StartX), coord(p.StartY)); err != nil {
			return err
		}
		if err := fakeButton(conn, root, 1, true); err != nil {
			return err
		}
		if err := fakeMotion(conn, root, coord(p.X), coord(p.Y)); err != nil {
			return err
		}
		return fakeButton(conn, root, 1, false)

	case DesktopActionScroll:
		btn, err := x11ScrollButton(p.ScrollDirection)
		if err != nil {
			return err
		}
		amount := p.ScrollAmount
		if amount <= 0 {
			amount = 3
		}
		if hasCoord {
			if err := fakeMotion(conn, root, coord(p.X), coord(p.Y)); err != nil {
				return err
			}
		}
		for i := 0; i < amount; i++ {
			if err := fakeButton(conn, root, btn, true); err != nil {
				return err
			}
			if err := fakeButton(conn, root, btn, false); err != nil {
				return err
			}
		}
		return nil

	case DesktopActionKey:
		if strings.TrimSpace(p.Text) == "" {
			return fmt.Errorf("key action requires text (the key chord)")
		}
		return pressChord(conn, root, km, p.Text)

	case DesktopActionType:
		if p.Text == "" {
			return fmt.Errorf("type action requires text")
		}
		return typeText(conn, root, km, p.Text)

	case DesktopActionHoldKey:
		if strings.TrimSpace(p.Text) == "" {
			return fmt.Errorf("hold-key action requires text (the key chord)")
		}
		return holdChord(conn, root, km, p.Text, clampDesktopDuration(p.DurationMs))
	}
	return fmt.Errorf("unsupported desktop input action: %s", p.Action)
}
func fakeMotion(conn *xgb.Conn, root xproto.Window, x, y int) error {
	return xtest.FakeInputChecked(conn, xEvMotionNotify, 0, 0, root, int16(x), int16(y), 0).Check()
}

func fakeButton(conn *xgb.Conn, root xproto.Window, button byte, press bool) error {
	t := byte(xEvButtonRelease)
	if press {
		t = xEvButtonPress
	}
	return xtest.FakeInputChecked(conn, t, button, 0, root, 0, 0, 0).Check()
}

func fakeKey(conn *xgb.Conn, root xproto.Window, kc xproto.Keycode, press bool) error {
	t := byte(xEvKeyRelease)
	if press {
		t = xEvKeyPress
	}
	return xtest.FakeInputChecked(conn, t, byte(kc), 0, root, 0, 0, 0).Check()
}

func x11ScrollButton(dir string) (byte, error) {
	switch dir {
	case "up":
		return 4, nil
	case "down":
		return 5, nil
	case "left":
		return 6, nil
	case "right":
		return 7, nil
	default:
		return 0, fmt.Errorf("invalid scroll direction %q", dir)
	}
}

func clickWithMods(conn *xgb.Conn, root xproto.Window, km *x11Keymap, button byte, repeat int, hasCoord bool, p DesktopPayload) error {
	if hasCoord {
		if err := fakeMotion(conn, root, coord(p.X), coord(p.Y)); err != nil {
			return err
		}
	}
	var mods []xproto.Keycode
	if strings.TrimSpace(p.Text) != "" {
		var err error
		mods, err = km.resolveModifiers(p.Text)
		if err != nil {
			return err
		}
	}
	for _, kc := range mods {
		if err := fakeKey(conn, root, kc, true); err != nil {
			return err
		}
	}
	for i := 0; i < repeat; i++ {
		if err := fakeButton(conn, root, button, true); err != nil {
			return err
		}
		if err := fakeButton(conn, root, button, false); err != nil {
			return err
		}
	}
	for i := len(mods) - 1; i >= 0; i-- {
		if err := fakeKey(conn, root, mods[i], false); err != nil {
			return err
		}
	}
	return nil
}

func pressChord(conn *xgb.Conn, root xproto.Window, km *x11Keymap, chord string) error {
	mods, key, err := km.resolveChord(chord)
	if err != nil {
		return err
	}
	for _, kc := range mods {
		if err := fakeKey(conn, root, kc, true); err != nil {
			return err
		}
	}
	if key.set {
		if key.shift {
			if err := fakeKey(conn, root, km.shift, true); err != nil {
				return err
			}
		}
		if err := fakeKey(conn, root, key.code, true); err != nil {
			return err
		}
		if err := fakeKey(conn, root, key.code, false); err != nil {
			return err
		}
		if key.shift {
			if err := fakeKey(conn, root, km.shift, false); err != nil {
				return err
			}
		}
	}
	for i := len(mods) - 1; i >= 0; i-- {
		if err := fakeKey(conn, root, mods[i], false); err != nil {
			return err
		}
	}
	return nil
}

func holdChord(conn *xgb.Conn, root xproto.Window, km *x11Keymap, chord string, d time.Duration) error {
	mods, key, err := km.resolveChord(chord)
	if err != nil {
		return err
	}
	down := func() error {
		for _, kc := range mods {
			if err := fakeKey(conn, root, kc, true); err != nil {
				return err
			}
		}
		if key.set {
			return fakeKey(conn, root, key.code, true)
		}
		return nil
	}
	up := func() error {
		if key.set {
			if err := fakeKey(conn, root, key.code, false); err != nil {
				return err
			}
		}
		for i := len(mods) - 1; i >= 0; i-- {
			if err := fakeKey(conn, root, mods[i], false); err != nil {
				return err
			}
		}
		return nil
	}
	if err := down(); err != nil {
		return err
	}
	if d <= 0 {
		d = time.Second
	}
	time.Sleep(d)
	return up()
}

func typeText(conn *xgb.Conn, root xproto.Window, km *x11Keymap, text string) error {
	for _, r := range text {
		loc, ok := km.lookupRune(r)
		if !ok {
			continue
		}
		if loc.shift {
			if err := fakeKey(conn, root, km.shift, true); err != nil {
				return err
			}
		}
		if err := fakeKey(conn, root, loc.code, true); err != nil {
			return err
		}
		if err := fakeKey(conn, root, loc.code, false); err != nil {
			return err
		}
		if loc.shift {
			if err := fakeKey(conn, root, km.shift, false); err != nil {
				return err
			}
		}
	}
	return nil
}
