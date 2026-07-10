// Package processwatcher monitors the running process list and
// automatically toggles NoSleep when user-specified applications are
// detected — useful for presentations, video playback, etc.
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
	doneCh    chan struct{}
	mu        sync.Mutex
	running   bool
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
	}
}

// Start begins scanning in a background goroutine.
func (w *Watcher) Start() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.running {
		return
	}
	w.stopCh = make(chan struct{})
	w.doneCh = make(chan struct{})
	w.running = true
	go w.loop(w.stopCh, w.doneCh)
}

// Stop signals the watcher to exit.
func (w *Watcher) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	stopCh := w.stopCh
	doneCh := w.doneCh
	w.running = false
	close(stopCh)
	w.mu.Unlock()
	<-doneCh
}

func (w *Watcher) loop(stopCh <-chan struct{}, doneCh chan<- struct{}) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	defer close(doneCh)

	// Fire initial state.
	w.check()

	for {
		select {
		case <-stopCh:
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
	active, err := w.anyRunning()
	if err != nil {
		return
	}
	was := w.wasActive.Swap(active)

	if active && !was && w.cbs.OnEnable != nil {
		w.cbs.OnEnable()
	} else if !active && was && w.cbs.OnDisable != nil {
		w.cbs.OnDisable()
	}
}

func (w *Watcher) anyRunning() (bool, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false, err
	}
	defer windows.CloseHandle(snapshot)

	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))

	err = windows.Process32First(snapshot, &pe)
	for err == nil {
		name := strings.ToLower(windows.UTF16ToString(pe.ExeFile[:]))
		for _, exe := range w.exes {
			if name == exe {
				return true, nil
			}
		}
		err = windows.Process32Next(snapshot, &pe)
	}
	if err == windows.ERROR_NO_MORE_FILES {
		return false, nil
	}
	return false, err
}
