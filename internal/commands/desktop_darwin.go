//go:build darwin

package commands
import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

func selectDesktopBackend(cfg RunuserConfig, runAs string, rawEnv map[string]string) DesktopBackend {
	sessEnv := desktopSessionEnv(rawEnv)
	shell := &shellBackend{cfg: cfg, runAs: runAs, env: sessEnv}
	if desktopBackendOverride() == "shell" {
		return shell
	}
	if nb := darwinNativeBackend(cfg, runAs, sessEnv); nb != nil {
		if ok, _ := nb.Probe(); ok {
			return nb
		}
	}
	return shell
}

func desktopProbe(_ map[string]string) (bool, string) {
	if _, err := exec.LookPath("screencapture"); err != nil {
		return false, "screencapture not found (expected on macOS)"
	}
	return true, ""
}

func desktopSessionEnv(env map[string]string) map[string]string {
	out := make(map[string]string, len(env))
	for k, v := range env {
		out[k] = v
	}
	return out
}

func desktopCapture(
	ctx context.Context,
	cfg RunuserConfig,
	runAs string,
	env map[string]string,
	pngPath string,
	_ DesktopPayload,
) error {
	inner := "screencapture -x -t png -D 1 " + shellQuote(pngPath)
	_, err := runDesktopShell(ctx, cfg, runAs, inner, env)
	return err
}

func darwinBackingScale() float64 {
	darwinScaleOnce.Do(func() {
		out, err := exec.Command("system_profiler", "SPDisplaysDataType").Output()
		if err != nil {
			return
		}
		if s := parseDarwinBackingScale(string(out)); s > 0 {
			darwinScaleVal = s
		}
	})
	return darwinScaleVal
}

var (
	darwinScaleOnce sync.Once
	darwinScaleVal  = 1.0
)

func desktopCursorPosition(
	ctx context.Context,
	cfg RunuserConfig,
	runAs string,
	env map[string]string,
) (int, int, error) {
	if err := requireCliclick(); err != nil {
		return 0, 0, err
	}
	out, err := runDesktopShell(ctx, cfg, runAs, "cliclick p:.", env)
	if err != nil {
		return 0, 0, err
	}
	x, y, err := parseTwoInts(out) // cliclick prints "x,y" in logical points
	if err != nil {
		return 0, 0, err
	}
	s := darwinBackingScale()
	return scaleUpCoord(x, s), scaleUpCoord(y, s), nil
}

func desktopInput(
	ctx context.Context,
	cfg RunuserConfig,
	runAs string,
	env map[string]string,
	p DesktopPayload,
) error {
	if p.Action == DesktopActionType {
		if err := requireCliclick(); err != nil {
			return err
		}
		if p.Text == "" {
			return fmt.Errorf("type action requires text")
		}
		_, err := runDesktopShell(ctx, cfg, runAs, "cliclick t:"+shellQuote(p.Text), env)
		return err
	}

	if err := requireCliclick(); err != nil {
		return err
	}
	inner, err := buildDarwinInput(p, darwinBackingScale())
	if err != nil {
		return err
	}
	_, err = runDesktopShell(ctx, cfg, runAs, inner, env)
	return err
}

func requireCliclick() error {
	if _, err := exec.LookPath("cliclick"); err != nil {
		return fmt.Errorf("cliclick not installed — run `brew install cliclick` to enable mouse/keyboard input on macOS")
	}
	return nil
}

func buildDarwinInput(p DesktopPayload, scale float64) (string, error) {
	x, y := scaleDownCoord(coord(p.X), scale), scaleDownCoord(coord(p.Y), scale)
	switch p.Action {
	case DesktopActionMouseMove:
		return fmt.Sprintf("cliclick m:%d,%d", x, y), nil
	case DesktopActionLeftClick:
		return fmt.Sprintf("cliclick c:%d,%d", x, y), nil
	case DesktopActionRightClick:
		return fmt.Sprintf("cliclick rc:%d,%d", x, y), nil
	case DesktopActionDoubleClick:
		return fmt.Sprintf("cliclick dc:%d,%d", x, y), nil
	case DesktopActionTripleClick:
		return fmt.Sprintf("cliclick c:%d,%d c:%d,%d c:%d,%d", x, y, x, y, x, y), nil
	case DesktopActionLeftClickDrag:
		return fmt.Sprintf("cliclick dd:%d,%d du:%d,%d",
			scaleDownCoord(coord(p.StartX), scale), scaleDownCoord(coord(p.StartY), scale), x, y), nil
	case DesktopActionLeftMouseDown:
		return fmt.Sprintf("cliclick dd:%d,%d", x, y), nil
	case DesktopActionLeftMouseUp:
		return fmt.Sprintf("cliclick du:%d,%d", x, y), nil
	case DesktopActionMiddleClick, DesktopActionScroll, DesktopActionKey, DesktopActionHoldKey:
		return "", fmt.Errorf("desktop action %q is not yet supported on macOS (v1) — use a Linux computer for full coverage", p.Action)
	}
	return "", fmt.Errorf("unsupported desktop input action: %s", strings.TrimSpace(p.Action))
}
