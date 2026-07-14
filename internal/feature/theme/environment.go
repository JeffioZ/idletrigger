package theme

import (
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/JeffioZ/idletrigger/internal/platform/windows/powerstate"
)

var (
	themeEnvironmentUser32 = windows.NewLazySystemDLL("user32.dll")
	pGetForegroundWindow   = themeEnvironmentUser32.NewProc("GetForegroundWindow")
	pGetWindowRect         = themeEnvironmentUser32.NewProc("GetWindowRect")
	pMonitorFromWindow     = themeEnvironmentUser32.NewProc("MonitorFromWindow")
	pGetMonitorInfo        = themeEnvironmentUser32.NewProc("GetMonitorInfoW")
)

// IsFullscreen returns true when the foreground window covers its monitor.
func IsFullscreen() bool {
	hwnd, _, _ := pGetForegroundWindow.Call()

	if hwnd == 0 {
		return false
	}

	type rect struct{ left, top, right, bottom int32 }
	var r rect
	if ok, _, _ := pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r))); ok == 0 {
		return false
	}

	const monitorDefaultToNearest = 2
	monitor, _, _ := pMonitorFromWindow.Call(hwnd, monitorDefaultToNearest)
	if monitor == 0 {
		return false
	}
	type monitorInfo struct {
		size    uint32
		monitor rect
		work    rect
		flags   uint32
	}
	var info monitorInfo
	info.size = uint32(unsafe.Sizeof(info))
	if ok, _, _ := pGetMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info))); ok == 0 {
		return false
	}

	const tolerance = 2
	return r.left <= info.monitor.left+tolerance &&
		r.top <= info.monitor.top+tolerance &&
		r.right >= info.monitor.right-tolerance &&
		r.bottom >= info.monitor.bottom-tolerance
}

func onBattery() bool {
	return powerstate.OnBattery()
}
