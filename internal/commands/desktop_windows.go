//go:build windows

package commands
import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

const winInputClass = `Add-Type @"
using System;using System.Runtime.InteropServices;
public class IdaptInput{
 [DllImport("user32.dll")] public static extern bool SetCursorPos(int X,int Y);
 [DllImport("user32.dll")] public static extern void mouse_event(uint flags,uint dx,uint dy,int data,int extra);
 [StructLayout(LayoutKind.Sequential)] public struct POINT{public int X;public int Y;}
 [DllImport("user32.dll")] public static extern bool GetCursorPos(out POINT p);
}
"@`

func selectDesktopBackend(cfg RunuserConfig, runAs string, rawEnv map[string]string) DesktopBackend {
	shell := &shellBackend{cfg: cfg, runAs: runAs, env: desktopSessionEnv(rawEnv)}
	if desktopBackendOverride() == "shell" {
		return shell
	}
	native := &winNativeBackend{}
	if ok, _ := native.Probe(); ok {
		return native
	}
	return shell
}

func desktopProbe(_ map[string]string) (bool, string) {
	if _, err := exec.LookPath("powershell"); err != nil {
		return false, "powershell not found (required for Windows desktop automation)"
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

func runPowerShell(ctx context.Context, env map[string]string, script string) (string, error) {
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	if len(env) > 0 {
		merged := make([]string, 0)
		for k, v := range env {
			merged = append(merged, k+"="+v)
		}
		cmd.Env = append(cmd.Environ(), merged...)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("powershell: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func desktopCapture(
	ctx context.Context,
	_ RunuserConfig,
	_ string,
	env map[string]string,
	pngPath string,
	_ DesktopPayload,
) error {
	script := `Add-Type -AssemblyName System.Windows.Forms,System.Drawing
$vs=[System.Windows.Forms.SystemInformation]::VirtualScreen
$bmp=New-Object System.Drawing.Bitmap($vs.Width,$vs.Height)
$g=[System.Drawing.Graphics]::FromImage($bmp)
$g.CopyFromScreen($vs.Location,[System.Drawing.Point]::Empty,$vs.Size)
$bmp.Save(` + psQuote(pngPath) + `,[System.Drawing.Imaging.ImageFormat]::Png)
$g.Dispose();$bmp.Dispose()`
	_, err := runPowerShell(ctx, env, script)
	return err
}

func desktopCursorPosition(
	ctx context.Context,
	_ RunuserConfig,
	_ string,
	env map[string]string,
) (int, int, error) {
	script := winInputClass + "\n$p=New-Object IdaptInput+POINT;[IdaptInput]::GetCursorPos([ref]$p)|Out-Null;Write-Output \"$($p.X) $($p.Y)\""
	out, err := runPowerShell(ctx, env, script)
	if err != nil {
		return 0, 0, err
	}
	return parseTwoInts(out)
}

func desktopInput(
	ctx context.Context,
	_ RunuserConfig,
	_ string,
	env map[string]string,
	p DesktopPayload,
) error {
	script, err := buildWindowsInput(p)
	if err != nil {
		return err
	}
	_, err = runPowerShell(ctx, env, script)
	return err
}

const (
	winMouseMoveTo = ""
	winLeftDown    = "0x0002"
	winLeftUp      = "0x0004"
	winRightDown   = "0x0008"
	winRightUp     = "0x0010"
	winMiddleDown  = "0x0020"
	winMiddleUp    = "0x0040"
	winWheel       = "0x0800"
	winWheelDelta  = 120
)

func winSetPos(x, y int) string {
	return fmt.Sprintf("[IdaptInput]::SetCursorPos(%d,%d)|Out-Null", x, y)
}

func winClick(down, up string) string {
	return fmt.Sprintf("[IdaptInput]::mouse_event(%s,0,0,0,0);[IdaptInput]::mouse_event(%s,0,0,0,0)", down, up)
}

func buildWindowsInput(p DesktopPayload) (string, error) {
	x, y := coord(p.X), coord(p.Y)
	hasCoord := p.X != nil && p.Y != nil
	pre := winInputClass + "\n"
	move := ""
	if hasCoord {
		move = winSetPos(x, y) + ";"
	}

	switch p.Action {
	case DesktopActionMouseMove:
		return pre + winSetPos(x, y), nil
	case DesktopActionLeftClick:
		return pre + move + winClick(winLeftDown, winLeftUp), nil
	case DesktopActionRightClick:
		return pre + move + winClick(winRightDown, winRightUp), nil
	case DesktopActionMiddleClick:
		return pre + move + winClick(winMiddleDown, winMiddleUp), nil
	case DesktopActionDoubleClick:
		return pre + move + winClick(winLeftDown, winLeftUp) + ";" + winClick(winLeftDown, winLeftUp), nil
	case DesktopActionTripleClick:
		return pre + move + winClick(winLeftDown, winLeftUp) + ";" + winClick(winLeftDown, winLeftUp) + ";" + winClick(winLeftDown, winLeftUp), nil
	case DesktopActionLeftMouseDown:
		return pre + move + fmt.Sprintf("[IdaptInput]::mouse_event(%s,0,0,0,0)", winLeftDown), nil
	case DesktopActionLeftMouseUp:
		return pre + move + fmt.Sprintf("[IdaptInput]::mouse_event(%s,0,0,0,0)", winLeftUp), nil
	case DesktopActionLeftClickDrag:
		return pre +
			winSetPos(coord(p.StartX), coord(p.StartY)) + ";" +
			fmt.Sprintf("[IdaptInput]::mouse_event(%s,0,0,0,0);", winLeftDown) +
			winSetPos(x, y) + ";" +
			fmt.Sprintf("[IdaptInput]::mouse_event(%s,0,0,0,0)", winLeftUp), nil
	case DesktopActionScroll:
		amount := p.ScrollAmount
		if amount <= 0 {
			amount = 3
		}
		delta := winWheelDelta * amount
		if p.ScrollDirection == "down" {
			delta = -delta
		}
		if p.ScrollDirection == "left" || p.ScrollDirection == "right" {
			return "", fmt.Errorf("horizontal scroll is not supported on Windows in v1")
		}
		return pre + move + fmt.Sprintf("[IdaptInput]::mouse_event(%s,0,0,%d,0)", winWheel, delta), nil
	case DesktopActionType:
		if p.Text == "" {
			return "", fmt.Errorf("type action requires text")
		}
		return "Add-Type -AssemblyName System.Windows.Forms\n[System.Windows.Forms.SendKeys]::SendWait(" +
			psQuote(sendKeysEscape(p.Text)) + ")", nil
	case DesktopActionKey:
		if strings.TrimSpace(p.Text) == "" {
			return "", fmt.Errorf("key action requires text (the key chord)")
		}
		return "Add-Type -AssemblyName System.Windows.Forms\n[System.Windows.Forms.SendKeys]::SendWait(" +
			psQuote(chordToSendKeys(p.Text)) + ")", nil
	case DesktopActionHoldKey:
		return "", fmt.Errorf("hold-key is not supported on Windows in v1")
	}
	return "", fmt.Errorf("unsupported desktop input action: %s", p.Action)
}

func sendKeysEscape(s string) string {
	repl := strings.NewReplacer(
		"{", "{{}", "}", "{}}",
		"+", "{+}", "^", "{^}", "%", "{%}", "~", "{~}",
		"(", "{(}", ")", "{)}", "[", "{[}", "]", "{]}",
	)
	return repl.Replace(s)
}

func chordToSendKeys(chord string) string {
	parts := strings.Split(chord, "+")
	var prefix strings.Builder
	key := ""
	for _, part := range parts {
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "ctrl", "control":
			prefix.WriteString("^")
		case "alt":
			prefix.WriteString("%")
		case "shift":
			prefix.WriteString("+")
		default:
			key = strings.TrimSpace(part)
		}
	}
	special := map[string]string{
		"return": "{ENTER}", "enter": "{ENTER}", "tab": "{TAB}",
		"escape": "{ESC}", "esc": "{ESC}", "backspace": "{BACKSPACE}",
		"delete": "{DEL}", "space": " ", "up": "{UP}", "down": "{DOWN}",
		"left": "{LEFT}", "right": "{RIGHT}", "home": "{HOME}", "end": "{END}",
	}
	if mapped, ok := special[strings.ToLower(key)]; ok {
		key = mapped
	}
	return prefix.String() + key
}
