package nativeform

import "unsafe"

// MessagePointer recovers a pointer carried by a pointer-bearing Win32
// message parameter. Window procedures must declare LPARAM as uintptr because
// most messages carry integers; declaring every LPARAM as unsafe.Pointer makes
// the Go runtime scan values such as mouse coordinates as invalid pointers.
// Callers must use this helper only for messages whose Win32 contract carries
// a pointer valid for the duration of the synchronous callback.
func MessagePointer(value uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&value))
}
