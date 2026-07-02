//go:build windows

package commands
import "golang.org/x/sys/windows"

const (
	dpiAwarenessContextPerMonitorAwareV2 = ^uintptr(3) // -4 as uintptr
	processPerMonitorDpiAware = 2
)

var shcore = windows.NewLazySystemDLL("shcore.dll")

var (
	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procSetProcessDpiAwareness        = shcore.NewProc("SetProcessDpiAwareness")
	procSetProcessDPIAware            = user32.NewProc("SetProcessDPIAware")
)

func init() {
	if procSetProcessDpiAwarenessContext.Find() == nil {
		if ret, _, _ := procSetProcessDpiAwarenessContext.Call(dpiAwarenessContextPerMonitorAwareV2); ret != 0 {
			return
		}
	}
	if procSetProcessDpiAwareness.Find() == nil {
		if ret, _, _ := procSetProcessDpiAwareness.Call(uintptr(processPerMonitorDpiAware)); ret == 0 {
			return
		}
	}
	if procSetProcessDPIAware.Find() == nil {
		_, _, _ = procSetProcessDPIAware.Call()
	}
}
