// Package actions implements Windows system power actions — sleep,
// hibernate, shutdown, and lock — via direct Win32 API calls.
package actions

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	powrprof = windows.NewLazySystemDLL("powrprof.dll")
	advapi32 = windows.NewLazySystemDLL("advapi32.dll")
)

// Sleep requests the system suspend state. The exact hardware sleep state is
// selected by Windows and the machine firmware.
func Sleep() error {
	// FALSE requests suspend rather than hibernation.
	proc := powrprof.NewProc("SetSuspendState")
	r, _, err := proc.Call(0, 0, 0) // Hibernate=FALSE, ForceCritical=FALSE, DisableWakeEvent=FALSE
	if r == 0 {
		return fmt.Errorf("SetSuspendState (sleep): %v", err)
	}
	return nil
}

// Hibernate puts the system into hibernation (S4).
func Hibernate() error {
	proc := powrprof.NewProc("SetSuspendState")
	r, _, err := proc.Call(1, 0, 0) // Hibernate=TRUE
	if r == 0 {
		return fmt.Errorf("SetSuspendState (hibernate): %v", err)
	}
	return nil
}

// Shutdown powers off the system after acquiring SE_SHUTDOWN_NAME privilege.
func Shutdown() error {
	if err := acquireShutdownPrivilege(); err != nil {
		return fmt.Errorf("acquire shutdown privilege: %w", err)
	}
	proc := user32.NewProc("ExitWindowsEx")
	const (
		EWX_SHUTDOWN = 0x00000001
		EWX_POWEROFF = 0x00000008
		// Mark the shutdown as planned so Windows records it correctly.
		SHTDN_REASON_FLAG_PLANNED = 0x80000000
	)
	flags := uintptr(EWX_SHUTDOWN | EWX_POWEROFF)
	r, _, err := proc.Call(flags, uintptr(SHTDN_REASON_FLAG_PLANNED))
	if r == 0 {
		return fmt.Errorf("ExitWindowsEx: %v", err)
	}
	return nil
}

// Lock locks the workstation.
func Lock() error {
	proc := user32.NewProc("LockWorkStation")
	r, _, err := proc.Call()
	if r == 0 {
		return fmt.Errorf("LockWorkStation: %v", err)
	}
	return nil
}

// acquireShutdownPrivilege enables SE_SHUTDOWN_NAME for the current process.
func acquireShutdownPrivilege() error {
	var token windows.Token
	err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &token)
	if err != nil {
		return fmt.Errorf("OpenProcessToken: %w", err)
	}
	defer token.Close()

	var luid windows.LUID
	advapi32Proc := advapi32.NewProc("LookupPrivilegeValueW")
	r, _, err := advapi32Proc.Call(0, uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("SeShutdownPrivilege"))), uintptr(unsafe.Pointer(&luid)))
	if r == 0 {
		return fmt.Errorf("LookupPrivilegeValue: %v", err)
	}

	tp := windows.Tokenprivileges{
		PrivilegeCount: 1,
		Privileges: [1]windows.LUIDAndAttributes{
			{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED},
		},
	}
	setLastError := windows.NewLazySystemDLL("kernel32.dll").NewProc("SetLastError")
	setLastError.Call(0)
	adjust := advapi32.NewProc("AdjustTokenPrivileges")
	r, _, callErr := adjust.Call(
		uintptr(token),
		0,
		uintptr(unsafe.Pointer(&tp)),
		uintptr(unsafe.Sizeof(tp)),
		0,
		0,
	)
	if r == 0 {
		return fmt.Errorf("AdjustTokenPrivileges: %v", callErr)
	}
	if callErr == windows.ERROR_NOT_ALL_ASSIGNED {
		return fmt.Errorf("AdjustTokenPrivileges: %w", windows.ERROR_NOT_ALL_ASSIGNED)
	}
	return nil
}
