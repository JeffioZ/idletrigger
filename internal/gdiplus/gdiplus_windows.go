//go:build windows

// Package gdiplus provides the small anti-aliased vector primitive used by
// native owner-drawn controls. It never owns an HDC or renders text.
package gdiplus

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	FillModeAlternate = 0
	FillModeWinding   = 1

	smoothingModeAntiAlias = 4
	pixelOffsetModeHalf    = 4
)

type Point struct{ X, Y int32 }

// DrawResult distinguishes a setup failure, for which GDI can safely draw on
// the existing HDC, from a final GDI+ call that may have changed pixels.
type DrawResult uint8

const (
	DrawNotStarted DrawResult = iota
	DrawCompleted
	DrawMayBeDirty
)

type startupInput struct {
	Version                  uint32
	DebugEventCallback       uintptr
	SuppressBackgroundThread uint32
	SuppressExternalCodecs   uint32
}

type lifecycleState uint8

const (
	stateNew lifecycleState = iota
	stateReady
	stateClosing
	stateClosed
	stateFailed
)

type lifecycleManager struct {
	mu       sync.Mutex
	changed  *sync.Cond
	state    lifecycleState
	token    uintptr
	active   int
	startup  func() (uintptr, bool)
	shutdown func(uintptr)
}

func newLifecycle(startup func() (uintptr, bool), shutdown func(uintptr)) *lifecycleManager {
	m := &lifecycleManager{startup: startup, shutdown: shutdown}
	m.changed = sync.NewCond(&m.mu)
	return m
}

func (m *lifecycleManager) start() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == stateReady {
		return true
	}
	if m.state != stateNew {
		return false
	}
	token, ok := m.startup()
	if !ok || token == 0 {
		m.state = stateFailed
		return false
	}
	m.token, m.state = token, stateReady
	return true
}

func (m *lifecycleManager) startDrawing() (func(), bool) {
	m.mu.Lock()
	if m.state != stateReady {
		m.mu.Unlock()
		return nil, false
	}
	m.active++
	m.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			m.mu.Lock()
			m.active--
			if m.state == stateClosing && m.active == 0 {
				m.changed.Broadcast()
			}
			m.mu.Unlock()
		})
	}, true
}

func (m *lifecycleManager) close() {
	m.mu.Lock()
	for m.state == stateClosing {
		m.changed.Wait()
	}
	if m.state == stateClosed || m.state == stateFailed {
		m.mu.Unlock()
		return
	}
	if m.state == stateNew {
		m.state = stateClosed
		m.mu.Unlock()
		return
	}
	m.state = stateClosing
	for m.active != 0 {
		m.changed.Wait()
	}
	token := m.token
	m.token = 0
	m.mu.Unlock()

	m.shutdown(token)

	m.mu.Lock()
	m.state = stateClosed
	m.changed.Broadcast()
	m.mu.Unlock()
}

type gdiAPI struct {
	createFromHDC      func(windows.Handle) (uintptr, bool)
	deleteGraphics     func(uintptr)
	setSmoothingMode   func(uintptr) bool
	setPixelOffsetMode func(uintptr) bool
	createSolidFill    func(uint32) (uintptr, bool)
	deleteBrush        func(uintptr)
	fillPolygon        func(uintptr, uintptr, []Point, int) bool
}

var (
	dll = windows.NewLazySystemDLL("gdiplus.dll")

	pStartup            = dll.NewProc("GdiplusStartup")
	pShutdown           = dll.NewProc("GdiplusShutdown")
	pCreateFromHDC      = dll.NewProc("GdipCreateFromHDC")
	pDeleteGraphics     = dll.NewProc("GdipDeleteGraphics")
	pSetSmoothingMode   = dll.NewProc("GdipSetSmoothingMode")
	pSetPixelOffsetMode = dll.NewProc("GdipSetPixelOffsetMode")
	pCreateSolidFill    = dll.NewProc("GdipCreateSolidFill")
	pDeleteBrush        = dll.NewProc("GdipDeleteBrush")
	pFillPolygonI       = dll.NewProc("GdipFillPolygonI")

	lifecycle = newLifecycle(startup, shutdown)
	api       = gdiAPI{
		createFromHDC:      createFromHDC,
		deleteGraphics:     deleteGraphics,
		setSmoothingMode:   setSmoothingMode,
		setPixelOffsetMode: setPixelOffsetMode,
		createSolidFill:    createSolidFill,
		deleteBrush:        deleteBrush,
		fillPolygon:        fillPolygon,
	}
)

func Start() bool { return lifecycle.start() }

func Shutdown() { lifecycle.close() }

// FillPolygon draws one compact anti-aliased vector icon. A caller must fully
// repaint its control in GDI when DrawMayBeDirty is returned.
func FillPolygon(hdc windows.Handle, points []Point, color uint32) DrawResult {
	return fillPolygonWith(lifecycle, api, hdc, points, color)
}

// DrawCheck preserves the existing checkbox footprint while rendering the
// diagonal mark as a filled anti-aliased polygon.
func DrawCheck(hdc windows.Handle, left, top, right, bottom int32, color uint32, width int32) DrawResult {
	w, h := right-left, bottom-top
	if w <= 0 || h <= 0 || width <= 0 {
		return DrawNotStarted
	}
	x1, y1 := left+w*24/100, top+h*51/100
	x2, y2 := left+w*43/100, top+h*70/100
	x3, y3 := left+w*77/100, top+h*30/100
	return FillPolygon(hdc, []Point{{X: x1 - width/2, Y: y1}, {X: x2, Y: y2 + width/2}, {X: x3 + width/2, Y: y3}, {X: x3, Y: y3 - width/2}, {X: x2, Y: y2 - width/2}, {X: x1 + width/2, Y: y1 - width/2}}, color)
}

func fillPolygonWith(m *lifecycleManager, a gdiAPI, hdc windows.Handle, points []Point, color uint32) DrawResult {
	if hdc == 0 || len(points) < 3 {
		return DrawNotStarted
	}
	end, ok := m.startDrawing()
	if !ok {
		return DrawNotStarted
	}
	defer end()
	graphics, ok := a.createFromHDC(hdc)
	if !ok {
		return DrawNotStarted
	}
	defer a.deleteGraphics(graphics)
	if !a.setSmoothingMode(graphics) || !a.setPixelOffsetMode(graphics) {
		return DrawNotStarted
	}
	brush, ok := a.createSolidFill(argb(color))
	if !ok {
		return DrawNotStarted
	}
	defer a.deleteBrush(brush)
	if !a.fillPolygon(graphics, brush, points, FillModeWinding) {
		return DrawMayBeDirty
	}
	return DrawCompleted
}

func startup() (uintptr, bool) {
	input := startupInput{Version: 1}
	var token uintptr
	status, _, _ := pStartup.Call(uintptr(unsafe.Pointer(&token)), uintptr(unsafe.Pointer(&input)), 0)
	return token, status == 0 && token != 0
}

func shutdown(token uintptr) { pShutdown.Call(token) }

func createFromHDC(hdc windows.Handle) (uintptr, bool) {
	var graphics uintptr
	status, _, _ := pCreateFromHDC.Call(uintptr(hdc), uintptr(unsafe.Pointer(&graphics)))
	return graphics, status == 0 && graphics != 0
}

func deleteGraphics(graphics uintptr) { pDeleteGraphics.Call(graphics) }

func setSmoothingMode(graphics uintptr) bool {
	status, _, _ := pSetSmoothingMode.Call(graphics, smoothingModeAntiAlias)
	return status == 0
}

func setPixelOffsetMode(graphics uintptr) bool {
	status, _, _ := pSetPixelOffsetMode.Call(graphics, pixelOffsetModeHalf)
	return status == 0
}

func createSolidFill(color uint32) (uintptr, bool) {
	var brush uintptr
	status, _, _ := pCreateSolidFill.Call(uintptr(color), uintptr(unsafe.Pointer(&brush)))
	return brush, status == 0 && brush != 0
}

func deleteBrush(brush uintptr) { pDeleteBrush.Call(brush) }

func fillPolygon(graphics, brush uintptr, points []Point, fillMode int) bool {
	status, _, _ := pFillPolygonI.Call(graphics, brush, uintptr(unsafe.Pointer(&points[0])), uintptr(len(points)), uintptr(fillMode))
	return status == 0
}

func argb(color uint32) uint32 {
	return 0xff000000 | ((color & 0x000000ff) << 16) | (color & 0x0000ff00) | ((color & 0x00ff0000) >> 16)
}
