package main

import (
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
