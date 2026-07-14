package logging

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInfoWritesSessionLines(t *testing.T) {
	dir := t.TempDir()
	Init(true, dir)
	Info("hello %s", "world")
	Close()

	text := readLog(t, filepath.Join(dir, "IdleTrigger.log"))
	for _, want := range []string{
		"=== IdleTrigger session started ===",
		"hello world",
		"=== IdleTrigger session ended ===",
		"[session:",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("log missing %q:\n%s", want, text)
		}
	}
}

func TestDisabledLoggingDoesNotCreateFile(t *testing.T) {
	dir := t.TempDir()
	Init(false, dir)
	Info("should not be written")
	Close()

	if _, err := os.Stat(filepath.Join(dir, "IdleTrigger.log")); !os.IsNotExist(err) {
		t.Fatalf("disabled logging created file or returned unexpected error: %v", err)
	}
}

func BenchmarkInfoDisabled(b *testing.B) {
	Init(false, b.TempDir())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Info("idle monitor sample: idle=%s threshold=%s tick=%d", time.Second, time.Minute, i)
	}
}

func TestInitRotatesLargeLog(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "IdleTrigger.log")
	old := bytes.Repeat([]byte("x"), maxLogSize+1)
	if err := os.WriteFile(logPath, old, 0o644); err != nil {
		t.Fatalf("seed large log: %v", err)
	}

	Init(true, dir)
	Info("fresh session")
	Close()

	backup := readLog(t, logPath+".1")
	if len(backup) != len(old) {
		t.Fatalf("rotated backup length = %d, want %d", len(backup), len(old))
	}
	current := readLog(t, logPath)
	if strings.Contains(current, strings.Repeat("x", 64)) {
		t.Fatalf("current log still contains old rotated content")
	}
	if !strings.Contains(current, "fresh session") {
		t.Fatalf("current log missing fresh session line:\n%s", current)
	}
}

func TestInitClosesPreviousSession(t *testing.T) {
	dir := t.TempDir()
	Init(true, dir)
	Info("first")
	Init(true, dir)
	Info("second")
	Close()

	text := readLog(t, filepath.Join(dir, "IdleTrigger.log"))
	if strings.Count(text, "=== IdleTrigger session started ===") != 2 {
		t.Fatalf("expected two session starts:\n%s", text)
	}
	if strings.Count(text, "=== IdleTrigger session ended ===") != 2 {
		t.Fatalf("expected two session ends:\n%s", text)
	}
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
