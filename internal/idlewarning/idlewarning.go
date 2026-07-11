// Package idlewarning displays an in-app, non-activating idle warning.
package idlewarning

import (
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/JeffioZ/idletrigger/internal/systray"
	"github.com/JeffioZ/idletrigger/internal/themeswitch"
)

type rect struct{ Left, Top, Right, Bottom int32 }
type point struct{ X, Y int32 }

type wndClassEx struct {
	Size, Style              uint32
	WndProc                  uintptr
	ClsExtra, WndExtra       int32
	Instance                 windows.Handle
	Icon, Cursor, Background windows.Handle
	MenuName, ClassName      *uint16
	IconSm                   windows.Handle
}

type monitorInfo struct {
	Size          uint32
	Monitor, Work rect
	Flags         uint32
}

// paintStruct is an opaque buffer large enough for PAINTSTRUCT on 32-bit and
// 64-bit Windows. Only BeginPaint/EndPaint consume it, so this avoids Go
// struct-alignment differences between architectures.
type paintStruct struct{ data [72]byte }

const (
	warningClass = "IdleTriggerIdleWarning"

	wmDestroy      = 0x0002
	wmClose        = 0x0010
	wmPaint        = 0x000F
	wmEraseBkgnd   = 0x0014
	wmLButtonUp    = 0x0202
	wmMouseMove    = 0x0200
	wmMouseLeave   = 0x02A3
	wsPopup        = 0x80000000
	wsBorder       = 0x00800000
	wsExTool       = 0x00000080
	wsExTopmost    = 0x00000008
	wsExNoActivate = 0x08000000
	swpNoActivate  = 0x0010
	swpShowWindow  = 0x0040
	monitorNearest = 2
	dtLeft         = 0x00000000
	dtCenter       = 0x00000001
	dtVCenter      = 0x00000004
	dtWordBreak    = 0x00000010
	dtCalcRect     = 0x00000400
	transparent    = 1
	tmeLeave       = 0x00000002
)

var (
	user32 = windows.NewLazySystemDLL("user32.dll")
	gdi32  = windows.NewLazySystemDLL("gdi32.dll")

	pCreateWindowEx    = user32.NewProc("CreateWindowExW")
	pDestroyWindow     = user32.NewProc("DestroyWindow")
	pDefWindowProc     = user32.NewProc("DefWindowProcW")
	pRegisterClassEx   = user32.NewProc("RegisterClassExW")
	pGetCursorPos      = user32.NewProc("GetCursorPos")
	pMonitorFromWindow = user32.NewProc("MonitorFromWindow")
	pGetMonitorInfo    = user32.NewProc("GetMonitorInfoW")
	pSetWindowPos      = user32.NewProc("SetWindowPos")
	pUpdateWindow      = user32.NewProc("UpdateWindow")
	pBeginPaint        = user32.NewProc("BeginPaint")
	pEndPaint          = user32.NewProc("EndPaint")
	pGetClientRect     = user32.NewProc("GetClientRect")
	pGetDC             = user32.NewProc("GetDC")
	pReleaseDC         = user32.NewProc("ReleaseDC")
	pInvalidateRect    = user32.NewProc("InvalidateRect")
	pTrackMouseEvent   = user32.NewProc("TrackMouseEvent")
	pFillRect          = user32.NewProc("FillRect")
	pDrawText          = user32.NewProc("DrawTextW")
	pSetTextColor      = gdi32.NewProc("SetTextColor")
	pSetBkMode         = gdi32.NewProc("SetBkMode")
	pGetDpiForWindow   = user32.NewProc("GetDpiForWindow")
	pDeleteObject      = gdi32.NewProc("DeleteObject")
	pCreateBrush       = gdi32.NewProc("CreateSolidBrush")
	pCreateFont        = gdi32.NewProc("CreateFontIndirectW")
	pSelectObject      = gdi32.NewProc("SelectObject")

	classOnce sync.Once
	classErr  error
	active    windows.Handle
	title     string
	body      string
	closeHot  bool
	dismissMu sync.RWMutex
	onDismiss func()

	warningProc = windows.NewCallback(wndProc)
)

type trackMouseEvent struct {
	Size      uint32
	Flags     uint32
	HwndTrack windows.Handle
	HoverTime uint32
}

// Show schedules a warning on the tray UI thread. It never activates the
// window, so displaying it does not itself reset the user's idle time.
func Show(titleText, bodyText string) {
	if !systray.Post(func() {
		showNow(titleText, bodyText)
	}) {
		return
	}
}

// SetOnDismiss sets the callback used when the user explicitly closes the warning.
func SetOnDismiss(fn func()) {
	dismissMu.Lock()
	onDismiss = fn
	dismissMu.Unlock()
}

// Hide closes the currently displayed warning. It must run on the tray UI thread.
func Hide() {
	if active != 0 {
		pDestroyWindow.Call(uintptr(active))
	}
	closeHot = false
}

func showNow(titleText, bodyText string) {
	Hide()
	if err := ensureClass(); err != nil {
		return
	}
	class, _ := windows.UTF16PtrFromString(warningClass)
	caption, _ := windows.UTF16PtrFromString("IdleTrigger")
	var cursor point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&cursor)))
	hwnd, _, _ := pCreateWindowEx.Call(
		wsExTool|wsExTopmost|wsExNoActivate,
		uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(caption)), wsPopup|wsBorder,
		uintptr(cursor.X), uintptr(cursor.Y), 1, 1, 0, 0, 0, 0,
	)
	if hwnd == 0 {
		return
	}
	active = windows.Handle(hwnd)
	title, body = titleText, bodyText
	position(active)
}

func ensureClass() error {
	classOnce.Do(func() {
		name, err := windows.UTF16PtrFromString(warningClass)
		if err != nil {
			classErr = err
			return
		}
		wc := wndClassEx{Size: uint32(unsafe.Sizeof(wndClassEx{})), WndProc: warningProc, ClassName: name}
		result, _, callErr := pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
		if result == 0 && callErr != windows.ERROR_CLASS_ALREADY_EXISTS {
			classErr = fmt.Errorf("register idle warning class: %w", callErr)
		}
	})
	return classErr
}

func position(hwnd windows.Handle) {
	sc := func(v int32) int32 { return scaleForWindow(hwnd, v) }
	width, margin := sc(348), sc(16)
	bodyHeight := measureText(hwnd, body, sc(13), 400, width-2*margin)
	height := sc(48) + bodyHeight + sc(14)
	if minimum := sc(112); height < minimum {
		height = minimum
	}
	monitor, _, _ := pMonitorFromWindow.Call(uintptr(hwnd), monitorNearest)
	info := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if monitor != 0 {
		pGetMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info)))
	}
	x := info.Work.Right - width - margin
	y := info.Work.Bottom - height - margin
	pSetWindowPos.Call(uintptr(hwnd), ^uintptr(0), uintptr(x), uintptr(y), uintptr(width), uintptr(height), swpNoActivate|swpShowWindow)
	pUpdateWindow.Call(uintptr(hwnd))
}

func measureText(hwnd windows.Handle, text string, size, weight, maxWidth int32) int32 {
	if text == "" || maxWidth <= 0 {
		return 0
	}
	dc, _, _ := pGetDC.Call(uintptr(hwnd))
	if dc == 0 {
		return 0
	}
	defer pReleaseDC.Call(uintptr(hwnd), dc)
	font := makeFont(size, weight)
	if font == 0 {
		return 0
	}
	defer pDeleteObject.Call(uintptr(font))
	old, _, _ := pSelectObject.Call(dc, uintptr(font))
	defer pSelectObject.Call(dc, old)
	value, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return 0
	}
	bounds := rect{Right: maxWidth}
	pDrawText.Call(dc, uintptr(unsafe.Pointer(value)), ^uintptr(0), uintptr(unsafe.Pointer(&bounds)), dtLeft|dtWordBreak|dtCalcRect)
	return bounds.Bottom - bounds.Top
}

func scaleForWindow(hwnd windows.Handle, v int32) int32 {
	dpi, _, _ := pGetDpiForWindow.Call(uintptr(hwnd))
	if dpi == 0 {
		return v
	}
	return int32(float64(v)*float64(dpi)/96 + 0.5)
}

func wndProc(hwnd windows.Handle, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case wmPaint:
		paint(hwnd)
		return 0
	case wmEraseBkgnd:
		return 1
	case wmClose:
		dismiss()
		return 0
	case wmLButtonUp:
		point := point{X: int32(int16(lParam)), Y: int32(int16(lParam >> 16))}
		if pointInRect(point, closeRect(hwnd)) {
			dismiss()
		}
		return 0
	case wmMouseMove:
		point := point{X: int32(int16(lParam)), Y: int32(int16(lParam >> 16))}
		setCloseHot(hwnd, pointInRect(point, closeRect(hwnd)))
		track := trackMouseEvent{Size: uint32(unsafe.Sizeof(trackMouseEvent{})), Flags: tmeLeave, HwndTrack: hwnd}
		pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&track)))
		return 0
	case wmMouseLeave:
		setCloseHot(hwnd, false)
		return 0
	case wmDestroy:
		if active == hwnd {
			active = 0
		}
		return 0
	}
	result, _, _ := pDefWindowProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return result
}

func setCloseHot(hwnd windows.Handle, value bool) {
	if closeHot == value {
		return
	}
	closeHot = value
	pInvalidateRect.Call(uintptr(hwnd), 0, 0)
}

func dismiss() {
	Hide()
	dismissMu.RLock()
	fn := onDismiss
	dismissMu.RUnlock()
	if fn != nil {
		fn()
	}
}

func paint(hwnd windows.Handle) {
	var ps paintStruct
	dc, _, _ := pBeginPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
	if dc == 0 {
		return
	}
	defer pEndPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
	var client rect
	pGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&client)))
	dark := themeswitch.Current() == themeswitch.ModeDark
	background, foreground, accent, closeHover := rgb(250, 251, 252), rgb(28, 33, 39), rgb(0, 112, 145), rgb(232, 236, 241)
	if dark {
		background, foreground, accent, closeHover = rgb(35, 39, 45), rgb(244, 247, 250), rgb(81, 225, 211), rgb(66, 72, 80)
	}
	brush := makeBrush(background)
	defer pDeleteObject.Call(uintptr(brush))
	pFillRect.Call(dc, uintptr(unsafe.Pointer(&client)), uintptr(brush))
	accentRect := client
	accentRect.Bottom = accentRect.Top + 4
	accentBrush := makeBrush(accent)
	pFillRect.Call(dc, uintptr(unsafe.Pointer(&accentRect)), uintptr(accentBrush))
	pDeleteObject.Call(uintptr(accentBrush))
	pSetTextColor.Call(dc, uintptr(foreground))
	pSetBkMode.Call(dc, transparent)
	sc := func(v int32) int32 { return scaleForWindow(hwnd, v) }
	margin := sc(16)
	close := closeRect(hwnd)
	if closeHot {
		closeBrush := makeBrush(closeHover)
		pFillRect.Call(dc, uintptr(unsafe.Pointer(&close)), uintptr(closeBrush))
		pDeleteObject.Call(uintptr(closeBrush))
	}
	drawText(dc, title, rect{Left: margin, Top: sc(17), Right: close.Left - sc(6), Bottom: sc(43)}, sc(15), 600, dtLeft)
	drawText(dc, body, rect{Left: margin, Top: sc(48), Right: client.Right - margin, Bottom: client.Bottom - sc(14)}, sc(13), 400, dtLeft|dtWordBreak)
	drawText(dc, "\u00d7", close, sc(18), 400, dtCenter|dtVCenter)
}

func closeRect(hwnd windows.Handle) rect {
	var client rect
	pGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&client)))
	size := scaleForWindow(hwnd, 28)
	margin := scaleForWindow(hwnd, 8)
	return rect{Left: client.Right - margin - size, Top: margin, Right: client.Right - margin, Bottom: margin + size}
}

func pointInRect(point point, bounds rect) bool {
	return point.X >= bounds.Left && point.X < bounds.Right && point.Y >= bounds.Top && point.Y < bounds.Bottom
}

func drawText(dc uintptr, text string, bounds rect, size, weight int32, format uintptr) {
	ptr, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return
	}
	font := makeFont(size, weight)
	if font != 0 {
		old, _, _ := pSelectObject.Call(dc, uintptr(font))
		defer pSelectObject.Call(dc, old)
		defer pDeleteObject.Call(uintptr(font))
	}
	pDrawText.Call(dc, uintptr(unsafe.Pointer(ptr)), ^uintptr(0), uintptr(unsafe.Pointer(&bounds)), format)
}

func makeFont(size, weight int32) windows.Handle {
	type logFont struct {
		Height, Width, Escapement, Orientation, Weight       int32
		Italic, Underline, StrikeOut, CharSet                byte
		OutPrecision, ClipPrecision, Quality, PitchAndFamily byte
		FaceName                                             [32]uint16
	}
	lf := logFont{Height: -size, Weight: weight, CharSet: 1}
	copy(lf.FaceName[:], windows.StringToUTF16("Microsoft YaHei UI"))
	result, _, _ := pCreateFont.Call(uintptr(unsafe.Pointer(&lf)))
	return windows.Handle(result)
}

func makeBrush(color uint32) windows.Handle {
	result, _, _ := pCreateBrush.Call(uintptr(color))
	return windows.Handle(result)
}

func rgb(r, g, b byte) uint32 { return uint32(r) | uint32(g)<<8 | uint32(b)<<16 }
