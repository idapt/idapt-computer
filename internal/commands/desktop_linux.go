//go:build linux

package commands
import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type linuxSession int

const (
	sessionX11 linuxSession = iota
	sessionWayland
)

func detectLinuxSession(env map[string]string) linuxSession {
	if env["WAYLAND_DISPLAY"] != "" && env["DISPLAY"] == "" {
		return sessionWayland
	}
	if env["DISPLAY"] != "" {
		return sessionX11
	}
	if os.Getenv("WAYLAND_DISPLAY") != "" && os.Getenv("DISPLAY") == "" {
		return sessionWayland
	}
	return sessionX11
}

func desktopProbe(env map[string]string) (bool, string) {
	if detectLinuxSession(env) == sessionWayland {
		var missing []string
		if _, err := exec.LookPath("grim"); err != nil {
			missing = append(missing, "grim")
		}
		_, wtypeErr := exec.LookPath("wtype")
		_, ydotoolErr := exec.LookPath("ydotool")
		if wtypeErr != nil && ydotoolErr != nil {
			missing = append(missing, "wtype or ydotool")
		}
		if len(missing) > 0 {
			return false, "Wayland desktop tooling missing: " + strings.Join(missing, ", ") +
				" — install grim (+ wtype for keyboard, ydotool for mouse), or use an X11 (Xorg) session"
		}
		return true, ""
	}
	var missing []string
	for _, bin := range []string{"xdotool", "scrot"} {
		if _, err := exec.LookPath(bin); err != nil {
			missing = append(missing, bin)
		}
	}
	if len(missing) > 0 {
		return false, "X11 desktop tooling missing: " + strings.Join(missing, ", ") +
			" — install a desktop environment (xvfb/xfce + xdotool + scrot) first"
	}
	return true, ""
}

func selectDesktopBackend(cfg RunuserConfig, runAs string, rawEnv map[string]string) DesktopBackend {
	sessEnv := desktopSessionEnv(rawEnv)
	shell := &shellBackend{cfg: cfg, runAs: runAs, env: sessEnv}
	if desktopBackendOverride() == "shell" {
		return shell
	}
	if detectLinuxSession(rawEnv) == sessionX11 {
		native := &x11NativeBackend{
			display:    sessEnv["DISPLAY"],
			xauthority: sessEnv["XAUTHORITY"],
		}
		if ok, _ := native.Probe(); ok {
			return native
		}
	} else {
		native := &waylandNativeBackend{env: sessEnv}
		if ok, _ := native.Probe(); ok {
			return native
		}
	}
	return shell
}

func desktopSessionEnv(env map[string]string) map[string]string {
	out := make(map[string]string, len(env)+2)
	for k, v := range env {
		out[k] = v
	}

	if detectLinuxSession(env) == sessionWayland {
		if out["WAYLAND_DISPLAY"] == "" {
			if w := os.Getenv("WAYLAND_DISPLAY"); w != "" {
				out["WAYLAND_DISPLAY"] = w
			}
		}
		if out["XDG_RUNTIME_DIR"] == "" {
			if r := os.Getenv("XDG_RUNTIME_DIR"); r != "" {
				out["XDG_RUNTIME_DIR"] = r
			}
		}
		return out
	}

	if out["DISPLAY"] == "" {
		if d := os.Getenv("IDAPT_DISPLAY"); d != "" {
			out["DISPLAY"] = d
		} else {
			out["DISPLAY"] = ":0"
		}
	}
	if out["XAUTHORITY"] == "" {
		if xa := os.Getenv("XAUTHORITY"); xa != "" {
			out["XAUTHORITY"] = xa
		}
	}
	return out
}

func displayPrefix(env map[string]string) string {
	var b strings.Builder
	if d := env["DISPLAY"]; d != "" {
		b.WriteString("DISPLAY=" + shellQuote(d) + " ")
	}
	if xa := env["XAUTHORITY"]; xa != "" {
		b.WriteString("XAUTHORITY=" + shellQuote(xa) + " ")
	}
	return b.String()
}

func waylandPrefix(env map[string]string) string {
	var b strings.Builder
	if w := env["WAYLAND_DISPLAY"]; w != "" {
		b.WriteString("WAYLAND_DISPLAY=" + shellQuote(w) + " ")
	}
	if r := env["XDG_RUNTIME_DIR"]; r != "" {
		b.WriteString("XDG_RUNTIME_DIR=" + shellQuote(r) + " ")
	}
	return b.String()
}

func desktopCapture(
	ctx context.Context,
	cfg RunuserConfig,
	runAs string,
	env map[string]string,
	pngPath string,
	_ DesktopPayload,
) error {
	var inner string
	if detectLinuxSession(env) == sessionWayland {
		inner = waylandPrefix(env) + "grim " + shellQuote(pngPath)
	} else {
		inner = displayPrefix(env) + "scrot --overwrite " + shellQuote(pngPath)
	}
	_, err := runDesktopShell(ctx, cfg, runAs, inner, env)
	return err
}

func desktopCursorPosition(
	ctx context.Context,
	cfg RunuserConfig,
	runAs string,
	env map[string]string,
) (int, int, error) {
	if detectLinuxSession(env) == sessionWayland {
		return 0, 0, fmt.Errorf("cursor-position is not available on Wayland — take a screenshot instead")
	}
	inner := displayPrefix(env) + `eval "$(xdotool getmouselocation --shell)"; printf '%s %s' "$X" "$Y"`
	out, err := runDesktopShell(ctx, cfg, runAs, inner, env)
	if err != nil {
		return 0, 0, err
	}
	return parseTwoInts(out)
}

func desktopInput(
	ctx context.Context,
	cfg RunuserConfig,
	runAs string,
	env map[string]string,
	p DesktopPayload,
) error {
	if detectLinuxSession(env) == sessionWayland {
		inner, err := buildWaylandInput(p)
		if err != nil {
			return err
		}
		_, err = runDesktopShell(ctx, cfg, runAs, waylandPrefix(env)+inner, env)
		return err
	}
	inner, err := buildLinuxInput(p)
	if err != nil {
		return err
	}
	_, err = runDesktopShell(ctx, cfg, runAs, displayPrefix(env)+inner, env)
	return err
}

func linuxScrollButton(dir string) (int, error) {
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

func buildLinuxInput(p DesktopPayload) (string, error) {
	moveTo := func() string {
		return fmt.Sprintf("mousemove --sync %d %d ", coord(p.X), coord(p.Y))
	}
	hasCoord := p.X != nil && p.Y != nil

	withMods := func(body string) string {
		mods := strings.TrimSpace(p.Text)
		if mods == "" {
			return "xdotool " + body
		}
		return fmt.Sprintf("xdotool keydown -- %s %s keyup -- %s",
			shellQuote(mods), body, shellQuote(mods))
	}

	switch p.Action {
	case DesktopActionMouseMove:
		return fmt.Sprintf("xdotool mousemove --sync %d %d", coord(p.X), coord(p.Y)), nil

	case DesktopActionLeftClick:
		return withMods(maybeMove(hasCoord, moveTo) + "click 1"), nil
	case DesktopActionRightClick:
		return withMods(maybeMove(hasCoord, moveTo) + "click 3"), nil
	case DesktopActionMiddleClick:
		return withMods(maybeMove(hasCoord, moveTo) + "click 2"), nil
	case DesktopActionDoubleClick:
		return withMods(maybeMove(hasCoord, moveTo) + "click --repeat 2 --delay 10 1"), nil
	case DesktopActionTripleClick:
		return withMods(maybeMove(hasCoord, moveTo) + "click --repeat 3 --delay 10 1"), nil

	case DesktopActionLeftClickDrag:
		return fmt.Sprintf(
			"xdotool mousemove --sync %d %d mousedown 1 mousemove --sync %d %d mouseup 1",
			coord(p.StartX), coord(p.StartY), coord(p.X), coord(p.Y),
		), nil

	case DesktopActionLeftMouseDown:
		return "xdotool " + maybeMove(hasCoord, moveTo) + "mousedown 1", nil
	case DesktopActionLeftMouseUp:
		return "xdotool " + maybeMove(hasCoord, moveTo) + "mouseup 1", nil

	case DesktopActionScroll:
		btn, err := linuxScrollButton(p.ScrollDirection)
		if err != nil {
			return "", err
		}
		amount := p.ScrollAmount
		if amount <= 0 {
			amount = 3
		}
		body := maybeMove(hasCoord, moveTo) +
			fmt.Sprintf("click --repeat %d %d", amount, btn)
		return withMods(body), nil

	case DesktopActionKey:
		if strings.TrimSpace(p.Text) == "" {
			return "", fmt.Errorf("key action requires text (the key chord)")
		}
		return "xdotool key -- " + shellQuote(p.Text), nil

	case DesktopActionType:
		if p.Text == "" {
			return "", fmt.Errorf("type action requires text")
		}
		return "xdotool type --delay 12 -- " + shellQuote(p.Text), nil

	case DesktopActionHoldKey:
		if strings.TrimSpace(p.Text) == "" {
			return "", fmt.Errorf("hold-key action requires text (the key chord)")
		}
		seconds := float64(clampDesktopDuration(p.DurationMs)) / float64(1e9)
		if seconds <= 0 {
			seconds = 1
		}
		return fmt.Sprintf("xdotool keydown -- %s sleep %g keyup -- %s",
			shellQuote(p.Text), seconds, shellQuote(p.Text)), nil
	}

	return "", fmt.Errorf("unsupported desktop input action: %s", p.Action)
}

func maybeMove(has bool, moveTo func() string) string {
	if has {
		return moveTo()
	}
	return ""
}

func buildWaylandInput(p DesktopPayload) (string, error) {
	hasCoord := p.X != nil && p.Y != nil
	move := func() string {
		return fmt.Sprintf("ydotool mousemove --absolute -x %d -y %d", coord(p.X), coord(p.Y))
	}
	click := func(code string) string {
		if hasCoord {
			return move() + "; ydotool click " + code
		}
		return "ydotool click " + code
	}

	switch p.Action {
	case DesktopActionType:
		if p.Text == "" {
			return "", fmt.Errorf("type action requires text")
		}
		return "printf %s " + shellQuote(p.Text) + " | wtype -", nil

	case DesktopActionKey:
		if strings.TrimSpace(p.Text) == "" {
			return "", fmt.Errorf("key action requires text (the key chord)")
		}
		return buildWtypeChord(p.Text), nil

	case DesktopActionMouseMove:
		return move(), nil
	case DesktopActionLeftClick:
		return click("0xC0"), nil
	case DesktopActionRightClick:
		return click("0xC1"), nil
	case DesktopActionMiddleClick:
		return click("0xC2"), nil
	case DesktopActionDoubleClick:
		return click("0xC0 0xC0"), nil
	case DesktopActionTripleClick:
		return click("0xC0 0xC0 0xC0"), nil
	case DesktopActionLeftMouseDown:
		return click("0x40"), nil // press, no release
	case DesktopActionLeftMouseUp:
		return click("0x80"), nil // release
	case DesktopActionLeftClickDrag:
		return fmt.Sprintf(
			"ydotool mousemove --absolute -x %d -y %d; ydotool click 0x40; ydotool mousemove --absolute -x %d -y %d; ydotool click 0x80",
			coord(p.StartX), coord(p.StartY), coord(p.X), coord(p.Y),
		), nil

	case DesktopActionScroll:
		return "", fmt.Errorf("scroll is not supported on Wayland (v1) — use an X11 session")
	case DesktopActionHoldKey:
		return "", fmt.Errorf("hold-key is not supported on Wayland (v1) — use an X11 session")
	}
	return "", fmt.Errorf("unsupported desktop input action on Wayland: %s", p.Action)
}

func buildWtypeChord(chord string) string {
	parts := strings.Split(chord, "+")
	var mods []string
	key := ""
	for _, part := range parts {
		token := strings.TrimSpace(part)
		if m, ok := wtypeModifier(token); ok {
			mods = append(mods, m)
		} else {
			key = token
		}
	}
	var b strings.Builder
	b.WriteString("wtype")
	for _, m := range mods {
		b.WriteString(" -M " + shellQuote(m))
	}
	if key != "" {
		b.WriteString(" -k " + shellQuote(key))
	}
	for i := len(mods) - 1; i >= 0; i-- {
		b.WriteString(" -m " + shellQuote(mods[i]))
	}
	return b.String()
}

func wtypeModifier(token string) (string, bool) {
	switch strings.ToLower(token) {
	case "ctrl", "control":
		return "ctrl", true
	case "alt":
		return "alt", true
	case "shift":
		return "shift", true
	case "super", "win", "logo", "meta", "cmd":
		return "logo", true
	case "altgr":
		return "altgr", true
	}
	return "", false
}
