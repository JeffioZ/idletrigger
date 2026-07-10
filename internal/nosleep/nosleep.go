// Package nosleep prevents Windows from automatically activating sleep mode
// or turning off the display via the Win32 SetThreadExecutionState API.
package nosleep

import (
	"runtime"
	"sync"
	"sync/atomic"

	"golang.org/x/sys/windows"
)

var (
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	enabled  atomic.Bool
	keepScr  atomic.Bool

	mu     sync.Mutex
	worker *executionWorker
)

type executionWorker struct {
	updates chan updateRequest
	stop    chan chan struct{}
}

type updateRequest struct {
	keepScreen bool
	done       chan struct{}
}

const (
	esContinuous      = 0x80000000
	esSystemRequired  = 0x00000001
	esDisplayRequired = 0x00000002
)

// Enable activates NoSleep. Calls are serialized onto one locked OS thread,
// because Windows tracks continuous execution requests per calling thread.
func Enable(keepScreenOn bool) {
	mu.Lock()
	defer mu.Unlock()

	if worker == nil {
		worker = &executionWorker{
			updates: make(chan updateRequest),
			stop:    make(chan chan struct{}),
		}
		ready := make(chan struct{})
		go worker.loop(keepScreenOn, ready)
		<-ready
	} else {
		done := make(chan struct{})
		worker.updates <- updateRequest{keepScreen: keepScreenOn, done: done}
		<-done
	}

	keepScr.Store(keepScreenOn)
	enabled.Store(true)
}

// Disable clears the request on the same OS thread that created it.
func Disable() {
	mu.Lock()
	defer mu.Unlock()

	if worker == nil {
		enabled.Store(false)
		keepScr.Store(false)
		return
	}

	done := make(chan struct{})
	worker.stop <- done
	<-done
	worker = nil
	enabled.Store(false)
	keepScr.Store(false)
}

func IsEnabled() bool { return enabled.Load() }

func IsKeepingScreenOn() bool { return keepScr.Load() }

func (w *executionWorker) loop(initialKeepScreen bool, ready chan struct{}) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	setExecutionState(initialKeepScreen)
	close(ready)

	for {
		select {
		case req := <-w.updates:
			setExecutionState(req.keepScreen)
			close(req.done)
		case done := <-w.stop:
			proc := kernel32.NewProc("SetThreadExecutionState")
			proc.Call(uintptr(esContinuous))
			close(done)
			return
		}
	}
}

func setExecutionState(keepScreen bool) {
	flags := uintptr(esContinuous | esSystemRequired)
	if keepScreen {
		flags |= esDisplayRequired
	}
	proc := kernel32.NewProc("SetThreadExecutionState")
	proc.Call(flags)
}
