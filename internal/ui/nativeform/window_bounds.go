package nativeform

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// WindowPlacement describes one top-level form placement in physical pixels.
// ClientWidth and ClientHeight are already DPI-scaled; DPI is used only for
// the non-client frame calculation.
type WindowPlacement struct {
	Window                    windows.Handle
	Anchor, Owner             windows.Handle
	Style, ExStyle            uintptr
	ClientWidth, ClientHeight int
	DPI                       uint32
	Suggested                 *Rect
	WorkArea                  *Rect
}

type windowMonitorInfo struct {
	Size          uint32
	Monitor, Work Rect
	Flags         uint32
}

const (
	windowMonitorNearest = 2
	windowSWPNoZOrder    = 0x0004
	windowSWPNoActivate  = 0x0010
)

var (
	windowUser32            = windows.NewLazySystemDLL("user32.dll")
	windowAdjustRect        = windowUser32.NewProc("AdjustWindowRectEx")
	windowAdjustRectForDPI  = windowUser32.NewProc("AdjustWindowRectExForDpi")
	windowGetClientRect     = windowUser32.NewProc("GetClientRect")
	windowGetWindowRect     = windowUser32.NewProc("GetWindowRect")
	windowGetMonitorInfo    = windowUser32.NewProc("GetMonitorInfoW")
	windowMonitorFromRect   = windowUser32.NewProc("MonitorFromRect")
	windowMonitorFromWindow = windowUser32.NewProc("MonitorFromWindow")
	windowSetWindowPos      = windowUser32.NewProc("SetWindowPos")
)

// PlaceWindow sizes and positions a top-level form inside one visible monitor
// work area. WM_DPICHANGED callers pass Suggested so Windows' recommended
// location is preserved before the result is clamped to the destination work
// area. Initial creation callers omit it and receive owner/work-area centering.
func PlaceWindow(options WindowPlacement) (Rect, error) {
	if options.Window == 0 {
		return Rect{}, fmt.Errorf("top-level window is required")
	}
	if options.DPI == 0 {
		options.DPI = 96
	}
	width, height, err := WindowSizeForClient(options.ClientWidth, options.ClientHeight, options.Style, options.ExStyle, options.DPI)
	if err != nil {
		return Rect{}, err
	}
	work, err := placementWorkArea(options)
	if err != nil {
		return Rect{}, err
	}

	var target Rect
	if options.Suggested != nil && options.Suggested.Right > options.Suggested.Left && options.Suggested.Bottom > options.Suggested.Top {
		target = *options.Suggested
	} else {
		center := work
		if options.Owner != 0 {
			var owner Rect
			if ok, _, _ := windowGetWindowRect.Call(uintptr(options.Owner), uintptr(unsafe.Pointer(&owner))); ok != 0 {
				center = owner
			}
		}
		target = CenteredRect(center, width, height)
	}
	target = ConstrainRect(target, work)
	result, _, callErr := windowSetWindowPos.Call(
		uintptr(options.Window), 0,
		uintptr(target.Left), uintptr(target.Top),
		uintptr(target.Right-target.Left), uintptr(target.Bottom-target.Top),
		windowSWPNoZOrder|windowSWPNoActivate,
	)
	if result == 0 {
		return Rect{}, fmt.Errorf("place top-level window: %w", callErr)
	}
	return target, nil
}

// WindowSizeForClient converts a physical client size to its physical outer
// frame size at a specific DPI. Windows 10's DPI-aware API is preferred, with
// the legacy calculation retained for the supported Server 2016 baseline.
func WindowSizeForClient(clientWidth, clientHeight int, style, exStyle uintptr, dpi uint32) (int32, int32, error) {
	if clientWidth < 1 {
		clientWidth = 1
	}
	if clientHeight < 1 {
		clientHeight = 1
	}
	frame := Rect{Right: int32(clientWidth), Bottom: int32(clientHeight)}
	var ok uintptr
	var callErr error
	if windowAdjustRectForDPI.Find() == nil {
		ok, _, callErr = windowAdjustRectForDPI.Call(uintptr(unsafe.Pointer(&frame)), style, 0, exStyle, uintptr(dpi))
	} else {
		ok, _, callErr = windowAdjustRect.Call(uintptr(unsafe.Pointer(&frame)), style, 0, exStyle)
	}
	if ok == 0 {
		return 0, 0, fmt.Errorf("adjust top-level window frame: %w", callErr)
	}
	return frame.Right - frame.Left, frame.Bottom - frame.Top, nil
}

func placementWorkArea(options WindowPlacement) (Rect, error) {
	if options.WorkArea != nil && options.WorkArea.Right > options.WorkArea.Left && options.WorkArea.Bottom > options.WorkArea.Top {
		return *options.WorkArea, nil
	}
	var monitor uintptr
	if options.Suggested != nil {
		monitor, _, _ = windowMonitorFromRect.Call(uintptr(unsafe.Pointer(options.Suggested)), windowMonitorNearest)
	} else {
		anchor := options.Anchor
		if anchor == 0 {
			anchor = options.Window
		}
		monitor, _, _ = windowMonitorFromWindow.Call(uintptr(anchor), windowMonitorNearest)
	}
	if monitor == 0 {
		return Rect{}, fmt.Errorf("resolve window monitor")
	}
	info := windowMonitorInfo{Size: uint32(unsafe.Sizeof(windowMonitorInfo{}))}
	if ok, _, callErr := windowGetMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info))); ok == 0 {
		return Rect{}, fmt.Errorf("read monitor work area: %w", callErr)
	}
	return info.Work, nil
}

func CenteredRect(area Rect, width, height int32) Rect {
	return Rect{
		Left:   area.Left + (area.Right-area.Left-width)/2,
		Top:    area.Top + (area.Bottom-area.Top-height)/2,
		Right:  area.Left + (area.Right-area.Left-width)/2 + width,
		Bottom: area.Top + (area.Bottom-area.Top-height)/2 + height,
	}
}

// ConstrainRect keeps a top-level rectangle wholly inside the work area. If
// the requested window is larger, its size is reduced before its origin is
// clamped so no edge can remain unreachable.
func ConstrainRect(window, work Rect) Rect {
	workWidth, workHeight := work.Right-work.Left, work.Bottom-work.Top
	if workWidth <= 0 || workHeight <= 0 {
		return window
	}
	width, height := window.Right-window.Left, window.Bottom-window.Top
	width = max(int32(1), min(width, workWidth))
	height = max(int32(1), min(height, workHeight))
	left := max(work.Left, min(window.Left, work.Right-width))
	top := max(work.Top, min(window.Top, work.Bottom-height))
	return Rect{Left: left, Top: top, Right: left + width, Bottom: top + height}
}

// ClientSize returns the current top-level client size in physical pixels.
func ClientSize(hwnd windows.Handle) (int, int, error) {
	var client Rect
	if ok, _, callErr := windowGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&client))); ok == 0 {
		return 0, 0, fmt.Errorf("read top-level client bounds: %w", callErr)
	}
	return int(client.Right - client.Left), int(client.Bottom - client.Top), nil
}
