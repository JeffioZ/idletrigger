// Package dialog shows simple message dialogs via MessageBoxW.
// (TaskDialog was tried for dark mode but the OS-level support is too
// version-sensitive to be reliable; kept simple intentionally.)
package dialog

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func Info(title, heading, body string) { show(title, compose(heading, body), 0x40) }
func Warn(title, heading, body string) { show(title, compose(heading, body), 0x30) }

func compose(heading, body string) string {
	if heading == "" {
		return body
	}
	if body == "" {
		return heading
	}
	return heading + "\r\n\r\n" + body
}

func show(title, text string, icon uintptr) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	proc := user32.NewProc("MessageBoxW")
	t, _ := syscall.UTF16PtrFromString(title)
	b, _ := syscall.UTF16PtrFromString(text)
	const mbOK = 0
	proc.Call(0, uintptr(unsafe.Pointer(b)), uintptr(unsafe.Pointer(t)), uintptr(mbOK)|icon)
}
