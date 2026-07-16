package nativeform

import (
	"golang.org/x/sys/windows"

	"github.com/JeffioZ/idletrigger/internal/platform/windows/resourceid"
)

const (
	wmSetIcon = 0x0080
	iconSmall = 0
	iconBig   = 1
	imageIcon = 1
)

var (
	iconUser32          = windows.NewLazySystemDLL("user32.dll")
	iconKernel32        = windows.NewLazySystemDLL("kernel32.dll")
	iconLoadImage       = iconUser32.NewProc("LoadImageW")
	iconDestroy         = iconUser32.NewProc("DestroyIcon")
	iconSendMessage     = iconUser32.NewProc("SendMessageW")
	iconGetModuleHandle = iconKernel32.NewProc("GetModuleHandleW")
)

// WindowIcons owns the per-window title-bar icons used by form-style windows.
// A dark mark is used on a light caption and a light mark on a dark caption,
// matching the main control panel. Handles loaded by LoadImageW are released
// when replaced or when the window is destroyed.
type WindowIcons struct {
	large, small windows.Handle
	dark         bool
	initialized  bool
}

// Apply refreshes the title-bar icon when the theme or DPI changes.
func (i *WindowIcons) Apply(hwnd windows.Handle, dark bool, largeSize, smallSize int, force bool) {
	if hwnd == 0 || (i.initialized && !force && i.dark == dark) {
		return
	}
	module, _, _ := iconGetModuleHandle.Call(0)
	if module == 0 {
		return
	}
	resource := uintptr(resourceid.TrayDarkIconID)
	if dark {
		resource = uintptr(resourceid.TrayLightIconID)
	}
	large, _, _ := iconLoadImage.Call(module, resource, imageIcon, uintptr(largeSize), uintptr(largeSize), 0)
	small, _, _ := iconLoadImage.Call(module, resource, imageIcon, uintptr(smallSize), uintptr(smallSize), 0)
	if large == 0 || small == 0 {
		if large != 0 {
			iconDestroy.Call(large)
		}
		if small != 0 {
			iconDestroy.Call(small)
		}
		return
	}
	i.release()
	i.large, i.small = windows.Handle(large), windows.Handle(small)
	i.dark, i.initialized = dark, true
	iconSendMessage.Call(uintptr(hwnd), wmSetIcon, iconBig, uintptr(i.large))
	iconSendMessage.Call(uintptr(hwnd), wmSetIcon, iconSmall, uintptr(i.small))
}

// Release frees every owned icon handle.
func (i *WindowIcons) Release() {
	i.release()
	i.initialized = false
}

func (i *WindowIcons) release() {
	for _, icon := range []windows.Handle{i.large, i.small} {
		if icon != 0 {
			iconDestroy.Call(uintptr(icon))
		}
	}
	i.large, i.small = 0, 0
}
