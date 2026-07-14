package main

import (
	"errors"
	"testing"

	"golang.org/x/sys/windows"
)

func TestUsableStdHandleRejectsInvalidHandles(t *testing.T) {
	for _, h := range []windows.Handle{0, windows.InvalidHandle} {
		if usableStdHandle(h) {
			t.Fatalf("usableStdHandle(%v) = true, want false", h)
		}
	}
}

func TestRunScreenshotFailureShutsDownBeforeReturning(t *testing.T) {
	shutdown := false
	if got := runScreenshotWith(nil, func() bool { return true }, func() { shutdown = true }, func([]string) error { return errors.New("failed") }, func() {}); got != 1 {
		t.Fatalf("runScreenshotWith failure = %d, want 1", got)
	}
	if !shutdown {
		t.Fatal("Shutdown was not called before the failure result")
	}
}
