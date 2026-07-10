package processwatcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcherDetectsCurrentProcessAndStops(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	enabled := make(chan struct{}, 1)
	w := New([]string{filepath.Base(exe)}, Callbacks{
		OnEnable: func() { enabled <- struct{}{} },
	}, time.Hour)
	w.Start()

	select {
	case <-enabled:
	case <-time.After(5 * time.Second):
		t.Fatal("watcher did not detect the current process")
	}

	done := make(chan struct{})
	go func() {
		w.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("watcher did not stop promptly")
	}

	// A stopped watcher can be started and stopped again.
	w.Start()
	w.Stop()
}
