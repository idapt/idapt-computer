//go:build linux

package commands
import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/randr"
	"github.com/jezek/xgb/xproto"
)
func (b *x11NativeBackend) ListDisplays(_ context.Context) ([]DesktopDisplay, error) {
	conn, err := b.connect()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return b.enumerateDisplays(conn), nil
}

func (b *x11NativeBackend) enumerateDisplays(conn *xgb.Conn) []DesktopDisplay {
	screen := xproto.Setup(conn).DefaultScreen(conn)
	displays := listX11Displays(conn, screen.Root)
	if len(displays) == 0 {
		displays = []DesktopDisplay{{
			ID:      "0",
			Name:    "primary",
			Width:   int(screen.WidthInPixels),
			Height:  int(screen.HeightInPixels),
			Primary: true,
			Scale:   1,
		}}
	}
	ensurePrimary(displays)
	return displays
}

func listX11Displays(conn *xgb.Conn, root xproto.Window) []DesktopDisplay {
	if err := randr.Init(conn); err != nil {
		return nil
	}
	res, err := randr.GetScreenResourcesCurrent(conn, root).Reply()
	if err != nil || res == nil {
		return nil
	}
	var primaryOut randr.Output
	if pr, err := randr.GetOutputPrimary(conn, root).Reply(); err == nil && pr != nil {
		primaryOut = pr.Output
	}

	var displays []DesktopDisplay
	for _, crtc := range res.Crtcs {
		ci, err := randr.GetCrtcInfo(conn, crtc, res.ConfigTimestamp).Reply()
		if err != nil || ci == nil {
			continue
		}
		if ci.Width == 0 || ci.Height == 0 || ci.Mode == 0 {
			continue
		}
		name := fmt.Sprintf("crtc-%d", crtc)
		primary := false
		if len(ci.Outputs) > 0 {
			if oi, err := randr.GetOutputInfo(conn, ci.Outputs[0], res.ConfigTimestamp).Reply(); err == nil && oi != nil && len(oi.Name) > 0 {
				name = string(oi.Name)
			}
			for _, o := range ci.Outputs {
				if o == primaryOut {
					primary = true
				}
			}
		}
		displays = append(displays, DesktopDisplay{
			ID:      strconv.FormatUint(uint64(crtc), 10),
			Name:    name,
			Width:   int(ci.Width),
			Height:  int(ci.Height),
			OffsetX: int(ci.X),
			OffsetY: int(ci.Y),
			Primary: primary,
			Scale:   1,
		})
	}
	return displays
}
func x11ActiveWindowTitle(conn *xgb.Conn, root xproto.Window) string {
	activeAtom := internAtom(conn, "_NET_ACTIVE_WINDOW")
	if activeAtom == 0 {
		return ""
	}
	prop, err := xproto.GetProperty(conn, false, root, activeAtom, xproto.AtomAny, 0, 1).Reply()
	if err != nil || prop == nil || len(prop.Value) < 4 {
		return ""
	}
	win := xproto.Window(xgb.Get32(prop.Value))
	if win == 0 {
		return ""
	}
	if utf8 := internAtom(conn, "UTF8_STRING"); utf8 != 0 {
		if nameAtom := internAtom(conn, "_NET_WM_NAME"); nameAtom != 0 {
			if title := readStringProperty(conn, win, nameAtom, utf8); title != "" {
				return title
			}
		}
	}
	return readStringProperty(conn, win, xproto.AtomWmName, xproto.AtomString)
}

func readStringProperty(conn *xgb.Conn, win xproto.Window, prop, typ xproto.Atom) string {
	reply, err := xproto.GetProperty(conn, false, win, prop, typ, 0, 256).Reply()
	if err != nil || reply == nil || len(reply.Value) == 0 {
		return ""
	}
	return strings.TrimRight(string(reply.Value), "\x00")
}

func internAtom(conn *xgb.Conn, name string) xproto.Atom {
	r, err := xproto.InternAtom(conn, true, uint16(len(name)), name).Reply()
	if err != nil || r == nil {
		return 0
	}
	return r.Atom
}
func (b *x11NativeBackend) Clipboard(ctx context.Context) (string, error) {
	switch {
	case binAvailable("xclip"):
		return runDesktopShell(ctx, b.cfg, b.runAs, displayPrefix(b.env)+"xclip -selection clipboard -o", b.env)
	case binAvailable("xsel"):
		return runDesktopShell(ctx, b.cfg, b.runAs, displayPrefix(b.env)+"xsel --clipboard --output", b.env)
	default:
		return "", errDesktopActionUnsupported
	}
}

func (b *x11NativeBackend) SetClipboard(ctx context.Context, text string) error {
	var tool string
	switch {
	case binAvailable("xclip"):
		tool = "xclip -selection clipboard -i"
	case binAvailable("xsel"):
		tool = "xsel --clipboard --input"
	default:
		return errDesktopActionUnsupported
	}
	inner := displayPrefix(b.env) + "printf %s " + shellQuote(text) + " | " + tool + " >/dev/null 2>&1"
	_, err := runDesktopShell(ctx, b.cfg, b.runAs, inner, b.env)
	return err
}
func (b *x11NativeBackend) ListWindows(ctx context.Context) ([]DesktopWindow, error) {
	if !binAvailable("wmctrl") {
		return nil, errDesktopActionUnsupported
	}
	out, err := runDesktopShell(ctx, b.cfg, b.runAs, displayPrefix(b.env)+"wmctrl -lpG", b.env)
	if err != nil {
		return nil, err
	}
	return parseWmctrlWindows(out), nil
}

func (b *x11NativeBackend) FocusWindow(ctx context.Context, windowID, titleMatch string) error {
	if !binAvailable("wmctrl") {
		return errDesktopActionUnsupported
	}
	var inner string
	if windowID != "" {
		inner = displayPrefix(b.env) + "wmctrl -i -a " + shellQuote(windowID)
	} else {
		inner = displayPrefix(b.env) + "wmctrl -a " + shellQuote(titleMatch)
	}
	_, err := runDesktopShell(ctx, b.cfg, b.runAs, inner, b.env)
	return err
}

func parseWmctrlWindows(out string) []DesktopWindow {
	atoi := func(s string) int { n, _ := strconv.Atoi(s); return n }
	var windows []DesktopWindow
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 8 {
			continue
		}
		title := ""
		if len(fields) > 8 {
			title = strings.Join(fields[8:], " ")
		}
		windows = append(windows, DesktopWindow{
			ID:     fields[0],
			Title:  title,
			X:      atoi(fields[3]),
			Y:      atoi(fields[4]),
			Width:  atoi(fields[5]),
			Height: atoi(fields[6]),
		})
	}
	return windows
}
