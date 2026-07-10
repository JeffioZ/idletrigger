// Package nosleep prevents Windows from automatically activating sleep mode
// or turning off the display via the Win32 SetThreadExecutionState API.
//
// Usage:
//
//	nosleep.Enable(true)   // prevent sleep + keep screen on
//	nosleep.Enable(false)  // prevent sleep only, screen may turn off
//	nosleep.Disable()      // restore normal power behaviour
package nosleep

import (
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	enabled  atomic.Bool
	keepScr  atomic.Bool

	stopCh chan struct{}
	mu     sync.Mutex
)

const (
	// win32 execution-state flags
	esContinuous      = 0x80000000
	esSystemRequired  = 0x00000001
	esDisplayRequired = 0x00000002
)

// Enable activates NoSleep.  keepScreenOn controls whether the display is
// also kept awake.  Safe to call multiple times — subsequent calls update
// the screen setting.
func Enable(keepScreenOn bool) {
	mu.Lock()
	defer mu.Unlock()

	keepScr.Store(keepScreenOn)

	if enabled.Swap(true) {
		// Already running — just update the flags at the next tick.
		return
	}

	stopCh = make(chan struct{})
	go loop()
}

// Disable deactivates NoSleep and restores normal system power behaviour.
func Disable() {
	mu.Lock()
	defer mu.Unlock()

	if !enabled.Swap(false) {
		return // already disabled
	}

	close(stopCh)
	stopCh = nil

	// Reset execution state — allow sleep again.
	proc := kernel32.NewProc("SetThreadExecutionState")
	proc.Call(uintptr(esContinuous))
}

// IsEnabled reports whether NoSleep is currently active.
func IsEnabled() bool {
	return enabled.Load()
}

// IsKeepingScreenOn reports whether the display is being kept on.
func IsKeepingScreenOn() bool {
	return keepScr.Load()
}

func loop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Fire immediately on start.
	tick()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			tick()
		}
	}
}

func tick() {
	flags := uintptr(esContinuous | esSystemRequired)
	if keepScr.Load() {
		flags |= esDisplayRequired
	}
	proc := kernel32.NewProc("SetThreadExecutionState")
	proc.Call(flags)

	// Keep Go's GC from collecting the callback while the call is in flight.
	_ = unsafe.Sizeof(0)
}
