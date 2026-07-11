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
var getLastInputInfoProc = user32.NewProc("GetLastInputInfo")
var getTickCountProc = kernel32dll.NewProc("GetTickCount")

type lastInputInfo struct {
	cbSize uint32
	dwTime uint32
}

func getLastInputTime() (uint32, error) {
	var lii lastInputInfo
	lii.cbSize = uint32(unsafe.Sizeof(lii))
	r, _, err := getLastInputInfoProc.Call(uintptr(unsafe.Pointer(&lii)))
	if r == 0 {
		return 0, err
	}
	return lii.dwTime, nil
}

func getTickCount() uint32 {
	r, _, _ := getTickCountProc.Call()
	return uint32(r)
}

// IdleDuration returns how long the user has been idle.
func IdleDuration() (time.Duration, error) {
	last, err := getLastInputTime()
	if err != nil {
		return 0, err
	}
	return elapsedSinceLastInput(getTickCount(), last) * time.Millisecond, nil
}

func elapsedSinceLastInput(now, last uint32) time.Duration {
	// GetLastInputInfo's timestamp may move forward unexpectedly when another
	// process injects input. Treat a negative signed delta as fresh activity;
	// IdleTrigger's maximum configured timeout is far below the 24.8-day signed
	// tick interval, so this cannot mask a configured timeout.
	delta := int32(now - last)
	if delta < 0 {
		return 0
	}
	return time.Duration(delta) * time.Millisecond
}

// Monitor periodically checks idle time and fires:
//   - onWarning: when idle reaches (threshold − warningOffset)
//   - onTrigger: when idle reaches threshold
//
// Both fire at most once per idle session and reset when activity resumes.
type Monitor struct {
	thresholdNs    atomic.Int64 // nanoseconds
	warningOffset  time.Duration
	onWarning      func()
	onTrigger      func()
	onActivity     func()
	pollInterval   time.Duration
	idleDuration   func() (time.Duration, error)
	inputTimestamp func() (uint32, error)
	startedAt      time.Time
	lastInputTick  uint32
	seenInputTick  bool

	warned    atomic.Bool
	triggered atomic.Bool
	stopCh    chan struct{}
	doneCh    chan struct{}
	running   bool
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
	if warningOffset < 0 {
		warningOffset = 0
	}
	m := &Monitor{
		warningOffset:  warningOffset,
		onWarning:      onWarning,
		onTrigger:      onTrigger,
		pollInterval:   pollInterval,
		idleDuration:   IdleDuration,
		inputTimestamp: getLastInputTime,
	}
	m.thresholdNs.Store(int64(threshold))
	if pollInterval <= 0 {
		m.pollInterval = 3 * time.Second
	}
	return m
}

// Start begins monitoring in a background goroutine.
func (m *Monitor) Start() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return
	}
	m.stopCh = make(chan struct{})
	m.doneCh = make(chan struct{})
	m.startedAt = time.Now()
	m.seenInputTick = false
	m.running = true
	go m.loop(m.stopCh, m.doneCh)
}

// Stop signals the monitor to exit.
func (m *Monitor) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	stopCh := m.stopCh
	doneCh := m.doneCh
	m.running = false
	close(stopCh)
	m.mu.Unlock()
	<-doneCh
}

// SetThreshold updates the idle threshold at runtime.
func (m *Monitor) SetThreshold(d time.Duration) { m.thresholdNs.Store(int64(d)) }

// Threshold returns the current idle threshold.
func (m *Monitor) Threshold() time.Duration { return time.Duration(m.thresholdNs.Load()) }

// SetOnActivity sets a callback for the first user activity after a warning.
// It must be configured before Start.
func (m *Monitor) SetOnActivity(fn func()) { m.onActivity = fn }

func (m *Monitor) loop(stopCh <-chan struct{}, doneCh chan<- struct{}) {
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()
	defer close(doneCh)

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			inputTimestampAvailable := false
			if tick, inputErr := m.inputTimestamp(); inputErr == nil {
				inputTimestampAvailable = true
				if m.seenInputTick && tick != m.lastInputTick {
					wasWarned := m.warned.Load()
					m.lastInputTick = tick
					m.resetSession()
					if wasWarned && m.onActivity != nil {
						m.onActivity()
					}
					continue
				}
				m.lastInputTick = tick
				m.seenInputTick = true
			}

			dur, err := m.idleDuration()
			if err != nil {
				continue
			}
			// GetLastInputInfo reports activity that predates this monitor. A
			// newly launched or re-enabled monitor must begin its own timeout
			// window rather than immediately act on that historic idle period.
			if sinceStart := time.Since(m.startedAt); dur > sinceStart {
				dur = sinceStart
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
					// An accepted action ends this idle session. Start a fresh
					// timeout window so unlocking a locked PC cannot immediately
					// trigger the same action again on stale input timestamps.
					m.resetSession()
				}
			}

			// Reset all state when activity resumes. If a warning was visible,
			// notify the UI so it can dismiss the now-stale countdown.
			if !inputTimestampAvailable && dur < warnAt {
				if m.warned.Swap(false) && m.onActivity != nil {
					m.onActivity()
				}
				m.triggered.Store(false)
			}
		}
	}
}

func (m *Monitor) resetSession() {
	m.startedAt = time.Now()
	m.warned.Store(false)
	m.triggered.Store(false)
}
