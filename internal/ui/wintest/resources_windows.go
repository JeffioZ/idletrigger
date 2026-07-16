//go:build windows

// Package wintest contains Win32 integration-test helpers. It is imported
// only by _test.go files and is therefore absent from normal release builds.
package wintest

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

type Resources struct {
	GDI, USER uint32
	Handles   uint32
	Threads   int
}

var (
	resourceUser32            = windows.NewLazySystemDLL("user32.dll")
	resourceKernel32          = windows.NewLazySystemDLL("kernel32.dll")
	resourceGetGUIResources   = resourceUser32.NewProc("GetGuiResources")
	resourceGetProcessHandles = resourceKernel32.NewProc("GetProcessHandleCount")
)

func SnapshotResources() (Resources, error) {
	process := windows.CurrentProcess()
	gdi, _, gdiErr := resourceGetGUIResources.Call(uintptr(process), 0)
	if gdi == 0 && gdiErr != windows.ERROR_SUCCESS {
		return Resources{}, fmt.Errorf("read GDI resources: %w", gdiErr)
	}
	user, _, userErr := resourceGetGUIResources.Call(uintptr(process), 1)
	if user == 0 && userErr != windows.ERROR_SUCCESS {
		return Resources{}, fmt.Errorf("read USER resources: %w", userErr)
	}
	var handles uint32
	if ok, _, callErr := resourceGetProcessHandles.Call(uintptr(process), uintptr(unsafe.Pointer(&handles))); ok == 0 {
		return Resources{}, fmt.Errorf("read process handle count: %w", callErr)
	}
	threads, err := currentProcessThreads()
	if err != nil {
		return Resources{}, err
	}
	return Resources{GDI: uint32(gdi), USER: uint32(user), Handles: handles, Threads: threads}, nil
}

func currentProcessThreads() (int, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return 0, fmt.Errorf("snapshot process threads: %w", err)
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	if err := windows.Thread32First(snapshot, &entry); err != nil {
		return 0, fmt.Errorf("read first process thread: %w", err)
	}
	processID := uint32(windows.GetCurrentProcessId())
	count := 0
	for {
		if entry.OwnerProcessID == processID {
			count++
		}
		err := windows.Thread32Next(snapshot, &entry)
		if err == windows.ERROR_NO_MORE_FILES {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("read process thread: %w", err)
		}
	}
	return count, nil
}
