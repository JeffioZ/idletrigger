package nativeform

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// paintStruct is PAINTSTRUCT's maximum size on the supported Windows
// architectures. BeginPaint and EndPaint own its contents.
type paintStruct struct{ data [72]byte }

type clientRect struct{ left, top, right, bottom int32 }

var (
	backgroundUser32        = windows.NewLazySystemDLL("user32.dll")
	backgroundBeginPaint    = backgroundUser32.NewProc("BeginPaint")
	backgroundEndPaint      = backgroundUser32.NewProc("EndPaint")
	backgroundGetClientRect = backgroundUser32.NewProc("GetClientRect")
	backgroundFillRect      = backgroundUser32.NewProc("FillRect")
)

// PaintWindowBackground handles WM_PAINT for form-style windows whose class
// intentionally has no fixed background brush. This keeps theme changes
// dynamic and avoids unpainted black/white areas around child controls.
func PaintWindowBackground(hwnd, brush windows.Handle) {
	if hwnd == 0 || brush == 0 {
		return
	}
	var paint paintStruct
	dc, _, _ := backgroundBeginPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&paint)))
	if dc != 0 {
		var bounds clientRect
		backgroundGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&bounds)))
		backgroundFillRect.Call(dc, uintptr(unsafe.Pointer(&bounds)), uintptr(brush))
	}
	backgroundEndPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&paint)))
}
