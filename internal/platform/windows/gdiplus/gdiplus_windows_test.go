//go:build windows

package gdiplus

import (
	"errors"
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
	var stopped lifecycleSession
	m := newLifecycle(func() (lifecycleSession, bool) {
		starts++
		return lifecycleSession{gdiplusToken: 7, notificationToken: 8, notificationUnhook: 9}, true
	}, func(session lifecycleSession) {
		stops++
		stopped = session
	})
	if !m.start() {
		t.Fatal("first Start failed")
	}
	if !m.start() {
		t.Fatal("idempotent Start failed")
	}
	if starts != 1 {
		t.Fatalf("start calls=%d", starts)
	}
	m.close()
	m.close()
	if stops != 1 || m.start() {
		t.Fatalf("stops=%d; Start after close must fail", stops)
	}
	if stopped.gdiplusToken != 7 || stopped.notificationToken != 8 || stopped.notificationUnhook != 9 {
		t.Fatalf("shutdown session=%+v, want all startup and notification tokens", stopped)
	}
}

func TestShutdownWaitsAndRejectsNewDrawing(t *testing.T) {
	stopped := make(chan struct{})
	m := newLifecycle(func() (lifecycleSession, bool) { return lifecycleSession{gdiplusToken: 7}, true }, func(lifecycleSession) { close(stopped) })
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
	m := newLifecycle(func() (lifecycleSession, bool) { starts++; return lifecycleSession{}, false }, func(lifecycleSession) {})
	if m.start() {
		t.Fatal("first Start unexpectedly succeeded")
	}
	if m.start() {
		t.Fatal("failed Start was retried successfully")
	}
	if starts != 1 {
		t.Fatalf("starts=%d, want 1", starts)
	}
}

func TestLoadRequiredAPIRejectsLoadAndProcFailures(t *testing.T) {
	wantErr := errors.New("unavailable")
	finderCalled := false
	if loadRequiredAPI(func() error { return wantErr }, func() error {
		finderCalled = true
		return nil
	}) {
		t.Fatal("DLL load failure unexpectedly succeeded")
	}
	if finderCalled {
		t.Fatal("procedure lookup ran after DLL load failure")
	}

	calls := 0
	if loadRequiredAPI(
		func() error { return nil },
		func() error { calls++; return nil },
		func() error { calls++; return wantErr },
		func() error { calls++; return nil },
	) {
		t.Fatal("procedure lookup failure unexpectedly succeeded")
	}
	if calls != 2 {
		t.Fatalf("procedure lookup calls=%d, want 2", calls)
	}
}

func TestLoadRequiredAPIAcceptsCompleteAPI(t *testing.T) {
	calls := 0
	if !loadRequiredAPI(
		func() error { calls++; return nil },
		func() error { calls++; return nil },
		func() error { calls++; return nil },
	) {
		t.Fatal("complete API was rejected")
	}
	if calls != 3 {
		t.Fatalf("calls=%d, want 3", calls)
	}
}

func TestConcurrentStartDrawingAndShutdown(t *testing.T) {
	var starts, stops int
	m := newLifecycle(func() (lifecycleSession, bool) { starts++; return lifecycleSession{gdiplusToken: 7}, true }, func(lifecycleSession) { stops++ })
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
	m := newLifecycle(func() (lifecycleSession, bool) { return lifecycleSession{gdiplusToken: 7}, true }, func(lifecycleSession) {})
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
	m := newLifecycle(func() (lifecycleSession, bool) { return lifecycleSession{gdiplusToken: 7}, true }, func(lifecycleSession) {})
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
	m := newLifecycle(func() (lifecycleSession, bool) { return lifecycleSession{gdiplusToken: 7}, true }, func(lifecycleSession) {})
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

func TestRoundedRectUsesWindingPathsAndOnePixelInset(t *testing.T) {
	m := newLifecycle(func() (lifecycleSession, bool) { return lifecycleSession{gdiplusToken: 7}, true }, func(lifecycleSession) {})
	if !m.start() {
		t.Fatal("Start failed")
	}
	var modes []int
	var lines [][4]int32
	fillCalls := 0
	next := uintptr(10)
	a := roundedRectTestAPI(&next, func(graphics, brush, path uintptr) bool {
		fillCalls++
		return true
	})
	a.createPath = func(mode int) (uintptr, bool) {
		modes = append(modes, mode)
		next++
		return next, true
	}
	a.addPathLine = func(_ uintptr, x1, y1, x2, y2 int32) bool {
		lines = append(lines, [4]int32{x1, y1, x2, y2})
		return true
	}
	if got := fillRoundedRectWith(m, a, 1, roundedRect{left: 10, top: 8, right: 70, bottom: 42}, 3, 0x00ffffff, 0x00d0d0d0); got != DrawCompleted {
		t.Fatalf("result=%d, want DrawCompleted", got)
	}
	if len(modes) != 2 || modes[0] != FillModeWinding || modes[1] != FillModeWinding {
		t.Fatalf("path fill modes=%v, want winding", modes)
	}
	if fillCalls != 2 {
		t.Fatalf("fill calls=%d, want 2", fillCalls)
	}
	if len(lines) < 5 {
		t.Fatalf("line count=%d, want at least outer and inner top lines", len(lines))
	}
	if lines[0] != [4]int32{13, 8, 67, 8} || lines[4] != [4]int32{13, 9, 67, 9} {
		t.Fatalf("outer/inner top lines=%v/%v, want one-pixel inset", lines[0], lines[4])
	}
}

func TestRoundedRectSetupFailureDoesNotDraw(t *testing.T) {
	m := newLifecycle(func() (lifecycleSession, bool) { return lifecycleSession{gdiplusToken: 7}, true }, func(lifecycleSession) {})
	if !m.start() {
		t.Fatal("Start failed")
	}
	next := uintptr(10)
	drawn := false
	a := roundedRectTestAPI(&next, func(uintptr, uintptr, uintptr) bool { drawn = true; return true })
	a.createPath = func(int) (uintptr, bool) { return 0, false }
	if got := fillRoundedRectWith(m, a, 1, roundedRect{left: 10, top: 8, right: 70, bottom: 42}, 3, 0, 0); got != DrawNotStarted {
		t.Fatalf("result=%d, want DrawNotStarted", got)
	}
	if drawn {
		t.Fatal("setup failure reached FillPath")
	}
}

func TestRoundedRectFinalFailureIsMarkedMayBeDirty(t *testing.T) {
	m := newLifecycle(func() (lifecycleSession, bool) { return lifecycleSession{gdiplusToken: 7}, true }, func(lifecycleSession) {})
	if !m.start() {
		t.Fatal("Start failed")
	}
	next := uintptr(10)
	calls := 0
	a := roundedRectTestAPI(&next, func(uintptr, uintptr, uintptr) bool {
		calls++
		return calls != 2
	})
	if got := fillRoundedRectWith(m, a, 1, roundedRect{left: 10, top: 8, right: 70, bottom: 42}, 3, 0, 0); got != DrawMayBeDirty {
		t.Fatalf("result=%d, want DrawMayBeDirty", got)
	}
}

func TestRoundedRectRejectsEmptyAndTooSmallBounds(t *testing.T) {
	m := newLifecycle(func() (lifecycleSession, bool) { return lifecycleSession{gdiplusToken: 7}, true }, func(lifecycleSession) {})
	if !m.start() {
		t.Fatal("Start failed")
	}
	next := uintptr(10)
	drawn := false
	a := roundedRectTestAPI(&next, func(uintptr, uintptr, uintptr) bool { drawn = true; return true })
	for _, bounds := range []roundedRect{
		{left: 5, top: 5, right: 5, bottom: 10},
		{left: 5, top: 5, right: 4, bottom: 10},
		{left: 5, top: 5, right: 7, bottom: 10},
		{left: 5, top: 5, right: 10, bottom: 7},
	} {
		if got := fillRoundedRectWith(m, a, 1, bounds, 1, 0, 0); got != DrawNotStarted {
			t.Fatalf("bounds=%#v result=%d, want DrawNotStarted", bounds, got)
		}
	}
	if drawn {
		t.Fatal("invalid bounds reached FillPath")
	}
}

func TestRoundedRectRadiusIsClampedAndInset(t *testing.T) {
	m := newLifecycle(func() (lifecycleSession, bool) { return lifecycleSession{gdiplusToken: 7}, true }, func(lifecycleSession) {})
	if !m.start() {
		t.Fatal("Start failed")
	}
	next := uintptr(10)
	beziers := 0
	a := roundedRectTestAPI(&next, func(uintptr, uintptr, uintptr) bool { return true })
	a.addPathBezier = func(_ uintptr, x1, y1, x2, y2, x3, y3, x4, y4 int32) bool {
		beziers++
		for _, value := range []int32{x1, y1, x2, y2, x3, y3, x4, y4} {
			if value < 0 || value > 10 {
				t.Fatalf("clamped Bezier coordinate=%d outside outer bounds", value)
			}
		}
		return true
	}
	if got := fillRoundedRectWith(m, a, 1, roundedRect{left: 0, top: 0, right: 10, bottom: 6}, 99, 0, 0); got != DrawCompleted {
		t.Fatalf("large radius result=%d, want DrawCompleted", got)
	}
	if beziers != 8 { // four outer corners and four inner corners
		t.Fatalf("Bezier count=%d, want 8", beziers)
	}
	beziers = 0
	if got := fillRoundedRectWith(m, a, 1, roundedRect{left: 0, top: 0, right: 3, bottom: 3}, 0, 0, 0); got != DrawCompleted {
		t.Fatalf("minimum valid bounds result=%d, want DrawCompleted", got)
	}
	if beziers != 0 {
		t.Fatalf("zero radius used %d Bezier curves", beziers)
	}
}

func roundedRectTestAPI(next *uintptr, fill func(uintptr, uintptr, uintptr) bool) gdiAPI {
	return gdiAPI{
		createFromHDC:      func(windows.Handle) (uintptr, bool) { return 9, true },
		deleteGraphics:     func(uintptr) {},
		setSmoothingMode:   func(uintptr) bool { return true },
		setPixelOffsetMode: func(uintptr) bool { return true },
		createSolidFill: func(uint32) (uintptr, bool) {
			*next++
			return *next, true
		},
		deleteBrush:     func(uintptr) {},
		createPath:      func(int) (uintptr, bool) { *next++; return *next, true },
		deletePath:      func(uintptr) {},
		startPathFigure: func(uintptr) bool { return true },
		closePathFigure: func(uintptr) bool { return true },
		addPathLine:     func(uintptr, int32, int32, int32, int32) bool { return true },
		addPathBezier:   func(uintptr, int32, int32, int32, int32, int32, int32, int32, int32) bool { return true },
		fillPath:        fill,
	}
}

func TestARGBConvertsWin32ColorRef(t *testing.T) {
	if got, want := argb(0x00563412), uint32(0xff123456); got != want {
		t.Fatalf("argb() = %#x, want %#x", got, want)
	}
}
