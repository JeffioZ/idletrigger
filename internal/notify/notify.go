// Package notify displays balloon notifications through the existing systray
// icon. The current systray dependency identifies its Windows icon by HWND+ID.
package notify

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const systrayIconID = 100

// Show displays a balloon tip through this process's systray window.
func Show(title, body string, duration int) error {
	hwnd, err := findSystrayWindow()
	if err != nil {
		return err
	}

	shell32 := windows.NewLazySystemDLL("shell32.dll")
	proc := shell32.NewProc("Shell_NotifyIconW")

	type guid struct {
		Data1 uint32
		Data2 uint16
		Data3 uint16
		Data4 [8]byte
	}
	type notifyIconData struct {
		Size            uint32
		Wnd             windows.Handle
		ID              uint32
		Flags           uint32
		CallbackMessage uint32
		Icon            windows.Handle
		Tip             [128]uint16
		State           uint32
		StateMask       uint32
		Info            [256]uint16
		TimeoutVersion  uint32
		InfoTitle       [64]uint16
		InfoFlags       uint32
		GuidItem        guid
		BalloonIcon     windows.Handle
	}

	var nid notifyIconData
	nid.Size = uint32(unsafe.Sizeof(nid))
	nid.Wnd = hwnd
	nid.ID = systrayIconID
	nid.Flags = 0x00000010 // NIF_INFO
	copy(nid.Info[:], syscall.StringToUTF16(body))
	copy(nid.InfoTitle[:], syscall.StringToUTF16(title))
	nid.TimeoutVersion = uint32(duration) * 1000
	nid.InfoFlags = 1 // NIIF_INFO

	r, _, callErr := proc.Call(1, uintptr(unsafe.Pointer(&nid))) // NIM_MODIFY
	if r == 0 {
		return fmt.Errorf("Shell_NotifyIconW: %v", callErr)
	}
	return nil
}

func findSystrayWindow() (windows.Handle, error) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	findWindowEx := user32.NewProc("FindWindowExW")
	getWindowPID := user32.NewProc("GetWindowThreadProcessId")
	className, _ := syscall.UTF16PtrFromString("SystrayClass")
	wantPID := uint32(os.Getpid())

	var previous windows.Handle
	for {
		hwnd, _, _ := findWindowEx.Call(0, uintptr(previous), uintptr(unsafe.Pointer(className)), 0)
		if hwnd == 0 {
			return 0, fmt.Errorf("systray window not found")
		}
		var pid uint32
		getWindowPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if pid == wantPID {
			return windows.Handle(hwnd), nil
		}
		previous = windows.Handle(hwnd)
	}
}
