package ipc

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const pipeName = `\\.\pipe\IdleTrigger`

const (
	pipeBufSize      = 4096
	pipeAccessDuplex = 0x00000003
	pipeTypeMessage  = 0x00000004
	pipeReadModeMsg  = 0x00000002
	pipeUnlimited    = 255
	pipeTimeout      = 1000
)

type Handler func(cmd string) string

// pipeSA builds a SECURITY_ATTRIBUTES that restricts the pipe to
// interactive users, admins, and SYSTEM.
// 构建安全描述符，仅允许交互用户、管理员和 SYSTEM 连接管道。 that restricts the named pipe to
// interactive users, administrators, and SYSTEM only — this prevents
// sandboxed / low-integrity processes from triggering system actions.
func pipeSA() (*windows.SecurityAttributes, error) {
	advapi32 := windows.NewLazySystemDLL("advapi32.dll")
	conv := advapi32.NewProc("ConvertStringSecurityDescriptorToSecurityDescriptorW")

	// SDDL: allow GENERIC_ALL to Interactive Users (IU), System (SY),
	// and Built-in Administrators (BA).  This prevents sandboxed / low-integrity
	// processes from connecting to the pipe.
	// SDDL 格式：允许交互用户、SYSTEM 和管理员完全访问，阻止沙箱/低完整性进程连接管道。
	sddl, _ := syscall.UTF16PtrFromString("D:(A;;GA;;;IU)(A;;GA;;;SY)(A;;GA;;;BA)")

	var sd unsafe.Pointer
	r, _, err := conv.Call(
		uintptr(unsafe.Pointer(sddl)),
		uintptr(1), // SDDL_REVISION_1
		uintptr(unsafe.Pointer(&sd)),
		0,
	)
	if r == 0 {
		return nil, fmt.Errorf("ConvertStringSecurityDescriptorToSecurityDescriptor: %v", err)
	}

	sa := &windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: (*windows.SECURITY_DESCRIPTOR)(sd),
		InheritHandle:      0,
	}
	return sa, nil
}

func Server(handler Handler) error {
	sa, _ := pipeSA() // if it fails, sa is nil (open to all — safe fallback on old Windows)

	for {
		pipePath, err := syscall.UTF16PtrFromString(pipeName)
		if err != nil {
			return fmt.Errorf("pipe path: %w", err)
		}

		openMode := uint32(pipeAccessDuplex | pipeTypeMessage | pipeReadModeMsg)
		pipeMode := uint32(pipeTypeMessage | pipeReadModeMsg)

		h, err := windows.CreateNamedPipe(
			pipePath, openMode, pipeMode,
			pipeUnlimited, pipeBufSize, pipeBufSize,
			pipeTimeout, sa,
		)
		if err != nil {
			return fmt.Errorf("CreateNamedPipe: %w", err)
		}

		err = windows.ConnectNamedPipe(h, nil)
		if err != nil && err != windows.ERROR_PIPE_CONNECTED {
			windows.CloseHandle(h)
			continue
		}

		go handleConn(h, handler)
	}
}

func handleConn(h windows.Handle, handler Handler) {
	defer windows.CloseHandle(h)

	buf := make([]byte, pipeBufSize)
	var done uint32
	err := windows.ReadFile(h, buf, &done, nil)
	if err != nil {
		return
	}

	cmd := strings.TrimSpace(string(buf[:done]))
	resp := handler(cmd)

	out := resp + "\r\n"
	windows.WriteFile(h, []byte(out), nil, nil)

	windows.FlushFileBuffers(h)
	windows.DisconnectNamedPipe(h)
}

func Send(cmd string) (string, bool) {
	pipePath, _ := syscall.UTF16PtrFromString(pipeName)

	const genericReadWrite = 0xC0000000

	h, err := windows.CreateFile(
		pipePath, genericReadWrite, 0, nil,
		windows.OPEN_EXISTING, 0, 0,
	)
	if err != nil {
		return "", false
	}
	defer windows.CloseHandle(h)

	data := []byte(cmd + "\r\n")
	var written uint32
	windows.WriteFile(h, data, &written, nil)

	buf := make([]byte, 4096)
	var done uint32
	windows.ReadFile(h, buf, &done, nil)

	return strings.TrimSpace(string(buf[:done])), true
}

func IsTrayRunning() bool {
	_, ok := Send("ping")
	return ok
}

func Notify(cmd string) { Send(cmd) }

