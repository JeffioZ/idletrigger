package nativeform

import "golang.org/x/sys/windows"

const (
	presentRDWInvalidate  = 0x0001
	presentRDWErase       = 0x0004
	presentRDWAllChildren = 0x0080
	presentRDWUpdateNow   = 0x0100
	presentRDWEraseNow    = 0x0200
	presentRDWFrame       = 0x0400
)

var (
	presenterUser32         = windows.NewLazySystemDLL("user32.dll")
	presenterInvalidateRect = presenterUser32.NewProc("InvalidateRect")
	presenterRedrawWindow   = presenterUser32.NewProc("RedrawWindow")
)

// PresentFrame commits one complete form frame after controls have been
// created, laid out, shown or populated. It deliberately does not use
// WM_SETREDRAW: DefWindowProc changes WS_VISIBLE for that message, which can
// expose an uninitialized top-level frame when a form switches views.
func PresentFrame(window windows.Handle, controls ...windows.Handle) {
	if window == 0 {
		return
	}
	presenterInvalidateRect.Call(uintptr(window), 0, 0)
	for _, control := range controls {
		if control != 0 {
			presenterInvalidateRect.Call(uintptr(control), 0, 1)
		}
	}
	presenterRedrawWindow.Call(uintptr(window), 0, 0, presentRDWInvalidate|presentRDWAllChildren|presentRDWUpdateNow|presentRDWFrame)
}

// PresentControl redraws a populated child immediately. List and preview
// controls use this after model updates so their first visible frame never
// depends on a later mouse or focus message.
func PresentControl(control windows.Handle, erase bool) {
	if control == 0 {
		return
	}
	// ListView keeps its Header as a child HWND. Commit the complete subtree so
	// an asynchronous model update cannot leave the header or untouched rows
	// blank until a later mouse-generated invalidation.
	flags := uintptr(presentRDWInvalidate | presentRDWAllChildren | presentRDWUpdateNow | presentRDWFrame)
	if erase {
		flags |= presentRDWErase | presentRDWEraseNow
	}
	presenterRedrawWindow.Call(uintptr(control), 0, 0, flags)
}
