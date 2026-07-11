// Package darkmode enables dark-mode rendering for Win32 popup menus
// and controls on Windows 10 1809+.  The APIs are ordinal-only exports
// from uxtheme.dll so we load them by ordinal via GetProcAddress.
package darkmode

import (
	"syscall"
)

// Enable forces the process to use the dark (immersive) theme.
// Harmless no-op on Windows versions that lack these APIs.
func Enable() {
	SetPreferredAppMode(false)
}

// SetPreferredAppMode updates the process-level Win32 theme preference.
// forceDark is used for native popup menus, which do not always follow
// AllowDark after the system theme changes while the process stays running.
func SetPreferredAppMode(forceDark bool) {
	withUxtheme(func(uxtheme uintptr, getProc *syscall.LazyProc) {
		// SetPreferredAppMode — ordinal 135. 1 = AllowDark, 2 = ForceDark.
		proc, _, _ := getProc.Call(uxtheme, uintptr(135))
		if proc != 0 {
			mode := uintptr(1)
			if forceDark {
				mode = 2
			}
			syscall.SyscallN(proc, mode)
		}
		flushMenuThemes(uxtheme, getProc)
	})
}

// AllowWindow opts a Win32 owner window into immersive dark mode where the
// current Windows version supports it. It is harmless when the API is absent.
func AllowWindow(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	withUxtheme(func(uxtheme uintptr, getProc *syscall.LazyProc) {
		// AllowDarkModeForWindow — ordinal 133.
		proc, _, _ := getProc.Call(uxtheme, uintptr(133))
		if proc != 0 {
			syscall.SyscallN(proc, hwnd, 1)
		}
	})
}

// RefreshMenuThemes asks Windows to rebuild cached popup-menu theme resources.
// This is useful after the OS theme changes while the tray app stays running.
func RefreshMenuThemes() {
	withUxtheme(func(uxtheme uintptr, getProc *syscall.LazyProc) {
		flushMenuThemes(uxtheme, getProc)
	})
}

func withUxtheme(fn func(uxtheme uintptr, getProc *syscall.LazyProc)) {
	uxtheme, err := syscall.LoadLibrary("uxtheme.dll")
	if err != nil {
		return
	}
	defer syscall.FreeLibrary(uxtheme)

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getProc := kernel32.NewProc("GetProcAddress")
	fn(uintptr(uxtheme), getProc)
}

func flushMenuThemes(uxtheme uintptr, getProc *syscall.LazyProc) {
	// FlushMenuThemes — ordinal 136.
	proc2, _, _ := getProc.Call(uxtheme, uintptr(136))
	if proc2 != 0 {
		syscall.SyscallN(proc2)
	}
}
