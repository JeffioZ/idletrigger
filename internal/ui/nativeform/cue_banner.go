package nativeform

import (
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// CueBanner paints a theme-aware hint inside an empty native EDIT control.
// Windows' built-in EM_SETCUEBANNER uses a fixed light-theme color on some
// systems, which makes the hint nearly invisible in dark mode.
type CueBanner struct {
	edit   windows.Handle
	text   string
	color  uint32
	scale  float64
	closed bool
}

type cueClientRect struct{ Left, Top, Right, Bottom int32 }

const (
	cueSubclassID    = 0x49544355
	cueWMSetFocus    = 0x0007
	cueWMKillFocus   = 0x0008
	cueWMEnable      = 0x000A
	cueWMSetText     = 0x000C
	cueWMGetTextLen  = 0x000E
	cueWMPaint       = 0x000F
	cueWMGetFont     = 0x0031
	cueWMNCDestroy   = 0x0082
	cueWMPrint       = 0x0317
	cueWMPrintClient = 0x0318
	cueEMGetMargins  = 0x00D4
)

var (
	cueUser32               = windows.NewLazySystemDLL("user32.dll")
	cueComctl32             = windows.NewLazySystemDLL("comctl32.dll")
	cueGetDC                = cueUser32.NewProc("GetDC")
	cueReleaseDC            = cueUser32.NewProc("ReleaseDC")
	cueGetClientRect        = cueUser32.NewProc("GetClientRect")
	cueGetFocus             = cueUser32.NewProc("GetFocus")
	cueInvalidateRect       = cueUser32.NewProc("InvalidateRect")
	cueSendMessage          = cueUser32.NewProc("SendMessageW")
	cueSetWindowSubclass    = cueComctl32.NewProc("SetWindowSubclass")
	cueRemoveWindowSubclass = cueComctl32.NewProc("RemoveWindowSubclass")
	cueDefSubclassProc      = cueComctl32.NewProc("DefSubclassProc")
	cueCallback             = windows.NewCallback(cueBannerProc)
	cueMu                   sync.Mutex
	cueBanners              = make(map[windows.Handle]*CueBanner)
)

func NewCueBanner(edit windows.Handle, text string, color uint32, scale float64) (*CueBanner, error) {
	if edit == 0 || text == "" {
		return nil, fmt.Errorf("cue banner edit and text are required")
	}
	if scale <= 0 {
		scale = 1
	}
	banner := &CueBanner{edit: edit, text: text, color: color, scale: scale}
	installed, _, callErr := cueSetWindowSubclass.Call(uintptr(edit), cueCallback, cueSubclassID, 0)
	if installed == 0 {
		return nil, fmt.Errorf("subclass cue banner: %w", callErr)
	}
	cueMu.Lock()
	cueBanners[edit] = banner
	cueMu.Unlock()
	cueInvalidateRect.Call(uintptr(edit), 0, 0)
	return banner, nil
}

func (c *CueBanner) Close() {
	if c == nil || c.closed {
		return
	}
	c.closed = true
	if c.edit != 0 {
		cueRemoveWindowSubclass.Call(uintptr(c.edit), cueCallback, cueSubclassID)
		cueMu.Lock()
		delete(cueBanners, c.edit)
		cueMu.Unlock()
		c.edit = 0
	}
}

func (c *CueBanner) SetTheme(color uint32) {
	if c == nil || c.closed {
		return
	}
	c.color = color
	if c.edit != 0 {
		cueInvalidateRect.Call(uintptr(c.edit), 0, 0)
	}
}

func (c *CueBanner) SetScale(scale float64) {
	if c == nil || c.closed {
		return
	}
	if scale <= 0 {
		scale = 1
	}
	c.scale = scale
	if c.edit != 0 {
		cueInvalidateRect.Call(uintptr(c.edit), 0, 0)
	}
}

func (c *CueBanner) draw(dc windows.Handle) {
	if c == nil || c.closed || dc == 0 || c.edit == 0 {
		return
	}
	length, _, _ := cueSendMessage.Call(uintptr(c.edit), cueWMGetTextLen, 0, 0)
	focused, _, _ := cueGetFocus.Call()
	if length != 0 || windows.Handle(focused) == c.edit {
		return
	}
	var client cueClientRect
	if ok, _, _ := cueGetClientRect.Call(uintptr(c.edit), uintptr(unsafe.Pointer(&client))); ok == 0 {
		return
	}
	font, _, _ := cueSendMessage.Call(uintptr(c.edit), cueWMGetFont, 0, 0)
	margins, _, _ := cueSendMessage.Call(uintptr(c.edit), cueEMGetMargins, 0, 0)
	left := int32(uint16(margins))
	if left <= 0 {
		left = scaledPixels(6, c.scale)
	}
	drawLabel(dc, Rect{Left: client.Left, Top: client.Top, Right: client.Right, Bottom: client.Bottom}, windows.Handle(font), c.text, c.color, true, left, left)
}

func cueBannerProc(hwnd windows.Handle, message uint32, wParam, lParam uintptr, subclassID, refData uintptr) uintptr {
	cueMu.Lock()
	banner := cueBanners[hwnd]
	cueMu.Unlock()
	result, _, _ := cueDefSubclassProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	if banner == nil {
		return result
	}
	switch message {
	case cueWMPaint:
		dc, _, _ := cueGetDC.Call(uintptr(hwnd))
		banner.draw(windows.Handle(dc))
		if dc != 0 {
			cueReleaseDC.Call(uintptr(hwnd), dc)
		}
	case cueWMPrint, cueWMPrintClient:
		banner.draw(windows.Handle(wParam))
	case cueWMSetFocus, cueWMKillFocus, cueWMEnable, cueWMSetText:
		cueInvalidateRect.Call(uintptr(hwnd), 0, 0)
	case cueWMNCDestroy:
		cueMu.Lock()
		delete(cueBanners, hwnd)
		cueMu.Unlock()
		banner.edit = 0
		banner.closed = true
	}
	return result
}
