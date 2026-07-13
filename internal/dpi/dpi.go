// Package dpi sets the process DPI awareness so that IdleTrigger renders
// crisply on high-DPI displays (4K, 150%+ scaling).  It tries Per-Monitor
// v2 first (Windows 10 1703+), falling back to System DPI awareness.
package dpi

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// Enable declares this process as DPI-aware.  Call once at startup before
// any UI is created.
func Enable() {
	// Per-Monitor DPI Awareness v2 is available on Windows 10 1703+.
	// The context values are negative handles: -4, -3, -2, -1.
	const perMonitorV2 = ^uintptr(3) // -4

	user32 := windows.NewLazySystemDLL("user32.dll")
	setCtx := user32.NewProc("SetProcessDpiAwarenessContext")
	if err := setCtx.Find(); err == nil {
		ret, _, _ := setCtx.Call(perMonitorV2)
		if ret != 0 {
			return
		}
	}

	// Fallback: SetProcessDpiAwareness (Windows 8.1+).
	shcore := windows.NewLazySystemDLL("shcore.dll")
	setAware := shcore.NewProc("SetProcessDpiAwareness")
	const perMonitor = 2
	if err := setAware.Find(); err == nil {
		ret, _, _ := setAware.Call(uintptr(perMonitor))
		if ret == 0 { // S_OK
			return
		}
	}

	// Final fallback for systems that only expose legacy system DPI awareness.
	setLegacy := user32.NewProc("SetProcessDPIAware")
	if err := setLegacy.Find(); err == nil {
		setLegacy.Call()
	}
}

// WithFixed96 creates a short-lived thread DPI context for deterministic
// off-screen/documentation captures. It is intentionally opt-in: normal UI
// windows continue to use the process's per-monitor DPI behavior.
func WithFixed96(run func() error) error {
	user32 := windows.NewLazySystemDLL("user32.dll")
	setThreadContext := user32.NewProc("SetThreadDpiAwarenessContext")
	if err := setThreadContext.Find(); err != nil {
		return run()
	}
	const unaware = ^uintptr(0) // DPI_AWARENESS_CONTEXT_UNAWARE (-1)
	previous, _, callErr := setThreadContext.Call(unaware)
	if previous == 0 {
		return fmt.Errorf("SetThreadDpiAwarenessContext: %w", callErr)
	}
	defer setThreadContext.Call(previous)
	return run()
}
