//go:build windows

package commands
import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procEnumDisplayMonitors = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfo      = user32.NewProc("GetMonitorInfoW")
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")
	procGetWindowText       = user32.NewProc("GetWindowTextW")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
)

const monitorinfofPrimary = 0x1

type winRect struct{ Left, Top, Right, Bottom int32 }

type monitorInfoEx struct {
	cbSize    uint32
	rcMonitor winRect
	rcWork    winRect
	dwFlags   uint32
	szDevice  [32]uint16
}

var (
	monEnumMu   sync.Mutex
	monEnumOut  []DesktopDisplay
	monEnumIdx  int
	monEnumProc = windows.NewCallback(monitorEnumCallback)
)

func monitorEnumCallback(hMonitor, _, _, _ uintptr) uintptr {
	var mi monitorInfoEx
	mi.cbSize = uint32(unsafe.Sizeof(mi))
	if r, _, _ := procGetMonitorInfo.Call(hMonitor, uintptr(unsafe.Pointer(&mi))); r != 0 {
		monEnumOut = append(monEnumOut, DesktopDisplay{
			ID:      strconv.Itoa(monEnumIdx),
			Name:    windows.UTF16ToString(mi.szDevice[:]),
			Width:   int(mi.rcMonitor.Right - mi.rcMonitor.Left),
			Height:  int(mi.rcMonitor.Bottom - mi.rcMonitor.Top),
			OffsetX: int(mi.rcMonitor.Left),
			OffsetY: int(mi.rcMonitor.Top),
			Primary: mi.dwFlags&monitorinfofPrimary != 0,
			Scale:   1,
		})
		monEnumIdx++
	}
	return 1 // continue enumeration
}

func enumDisplays() []DesktopDisplay {
	monEnumMu.Lock()
	defer monEnumMu.Unlock()
	monEnumOut = nil
	monEnumIdx = 0
	procEnumDisplayMonitors.Call(0, 0, monEnumProc, 0)
	displays := monEnumOut
	monEnumOut = nil
	if len(displays) == 0 {
		b := &winNativeBackend{}
		_, _, vw, vh := b.virtualScreen()
		displays = []DesktopDisplay{{ID: "0", Name: "primary", Width: vw, Height: vh, Primary: true, Scale: 1}}
	}
	ensurePrimary(displays)
	return displays
}

func (b *winNativeBackend) ListDisplays(_ context.Context) ([]DesktopDisplay, error) {
	return enumDisplays(), nil
}

func foregroundWindowTitle() string {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return ""
	}
	buf := make([]uint16, 512)
	n, _, _ := procGetWindowText.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf[:n])
}
func (b *winNativeBackend) Clipboard(ctx context.Context) (string, error) {
	if !binAvailable("powershell") {
		return "", errDesktopActionUnsupported
	}
	return runPowerShell(ctx, b.env, "Get-Clipboard -Raw")
}

func (b *winNativeBackend) SetClipboard(ctx context.Context, text string) error {
	if !binAvailable("powershell") {
		return errDesktopActionUnsupported
	}
	_, err := runPowerShell(ctx, b.env, "Set-Clipboard -Value "+psQuote(text))
	return err
}

const psWinDelim = "|||"

func (b *winNativeBackend) ListWindows(ctx context.Context) ([]DesktopWindow, error) {
	if !binAvailable("powershell") {
		return nil, errDesktopActionUnsupported
	}
	script := "Get-Process | Where-Object { $_.MainWindowHandle -ne 0 -and $_.MainWindowTitle } | " +
		"ForEach-Object { $_.MainWindowHandle.ToString() + '" + psWinDelim + "' + $_.ProcessName + '" + psWinDelim + "' + $_.MainWindowTitle }"
	out, err := runPowerShell(ctx, b.env, script)
	if err != nil {
		return nil, err
	}
	wins := parsePowerShellWindows(out)
	fg, _, _ := procGetForegroundWindow.Call()
	fgID := strconv.FormatUint(uint64(fg), 10)
	for i := range wins {
		if wins[i].ID == fgID {
			wins[i].Focused = true
		}
	}
	return wins, nil
}

func (b *winNativeBackend) FocusWindow(ctx context.Context, windowID, titleMatch string) error {
	if windowID == "" {
		wins, err := b.ListWindows(ctx)
		if err != nil {
			return err
		}
		match := strings.ToLower(titleMatch)
		for _, w := range wins {
			if strings.Contains(strings.ToLower(w.Title), match) {
				windowID = w.ID
				break
			}
		}
		if windowID == "" {
			return fmt.Errorf("no window matching %q", titleMatch)
		}
	}
	hwnd, err := strconv.ParseUint(windowID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid windowId %q: %w", windowID, err)
	}
	if r, _, e := procSetForegroundWindow.Call(uintptr(hwnd)); r == 0 {
		return fmt.Errorf("SetForegroundWindow failed: %v", e)
	}
	return nil
}

func parsePowerShellWindows(out string) []DesktopWindow {
	var wins []DesktopWindow
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, psWinDelim, 3)
		if len(parts) < 3 {
			continue
		}
		wins = append(wins, DesktopWindow{
			ID:    strings.TrimSpace(parts[0]),
			App:   parts[1],
			Title: parts[2],
		})
	}
	return wins
}
