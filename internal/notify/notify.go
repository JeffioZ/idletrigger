// Package notify — Windows balloon-tip notifications via Shell_NotifyIcon.
// Windows 托盘气泡通知，通过 Shell_NotifyIcon + NIF_INFO 实现。  These show a brief
// message near the tray without stealing focus.
package notify

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Show displays a balloon tip using the tray icon identified by hWnd.
// hWnd should be the HWND of the hidden window created by the systray
// library.  duration is in seconds (10–30 recommended).
func Show(hWnd uintptr, title, body string, duration int) {
	shell32 := windows.NewLazySystemDLL("shell32.dll")
	proc := shell32.NewProc("Shell_NotifyIconW")

	const (
		nimModify = 1
		nifInfo   = 0x00000010
	)

	type guid struct {
		Data1 uint32
		Data2 uint16
		Data3 uint16
		Data4 [8]byte
	}

	type notifyIconData struct {
		Size            uint32
		Wnd             uintptr
		ID              uint32
		Flags           uint32
		CallbackMessage uint32
		Icon            uintptr
		Tip             [128]uint16
		State           uint32
		StateMask       uint32
		Info            [256]uint16
		TimeoutVersion  uint32
		InfoTitle       [64]uint16
		InfoFlags       uint32
		GuidItem        guid
		BalloonIcon     uintptr
	}

	var nid notifyIconData
	nid.Size = uint32(unsafe.Sizeof(nid))
	nid.Wnd = hWnd
	nid.ID = 1 // must match the ID used by systray (typically 1)
	nid.Flags = nifInfo

	copy(nid.Info[:], syscall.StringToUTF16(body))
	copy(nid.InfoTitle[:], syscall.StringToUTF16(title))
	nid.TimeoutVersion = uint32(duration) * 1000

	const niiInfo = 1 // standard info icon
	nid.InfoFlags = niiInfo

	proc.Call(uintptr(nimModify), uintptr(unsafe.Pointer(&nid)))

	_ = unsafe.Sizeof(0)
}
