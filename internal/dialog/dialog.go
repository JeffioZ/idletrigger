// Package dialog shows simple message dialogs via MessageBoxW.
// (TaskDialog was tried for dark mode but the OS-level support is too
// version-sensitive to be reliable; kept simple intentionally.)
package dialog

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func Info(title, heading, body string) { show(title, heading+"\n\n"+body, 0x40) }
func Warn(title, heading, body string) { show(title, heading+"\n\n"+body, 0x30) }

func show(title, text string, icon uintptr) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	proc := user32.NewProc("MessageBoxW")
	t, _ := syscall.UTF16PtrFromString(title)
	b, _ := syscall.UTF16PtrFromString(text)
	const mbOK = 0
	proc.Call(0, uintptr(unsafe.Pointer(b)), uintptr(unsafe.Pointer(t)), uintptr(mbOK)|icon)
}
