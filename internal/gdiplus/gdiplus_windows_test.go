//go:build windows

package gdiplus

import (
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestFillModeConstantsMatchSDK(t *testing.T) {
	if FillModeAlternate != 0 || FillModeWinding != 1 {
		t.Fatalf("fill modes = alternate:%d winding:%d, want 0/1", FillModeAlternate, FillModeWinding)
	}
}

func TestLifecycleStartAndShutdownAreIdempotent(t *testing.T) {
	var starts, stops int
	m := newLifecycle(func() (uintptr, bool) { starts++; return 7, true }, func(uintptr) { stops++ })
	if !m.start() || !m.start() || starts != 1 {
		t.Fatalf("start calls=%d", starts)
	}
	m.close()
	m.close()
	if stops != 1 || m.start() {
		t.Fatalf("stops=%d; Start after close must fail", stops)
	}
}

func TestShutdownWaitsAndRejectsNewDrawing(t *testing.T) {
	stopped := make(chan struct{})
	m := newLifecycle(func() (uintptr, bool) { return 7, true }, func(uintptr) { close(stopped) })
	if !m.start() {
		t.Fatal("Start failed")
	}
	end, ok := m.startDrawing()
	if !ok {
		t.Fatal("startDrawing failed")
	}
	done := make(chan struct{})
	go func() { m.close(); close(done) }()
	time.Sleep(20 * time.Millisecond)
	if _, ok := m.startDrawing(); ok {
		t.Fatal("drawing entered while closing")
	}
	select {
	case <-done:
		t.Fatal("Shutdown did not wait")
	default:
	}
	end()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Shutdown did not run")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Shutdown did not return")
	}
}

func TestStartupFailureIsNotRetried(t *testing.T) {
	starts := 0
	m := newLifecycle(func() (uintptr, bool) { starts++; return 0, false }, func(uintptr) {})
	if m.start() || m.start() || starts != 1 {
		t.Fatalf("starts=%d, want 1", starts)
	}
}

func TestConcurrentStartDrawingAndShutdown(t *testing.T) {
	var starts, stops int
	m := newLifecycle(func() (uintptr, bool) { starts++; return 7, true }, func(uintptr) { stops++ })
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() { defer wg.Done(); m.start() }()
	}
	started := make(chan struct{})
	go func() { wg.Wait(); close(started) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("concurrent Start calls did not finish")
	}
	if starts != 1 {
		t.Fatalf("starts=%d, want 1", starts)
	}
	end, ok := m.startDrawing()
	if !ok {
		t.Fatal("startDrawing failed")
	}
	wg.Add(1)
	go func() { defer wg.Done(); m.close() }()
	end()
	closed := make(chan struct{})
	go func() { wg.Wait(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("concurrent Shutdown did not finish")
	}
	if stops != 1 {
		t.Fatalf("stops=%d, want 1", stops)
	}
}

func TestSetupFailureDoesNotDraw(t *testing.T) {
	m := newLifecycle(func() (uintptr, bool) { return 7, true }, func(uintptr) {})
	if !m.start() {
		t.Fatal("Start failed")
	}
	filled := false
	deleted := false
	a := gdiAPI{
		createFromHDC:      func(windows.Handle) (uintptr, bool) { return 9, true },
		deleteGraphics:     func(uintptr) { deleted = true },
		setSmoothingMode:   func(uintptr) bool { return false },
		setPixelOffsetMode: func(uintptr) bool { t.Fatal("pixel offset should not run"); return true },
		createSolidFill:    func(uint32) (uintptr, bool) { t.Fatal("brush should not be created"); return 0, false },
		deleteBrush:        func(uintptr) {},
		fillPolygon:        func(uintptr, uintptr, []Point, int) bool { filled = true; return true },
	}
	if got := fillPolygonWith(m, a, 1, []Point{{X: 0, Y: 0}, {X: 2, Y: 0}, {X: 1, Y: 2}}, 0); got != DrawNotStarted {
		t.Fatalf("result=%d, want DrawNotStarted", got)
	}
	if filled || !deleted {
		t.Fatalf("filled=%v deleted=%v", filled, deleted)
	}
}

func TestPixelOffsetFailureDoesNotDraw(t *testing.T) {
	m := newLifecycle(func() (uintptr, bool) { return 7, true }, func(uintptr) {})
	if !m.start() {
		t.Fatal("Start failed")
	}
	filled := false
	a := gdiAPI{
		createFromHDC:      func(windows.Handle) (uintptr, bool) { return 9, true },
		deleteGraphics:     func(uintptr) {},
		setSmoothingMode:   func(uintptr) bool { return true },
		setPixelOffsetMode: func(uintptr) bool { return false },
		createSolidFill:    func(uint32) (uintptr, bool) { t.Fatal("brush should not be created"); return 0, false },
		deleteBrush:        func(uintptr) {},
		fillPolygon:        func(uintptr, uintptr, []Point, int) bool { filled = true; return true },
	}
	if got := fillPolygonWith(m, a, 1, []Point{{X: 0, Y: 0}, {X: 2, Y: 0}, {X: 1, Y: 2}}, 0); got != DrawNotStarted || filled {
		t.Fatalf("result=%d filled=%v, want DrawNotStarted/false", got, filled)
	}
}

func TestFinalDrawFailureIsMarkedMayBeDirty(t *testing.T) {
	m := newLifecycle(func() (uintptr, bool) { return 7, true }, func(uintptr) {})
	if !m.start() {
		t.Fatal("Start failed")
	}
	a := gdiAPI{
		createFromHDC:      func(windows.Handle) (uintptr, bool) { return 9, true },
		deleteGraphics:     func(uintptr) {},
		setSmoothingMode:   func(uintptr) bool { return true },
		setPixelOffsetMode: func(uintptr) bool { return true },
		createSolidFill:    func(uint32) (uintptr, bool) { return 11, true },
		deleteBrush:        func(uintptr) {},
		fillPolygon:        func(uintptr, uintptr, []Point, int) bool { return false },
	}
	if got := fillPolygonWith(m, a, 1, []Point{{X: 0, Y: 0}, {X: 2, Y: 0}, {X: 1, Y: 2}}, 0); got != DrawMayBeDirty {
		t.Fatalf("result=%d, want DrawMayBeDirty", got)
	}
}

func TestARGBConvertsWin32ColorRef(t *testing.T) {
	if got, want := argb(0x00563412), uint32(0xff123456); got != want {
		t.Fatalf("argb() = %#x, want %#x", got, want)
	}
}
