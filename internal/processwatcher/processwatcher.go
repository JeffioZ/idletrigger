// Package processwatcher — watches the process list and auto-toggles
// NoSleep when user-specified applications are running.
// 监测进程列表，在指定应用运行时自动切换 NoSleep。 — useful for presentations, video playback, etc.
package processwatcher

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Callbacks are invoked when watched-process state changes.
type Callbacks struct {
	OnEnable  func() // all watched procs gone → some now running
	OnDisable func() // some watched procs running → all gone
}

// Watcher periodically scans the process list for configured executables.
type Watcher struct {
	exes      []string
	cbs       Callbacks
	interval  time.Duration
	stopCh    chan struct{}
	mu        sync.Mutex
	wasActive atomic.Bool
}

// New creates a Watcher.  exeNames are case-insensitive .exe names like
// ["chrome.exe", "powerpnt.exe"].  interval controls the scan frequency
// (default 10 s if ≤ 0).
func New(exeNames []string, cbs Callbacks, interval time.Duration) *Watcher {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	// Lower-case all names for comparison.
	lowered := make([]string, len(exeNames))
	for i, n := range exeNames {
		lowered[i] = strings.ToLower(n)
	}
	return &Watcher{
		exes:     lowered,
		cbs:      cbs,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start begins scanning in a background goroutine.
func (w *Watcher) Start() {
	go w.loop()
}

// Stop signals the watcher to exit.
func (w *Watcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	select {
	case <-w.stopCh:
	default:
		close(w.stopCh)
	}
}

func (w *Watcher) loop() {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	// Fire initial state.
	w.check()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.check()
		}
	}
}

func (w *Watcher) check() {
	if len(w.exes) == 0 {
		return
	}
	active := w.anyRunning()
	was := w.wasActive.Swap(active)

	if active && !was && w.cbs.OnEnable != nil {
		w.cbs.OnEnable()
	} else if !active && was && w.cbs.OnDisable != nil {
		w.cbs.OnDisable()
	}
}

func (w *Watcher) anyRunning() bool {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snapshot)

	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))

	err = windows.Process32First(snapshot, &pe)
	for err == nil {
		name := strings.ToLower(windows.UTF16ToString(pe.ExeFile[:]))
		for _, exe := range w.exes {
			if name == exe {
				return true
			}
		}
		err = windows.Process32Next(snapshot, &pe)
	}
	return false
}
