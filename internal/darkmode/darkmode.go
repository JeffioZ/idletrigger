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
	uxtheme, err := syscall.LoadLibrary("uxtheme.dll")
	if err != nil {
		return
	}

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getProc := kernel32.NewProc("GetProcAddress")

	// SetPreferredAppMode — ordinal 135.  ForceDark (2) is more
	// aggressive than AllowDark (1) and works on more controls,
	// including TaskDialog.
	proc1, _, _ := getProc.Call(uintptr(uxtheme), uintptr(135))
	if proc1 != 0 {
		const forceDark = 2
		syscall.Syscall(proc1, 1, uintptr(forceDark), 0, 0)
	}

	// FlushMenuThemes — ordinal 136
	proc2, _, _ := getProc.Call(uintptr(uxtheme), uintptr(136))
	if proc2 != 0 {
		syscall.Syscall(proc2, 0, 0, 0, 0)
	}
}
