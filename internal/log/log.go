// Package log provides lightweight file logging for debugging.
// Logs are written to IdleTrigger.log next to the executable;
// falls back to %TEMP% if the EXE directory is not writable.
package log

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	mu        sync.Mutex
	w         io.WriteCloser
	on        bool
	sessionID string
)

const maxLogSize = 5 << 20

// Init opens the log file.  If enabled is false the package becomes a
// silent no-op.  exeDir should be os.Executable()'s directory.
func Init(enabled bool, exeDir string) {
	mu.Lock()
	defer mu.Unlock()
	if w != nil {
		writeLocked("=== IdleTrigger session ended ===")
		w.Close()
		w = nil
	}
	on = enabled
	if !on {
		sessionID = ""
		return
	}
	sessionID = fmt.Sprintf("%x-%x", time.Now().UnixNano(), os.Getpid())

	path := filepath.Join(exeDir, "IdleTrigger.log")
	rotate(path)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		// Fall back to %TEMP%.
		path = filepath.Join(os.TempDir(), "IdleTrigger.log")
		rotate(path)
		f, err = os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			on = false
			return
		}
	}

	w = f
	writeLocked("=== IdleTrigger session started ===")
}

// Close flushes and closes the log file.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if w != nil {
		writeLocked("=== IdleTrigger session ended ===")
		w.Close()
		w = nil
	}
	on = false
	sessionID = ""
}

// Info writes a timestamped informational message.
func Info(format string, args ...interface{}) {
	write(fmt.Sprintf(format, args...))
}

func write(msg string) {
	mu.Lock()
	defer mu.Unlock()
	if !on || w == nil {
		return
	}
	writeLocked(msg)
}

func writeLocked(msg string) {
	ts := time.Now().Format("2006-01-02 15:04:05.000")
	fmt.Fprintf(w, "[%s] [session:%s] %s\n", ts, sessionID, msg)
}

func rotate(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < maxLogSize {
		return
	}
	backup := path + ".1"
	os.Remove(backup)
	os.Rename(path, backup)
}
