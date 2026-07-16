package nativeform

import (
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// CueBanner lets a native EDIT paint its own hint as temporary display text.
// WM_GETTEXT and WM_GETTEXTLENGTH still report an empty field while the cue is
// active, so search, validation and dirty-state logic never consume the hint.
// The first real character or paste clears it before the edit processes input.
type CueBanner struct {
	edit       windows.Handle
	text       string
	color      uint32
	scale      float64
	displaying bool
	internal   bool
	closed     bool
}

const (
	cueSubclassID   = 0x49544355
	cueWMEnable     = 0x000A
	cueWMSetText    = 0x000C
	cueWMGetTextLen = 0x000E
	cueWMPaint      = 0x000F
	cueWMNCDestroy  = 0x0082
	cueEMReplaceSel = 0x00C2
	cueWMKeyDown    = 0x0100
	cueWMChar       = 0x0102
	cueWMIMEStart   = 0x010D
	cueWMIMEEnd     = 0x010E
	cueWMCut        = 0x0300
	cueWMCopy       = 0x0301
	cueWMPaste      = 0x0302
	cueWMClear      = 0x0303
	cueWMUndo       = 0x0304
	cueBackspace    = 0x08
	cueDelete       = 0x2E
)

var (
	cueUser32               = windows.NewLazySystemDLL("user32.dll")
	cueComctl32             = windows.NewLazySystemDLL("comctl32.dll")
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
	if installed, _, callErr := cueSetWindowSubclass.Call(uintptr(edit), cueCallback, cueSubclassID, 0); installed == 0 {
		return nil, fmt.Errorf("subclass cue banner: %w", callErr)
	}
	cueMu.Lock()
	cueBanners[edit] = banner
	cueMu.Unlock()
	banner.ensureVisible()
	return banner, nil
}

func (c *CueBanner) Close() {
	if c == nil || c.closed {
		return
	}
	if c.displaying {
		c.hide()
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
	c.invalidate()
}

func (c *CueBanner) SetScale(scale float64) {
	if c == nil || c.closed {
		return
	}
	if scale <= 0 {
		scale = 1
	}
	c.scale = scale
}

func (c *CueBanner) invalidate() {
	if c != nil && c.edit != 0 {
		cueInvalidateRect.Call(uintptr(c.edit), 0, 1)
		PresentControl(c.edit, true)
	}
}

func (c *CueBanner) setRawText(value string) {
	if c == nil || c.edit == 0 {
		return
	}
	text, err := windows.UTF16PtrFromString(value)
	if err != nil {
		return
	}
	c.internal = true
	cueSendMessage.Call(uintptr(c.edit), cueWMSetText, 0, uintptr(unsafe.Pointer(text)))
	c.internal = false
}

func (c *CueBanner) ensureVisible() {
	if c == nil || c.closed || c.edit == 0 || c.displaying {
		return
	}
	length, _, _ := cueSendMessage.Call(uintptr(c.edit), cueWMGetTextLen, 0, 0)
	if length != 0 {
		return
	}
	c.displaying = true
	c.setRawText(c.text)
	c.invalidate()
}

func (c *CueBanner) hide() {
	if c == nil || c.closed || c.edit == 0 || !c.displaying {
		return
	}
	c.displaying = false
	c.setRawText("")
	c.invalidate()
}

func (c *CueBanner) finishEdit() {
	if c == nil || c.closed || c.edit == 0 || c.displaying {
		return
	}
	c.ensureVisible()
}

func cueBannerProc(hwnd windows.Handle, message uint32, wParam, lParam uintptr, subclassID, refData uintptr) uintptr {
	cueMu.Lock()
	banner := cueBanners[hwnd]
	cueMu.Unlock()
	if banner == nil {
		result, _, _ := cueDefSubclassProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
		return result
	}
	if banner.displaying && !banner.internal {
		switch message {
		case cueWMCopy, cueWMCut, cueWMClear, cueWMUndo:
			return 0
		case cueWMChar:
			if wParam == cueBackspace {
				return 0
			}
			banner.hide()
		case cueWMPaste, cueEMReplaceSel:
			banner.hide()
		case cueWMIMEStart:
			banner.hide()
		case cueWMKeyDown:
			if wParam == cueBackspace || wParam == cueDelete {
				return 0
			}
		}
	}
	if message == cueWMSetText && !banner.internal {
		banner.displaying = false
		result, _, _ := cueDefSubclassProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
		banner.ensureVisible()
		return result
	}
	result, _, _ := cueDefSubclassProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	switch message {
	case cueWMChar, cueWMCut, cueWMPaste, cueWMClear, cueWMUndo, cueEMReplaceSel, cueWMIMEEnd:
		banner.finishEdit()
	case cueWMKeyDown:
		if wParam == cueBackspace || wParam == cueDelete {
			banner.finishEdit()
		}
	case cueWMEnable, cueWMPaint:
		// The owning WM_CTLCOLOREDIT handler reads displaying and supplies the
		// cue color during the edit's normal native paint.
	case cueWMNCDestroy:
		cueMu.Lock()
		delete(cueBanners, hwnd)
		cueMu.Unlock()
		banner.edit, banner.displaying, banner.closed = 0, false, true
	}
	return result
}
