//go:build devtools

package main

import (
	"errors"
	"testing"
)

func TestRunScreenshotFailureShutsDownBeforeReturning(t *testing.T) {
	shutdown := false
	if got := runScreenshotWith(nil, func() bool { return true }, func() { shutdown = true }, func([]string) error { return errors.New("failed") }, func() {}); got != 1 {
		t.Fatalf("runScreenshotWith failure = %d, want 1", got)
	}
	if !shutdown {
		t.Fatal("Shutdown was not called before the failure result")
	}
}
