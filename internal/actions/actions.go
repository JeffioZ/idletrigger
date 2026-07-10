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

// Sleep puts the system into sleep (S3 suspend).  If hibernation is enabled
// by system policy, SetSuspendState may hibernate instead — use Hibernate()
// for an explicit hibernation request.
func Sleep() error {
	// SetSuspendState(FALSE, FALSE, FALSE) → sleep
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
		EWX_SHUTDOWN = 0x00000008
		EWX_POWEROFF = 0x00000010
		EWX_FORCE    = 0x00000004
	)
	flags := uintptr(EWX_SHUTDOWN | EWX_POWEROFF | EWX_FORCE)
	r, _, err := proc.Call(flags, 0)
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
	err = windows.AdjustTokenPrivileges(token, false, &tp, uint32(unsafe.Sizeof(tp)), nil, nil)
	if err != nil {
		return fmt.Errorf("AdjustTokenPrivileges: %w", err)
	}
	return nil
}
