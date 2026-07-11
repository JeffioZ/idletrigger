package main

import (
	"path/filepath"
	"testing"
)

func TestAppIconContainsTrayDPIFrames(t *testing.T) {
	frames, err := readFrames(filepath.Join("..", "..", "assets", "app.ico"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[byte]bool{16: false, 20: false, 24: false, 32: false, 40: false, 48: false, 64: false}
	for _, frame := range frames {
		if frame.width == frame.height {
			if _, ok := want[frame.width]; ok {
				want[frame.width] = true
			}
		}
	}
	for size, found := range want {
		if !found {
			t.Errorf("app.ico is missing the %dx%d tray frame", size, size)
		}
	}
}
