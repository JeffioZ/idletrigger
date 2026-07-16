package nativeform

import "golang.org/x/sys/windows"

const sourceCopy = 0x00CC0020

var (
	bufferGDI32            = windows.NewLazySystemDLL("gdi32.dll")
	bufferCreateCompatible = bufferGDI32.NewProc("CreateCompatibleDC")
	bufferCreateBitmap     = bufferGDI32.NewProc("CreateCompatibleBitmap")
	bufferSelectObject     = bufferGDI32.NewProc("SelectObject")
	bufferDeleteObject     = bufferGDI32.NewProc("DeleteObject")
	bufferDeleteDC         = bufferGDI32.NewProc("DeleteDC")
	bufferBitBlt           = bufferGDI32.NewProc("BitBlt")
)

// DrawBuffered paints one owner-drawn region off-screen and commits it with a
// single BitBlt. This prevents the background-clear and rounded-card passes
// from becoming visible as separate hover frames.
func DrawBuffered(target windows.Handle, bounds Rect, paint func(windows.Handle, Rect)) bool {
	width, height := bounds.Right-bounds.Left, bounds.Bottom-bounds.Top
	if target == 0 || width <= 0 || height <= 0 || paint == nil {
		return false
	}
	memoryDC, _, _ := bufferCreateCompatible.Call(uintptr(target))
	if memoryDC == 0 {
		return false
	}
	defer bufferDeleteDC.Call(memoryDC)
	bitmap, _, _ := bufferCreateBitmap.Call(uintptr(target), uintptr(width), uintptr(height))
	if bitmap == 0 {
		return false
	}
	defer bufferDeleteObject.Call(bitmap)
	previous, _, _ := bufferSelectObject.Call(memoryDC, bitmap)
	if previous == 0 {
		return false
	}
	defer bufferSelectObject.Call(memoryDC, previous)
	local := Rect{Right: width, Bottom: height}
	paint(windows.Handle(memoryDC), local)
	result, _, _ := bufferBitBlt.Call(
		uintptr(target), uintptr(bounds.Left), uintptr(bounds.Top), uintptr(width), uintptr(height),
		memoryDC, 0, 0, sourceCopy,
	)
	return result != 0
}
