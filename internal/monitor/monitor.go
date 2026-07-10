// Package monitor tracks user idle time via the Win32 GetLastInputInfo API
// and fires callbacks when the configured threshold is approached and exceeded.
package monitor

import (
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var user32 = windows.NewLazySystemDLL("user32.dll")
var kernel32dll = windows.NewLazySystemDLL("kernel32.dll")

type lastInputInfo struct {
	cbSize uint32
	dwTime uint32
}

func getLastInputTime() (uint32, error) {
	proc := user32.NewProc("GetLastInputInfo")
	var lii lastInputInfo
	lii.cbSize = uint32(unsafe.Sizeof(lii))
	r, _, err := proc.Call(uintptr(unsafe.Pointer(&lii)))
	if r == 0 {
		return 0, err
	}
	return lii.dwTime, nil
}

func getTickCount() uint32 {
	proc := kernel32dll.NewProc("GetTickCount")
	r, _, _ := proc.Call()
	return uint32(r)
}

// IdleDuration returns how long the user has been idle.
func IdleDuration() (time.Duration, error) {
	last, err := getLastInputTime()
	if err != nil {
		return 0, err
	}
	now := getTickCount()
	var elapsed uint32
	if now >= last {
		elapsed = now - last
	} else {
		elapsed = (^uint32(0) - last) + now + 1
	}
	return time.Duration(elapsed) * time.Millisecond, nil
}

// Monitor periodically checks idle time and fires:
//   - onWarning: when idle reaches (threshold − warningOffset)
//   - onTrigger: when idle reaches threshold (only if warning was fired)
//
// Both fire at most once per idle session and reset when activity resumes.
type Monitor struct {
	thresholdNs   atomic.Int64 // nanoseconds
	warningOffset time.Duration
	onWarning     func()
	onTrigger     func()
	pollInterval  time.Duration

	warned    atomic.Bool
	triggered atomic.Bool
	stopCh    chan struct{}
	stopped   atomic.Bool
	mu        sync.Mutex
}

// New creates a Monitor.
//   - threshold: idle duration after which onTrigger fires
//   - warningOffset: how long before threshold to fire onWarning (0 = no warning)
//   - onWarning: called when approaching the threshold (may be nil)
//   - onTrigger: called when threshold is reached
//   - pollInterval: check frequency (default 3 s if ≤ 0)
func New(
	threshold, warningOffset time.Duration,
	onWarning, onTrigger func(),
	pollInterval time.Duration,
) *Monitor {
	m := &Monitor{
		warningOffset: warningOffset,
		onWarning:     onWarning,
		onTrigger:     onTrigger,
		pollInterval:  pollInterval,
		stopCh:        make(chan struct{}),
	}
	m.thresholdNs.Store(int64(threshold))
	if pollInterval <= 0 {
		m.pollInterval = 3 * time.Second
	}
	if warningOffset < 0 {
		warningOffset = 0
	}
	return m
}

// Start begins monitoring in a background goroutine.
func (m *Monitor) Start() {
	m.mu.Lock()
	if m.stopped.Load() {
		m.stopCh = make(chan struct{})
		m.stopped.Store(false)
	}
	m.mu.Unlock()
	go m.loop()
}

// Stop signals the monitor to exit.
func (m *Monitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopped.Load() {
		return
	}
	m.stopped.Store(true)
	close(m.stopCh)
}

// SetThreshold updates the idle threshold at runtime.
func (m *Monitor) SetThreshold(d time.Duration) { m.thresholdNs.Store(int64(d)) }

// Threshold returns the current idle threshold.
func (m *Monitor) Threshold() time.Duration { return time.Duration(m.thresholdNs.Load()) }

func (m *Monitor) loop() {
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			dur, err := IdleDuration()
			if err != nil {
				continue
			}

			warnAt := time.Duration(m.thresholdNs.Load()) - m.warningOffset
			if warnAt < 0 {
				warnAt = 0
			}

			// Phase 1: warning window — idle >= (threshold − offset) but < threshold
			if dur >= warnAt && dur < time.Duration(m.thresholdNs.Load()) {
				if !m.warned.Swap(true) && m.onWarning != nil {
					m.onWarning()
				}
			}

			// Phase 2: trigger — idle >= threshold
			if dur >= time.Duration(m.thresholdNs.Load()) {
				if !m.triggered.Swap(true) && m.onTrigger != nil {
					m.onTrigger()
				}
			}

			// Reset all state when activity resumes.
			if dur < warnAt {
				m.warned.Store(false)
				m.triggered.Store(false)
			}
		}
	}
}
