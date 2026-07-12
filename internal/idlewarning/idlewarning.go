// Package idlewarning displays an in-app, non-activating idle warning.
package idlewarning

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	mylog "github.com/JeffioZ/idletrigger/internal/log"
	"github.com/JeffioZ/idletrigger/internal/systray"
	"github.com/JeffioZ/idletrigger/internal/themeswitch"
	"github.com/JeffioZ/idletrigger/internal/uicolors"
	"github.com/JeffioZ/idletrigger/internal/uifont"
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

	wmDestroy        = 0x0002
	wmClose          = 0x0010
	wmPaint          = 0x000F
	wmEraseBkgnd     = 0x0014
	wmLButtonDown    = 0x0201
	wmLButtonUp      = 0x0202
	wmMouseMove      = 0x0200
	wmMouseLeave     = 0x02A3
	wmDpiChanged     = 0x02E0
	wsPopup          = 0x80000000
	wsExTool         = 0x00000080
	wsExTopmost      = 0x00000008
	wsExNoActivate   = 0x08000000
	swpNoActivate    = 0x0010
	swpShowWindow    = 0x0040
	monitorNearest   = 2
	dtLeft           = 0x00000000
	dtCenter         = 0x00000001
	dtVCenter        = 0x00000004
	dtWordBreak      = 0x00000010
	dtCalcRect       = 0x00000400
	transparent      = 1
	tmeLeave         = 0x00000002
	warningMinHeight = 92
	// Replaces the former WS_BORDER non-client edge. Keeping this one physical
	// pixel inside the client rect preserves the old content and hit-test area.
	warningBorderInset = 1
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
	pFrameRect         = user32.NewProc("FrameRect")
	pDrawText          = user32.NewProc("DrawTextW")
	pSetTextColor      = gdi32.NewProc("SetTextColor")
	pSetBkMode         = gdi32.NewProc("SetBkMode")
	pGetDpiForWindow   = user32.NewProc("GetDpiForWindow")
	pDeleteObject      = gdi32.NewProc("DeleteObject")
	pCreateBrush       = gdi32.NewProc("CreateSolidBrush")
	pCreatePen         = gdi32.NewProc("CreatePen")
	pMoveToEx          = gdi32.NewProc("MoveToEx")
	pLineTo            = gdi32.NewProc("LineTo")
	pSelectObject      = gdi32.NewProc("SelectObject")

	classOnce  sync.Once
	classErr   error
	active     windows.Handle
	title      string
	body       string
	closeHot   bool
	closeDown  bool
	activeSeq  uint64
	titleFont  windows.Handle
	bodyFont   windows.Handle
	fontChoice uifont.Choice
	uiChinese  *bool
	nextSeq    atomic.Uint64
	dismissMu  sync.RWMutex
	onDismiss  func()
	languageMu sync.RWMutex

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
	seq := nextSeq.Add(1)
	if !systray.Post(func() {
		showNow(titleText, bodyText, seq)
	}) {
		return
	}
}

// ShowCountdown displays a warning and refreshes its body once per second.
// bodyForSecond receives the remaining second count, including the initial
// value. The warning remains non-activating; user activity is still observed by
// the idle monitor rather than by this window.
func ShowCountdown(titleText string, seconds int, bodyForSecond func(int) string) {
	if bodyForSecond == nil {
		return
	}
	if seconds < 0 {
		seconds = 0
	}
	seq := nextSeq.Add(1)
	if !systray.Post(func() {
		showNow(titleText, bodyForSecond(seconds), seq)
	}) {
		return
	}
	if seconds == 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for remaining := seconds - 1; remaining >= 0; remaining-- {
			<-ticker.C
			text := bodyForSecond(remaining)
			if !systray.Post(func() { updateBodyNow(seq, text) }) {
				return
			}
			if remaining == 0 {
				return
			}
		}
	}()
}

// SetOnDismiss sets the callback used when the user explicitly closes the warning.
func SetOnDismiss(fn func()) {
	dismissMu.Lock()
	onDismiss = fn
	dismissMu.Unlock()
}

// SetLanguage supplies the resolved application UI language before a warning
// is shown, including when it differs from the Windows display language.
func SetLanguage(chinese bool) {
	languageMu.Lock()
	uiChinese = &chinese
	languageMu.Unlock()
}

// Hide closes the currently displayed warning. It must run on the tray UI thread.
func Hide() {
	if active != 0 {
		pDestroyWindow.Call(uintptr(active))
	}
	activeSeq = 0
	closeHot = false
	closeDown = false
	for _, font := range []windows.Handle{titleFont, bodyFont} {
		if font != 0 {
			pDeleteObject.Call(uintptr(font))
		}
	}
	titleFont, bodyFont = 0, 0
}

func showNow(titleText, bodyText string, seq uint64) {
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
		uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(caption)), wsPopup,
		uintptr(cursor.X), uintptr(cursor.Y), 1, 1, 0, 0, 0, 0,
	)
	if hwnd == 0 {
		return
	}
	active = windows.Handle(hwnd)
	activeSeq = seq
	title, body = titleText, bodyText
	rebuildFonts(active)
	mylog.Info("UI font: surface=idle-warning ui_language=%s system_language=%s system_locale=%s face=%q reason=%s dpi=%d title_px=%d body_px=%d", fontChoice.UILanguage, fontChoice.SystemLanguage, fontChoice.SystemLocale, fontChoice.Face, fontChoice.Reason, dpiForWindow(active), scaledFontSize(active, 15), scaledFontSize(active, 13))
	position(active)
}

func updateBodyNow(seq uint64, bodyText string) {
	if active == 0 || activeSeq != seq {
		return
	}
	if body == bodyText {
		return
	}
	body = bodyText
	position(active)
	pInvalidateRect.Call(uintptr(active), 0, 0)
	pUpdateWindow.Call(uintptr(active))
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
	height := sc(44) + bodyHeight + sc(16)
	if minimum := sc(warningMinHeight); height < minimum {
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
	font := bodyFont
	if font == 0 {
		return 0
	}
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

func dpiForWindow(hwnd windows.Handle) uintptr {
	dpi, _, _ := pGetDpiForWindow.Call(uintptr(hwnd))
	if dpi == 0 {
		return 96
	}
	return dpi
}

func scaledFontSize(hwnd windows.Handle, size int32) int32 {
	return int32(float64(size)*float64(dpiForWindow(hwnd))/96 + 0.5)
}

func rebuildFonts(hwnd windows.Handle) {
	chinese := uifont.SystemLanguageIsChinese()
	languageMu.RLock()
	if uiChinese != nil {
		chinese = *uiChinese
	}
	languageMu.RUnlock()
	newTitle, choice := uifont.New(scaledFontSize(hwnd, 15), 600, chinese)
	newBody, _ := uifont.New(scaledFontSize(hwnd, 13), 400, chinese)
	if newTitle == 0 || newBody == 0 {
		if newTitle != 0 {
			pDeleteObject.Call(uintptr(newTitle))
		}
		if newBody != 0 {
			pDeleteObject.Call(uintptr(newBody))
		}
		return
	}
	oldTitle, oldBody := titleFont, bodyFont
	titleFont, bodyFont, fontChoice = newTitle, newBody, choice
	for _, font := range []windows.Handle{oldTitle, oldBody} {
		if font != 0 {
			pDeleteObject.Call(uintptr(font))
		}
	}
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
	case wmLButtonDown:
		point := point{X: int32(int16(lParam)), Y: int32(int16(lParam >> 16))}
		setCloseState(hwnd, pointInRect(point, closeRect(hwnd)), pointInRect(point, closeRect(hwnd)))
		return 0
	case wmLButtonUp:
		point := point{X: int32(int16(lParam)), Y: int32(int16(lParam >> 16))}
		inside := pointInRect(point, closeRect(hwnd))
		pressed := closeDown
		setCloseState(hwnd, inside, false)
		if pressed && inside {
			dismiss()
		}
		return 0
	case wmMouseMove:
		point := point{X: int32(int16(lParam)), Y: int32(int16(lParam >> 16))}
		inside := pointInRect(point, closeRect(hwnd))
		setCloseState(hwnd, inside, closeDown && inside)
		track := trackMouseEvent{Size: uint32(unsafe.Sizeof(trackMouseEvent{})), Flags: tmeLeave, HwndTrack: hwnd}
		pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&track)))
		return 0
	case wmMouseLeave:
		setCloseState(hwnd, false, false)
		return 0
	case wmDpiChanged:
		rebuildFonts(hwnd)
		position(hwnd)
		mylog.Info("UI font: surface=idle-warning rebuilt reason=dpi-change dpi=%d face=%q", dpiForWindow(hwnd), fontChoice.Face)
		return 0
	case wmDestroy:
		if active == hwnd {
			active = 0
			activeSeq = 0
		}
		return 0
	}
	result, _, _ := pDefWindowProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return result
}

func setCloseState(hwnd windows.Handle, hot, down bool) {
	if closeHot == hot && closeDown == down {
		return
	}
	closeHot = hot
	closeDown = down
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
	palette := uicolors.ForTheme(dark)
	brush := makeBrush(palette.WindowBackground)
	defer pDeleteObject.Call(uintptr(brush))
	pFillRect.Call(dc, uintptr(unsafe.Pointer(&client)), uintptr(brush))
	borderBrush := makeBrush(palette.Border)
	pFrameRect.Call(dc, uintptr(unsafe.Pointer(&client)), uintptr(borderBrush))
	pDeleteObject.Call(uintptr(borderBrush))
	content := warningContentRect(client)
	sc := func(v int32) int32 { return scaleForWindow(hwnd, v) }
	margin := sc(16)
	titleTop, titleBottom := sc(15), sc(42)
	// Use the same compact title accent as the main panel.  A taller bar makes
	// the smaller warning title look visually top-heavy at higher DPI scales.
	accentHeight := sc(16)
	// DrawText places this font's visible glyphs slightly above the centre of
	// its layout rectangle. Compensate for that GDI ascent so the accent aligns
	// with the title the user actually sees, rather than only with its bounds.
	accentTop := titleTop + (titleBottom-titleTop-accentHeight)/2 - sc(3)
	accentRect := rect{Left: content.Left + margin, Top: content.Top + accentTop, Right: content.Left + margin + sc(3), Bottom: content.Top + accentTop + accentHeight}
	accentBrush := makeBrush(palette.Accent)
	pFillRect.Call(dc, uintptr(unsafe.Pointer(&accentRect)), uintptr(accentBrush))
	pDeleteObject.Call(uintptr(accentBrush))
	pSetBkMode.Call(dc, transparent)
	close := closeRect(hwnd)
	closeText := palette.CloseText
	if closeHot {
		fill := palette.CloseHover
		if closeDown {
			fill = palette.ClosePressed
		}
		closeBrush := makeBrush(fill)
		pFillRect.Call(dc, uintptr(unsafe.Pointer(&close)), uintptr(closeBrush))
		pDeleteObject.Call(uintptr(closeBrush))
		closeText = palette.CloseActiveText
	}
	pSetTextColor.Call(dc, uintptr(palette.PrimaryText))
	drawText(dc, title, rect{Left: content.Left + margin + sc(10), Top: content.Top + titleTop, Right: close.Left - sc(6), Bottom: content.Top + titleBottom}, titleFont, dtLeft)
	pSetTextColor.Call(dc, uintptr(palette.SecondaryText))
	drawText(dc, body, rect{Left: content.Left + margin + sc(10), Top: content.Top + sc(47), Right: content.Right - margin, Bottom: content.Bottom - sc(14)}, bodyFont, dtLeft|dtWordBreak)
	drawCloseGlyph(dc, close, closeText)
}

func closeRect(hwnd windows.Handle) rect {
	var client rect
	pGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&client)))
	client = warningContentRect(client)
	size := scaleForWindow(hwnd, 28)
	margin := scaleForWindow(hwnd, 8)
	return closeRectForClient(client, size, margin)
}

func closeRectForClient(client rect, size, margin int32) rect {
	return rect{Left: client.Right - margin - size, Top: client.Top + margin, Right: client.Right - margin, Bottom: client.Top + margin + size}
}

func warningContentRect(client rect) rect {
	if client.Right-client.Left <= 2*warningBorderInset || client.Bottom-client.Top <= 2*warningBorderInset {
		return client
	}
	return rect{Left: client.Left + warningBorderInset, Top: client.Top + warningBorderInset, Right: client.Right - warningBorderInset, Bottom: client.Bottom - warningBorderInset}
}

func pointInRect(point point, bounds rect) bool {
	return point.X >= bounds.Left && point.X < bounds.Right && point.Y >= bounds.Top && point.Y < bounds.Bottom
}

func drawCloseGlyph(dc uintptr, bounds rect, color uint32) {
	// This is a graphic element, not text. Do not rely on the private-use
	// U+E711 glyph from Segoe MDL2 Assets: GDI can silently substitute another
	// font when it is absent, leaving a tofu box on customized Windows images.
	size := bounds.Bottom - bounds.Top
	inset, stroke := closeGlyphMetrics(size)
	pen, _, _ := pCreatePen.Call(0, uintptr(stroke), uintptr(color))
	if pen == 0 {
		return
	}
	defer pDeleteObject.Call(pen)
	old, _, _ := pSelectObject.Call(dc, pen)
	defer pSelectObject.Call(dc, old)
	x1, y1 := bounds.Left+inset, bounds.Top+inset
	x2, y2 := bounds.Right-inset-1, bounds.Bottom-inset-1
	pMoveToEx.Call(dc, uintptr(x1), uintptr(y1), 0)
	pLineTo.Call(dc, uintptr(x2), uintptr(y2))
	pMoveToEx.Call(dc, uintptr(x2), uintptr(y1), 0)
	pLineTo.Call(dc, uintptr(x1), uintptr(y2))
}

func closeGlyphMetrics(size int32) (inset, stroke int32) {
	inset, stroke = size*9/28, size/12
	if stroke < 1 {
		stroke = 1
	}
	return inset, stroke
}

func drawText(dc uintptr, text string, bounds rect, font windows.Handle, format uintptr) {
	ptr, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return
	}
	if font != 0 {
		old, _, _ := pSelectObject.Call(dc, uintptr(font))
		defer pSelectObject.Call(dc, old)
	}
	pDrawText.Call(dc, uintptr(unsafe.Pointer(ptr)), ^uintptr(0), uintptr(unsafe.Pointer(&bounds)), format)
}

func makeBrush(color uint32) windows.Handle {
	result, _, _ := pCreateBrush.Call(uintptr(color))
	return windows.Handle(result)
}
