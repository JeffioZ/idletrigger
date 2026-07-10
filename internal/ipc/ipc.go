package ipc

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const pipeBaseName = `\\.\pipe\IdleTrigger`

const (
	pipeBufSize      = 4096
	pipeAccessDuplex = 0x00000003
	pipeTypeMessage  = 0x00000004
	pipeReadModeMsg  = 0x00000002
	pipeRejectRemote = 0x00000008
	pipeUnlimited    = 255
	pipeTimeout      = 1000
)

type Handler func(cmd string) string

func pipeName() (string, error) {
	var sessionID uint32
	if err := windows.ProcessIdToSessionId(uint32(os.Getpid()), &sessionID); err != nil {
		return "", fmt.Errorf("ProcessIdToSessionId: %w", err)
	}
	return fmt.Sprintf("%s-%d", pipeBaseName, sessionID), nil
}

// pipeSA restricts the pipe to this logon session, administrators, and SYSTEM.
func pipeSA() (*windows.SecurityAttributes, unsafe.Pointer, error) {
	advapi32 := windows.NewLazySystemDLL("advapi32.dll")
	conv := advapi32.NewProc("ConvertStringSecurityDescriptorToSecurityDescriptorW")

	logonSID, err := currentLogonSID()
	if err != nil {
		return nil, nil, err
	}
	sddl, err := syscall.UTF16PtrFromString(
		"D:(A;;GA;;;" + logonSID + ")(A;;GA;;;SY)(A;;GA;;;BA)",
	)
	if err != nil {
		return nil, nil, fmt.Errorf("security descriptor string: %w", err)
	}

	var sd unsafe.Pointer
	r, _, err := conv.Call(
		uintptr(unsafe.Pointer(sddl)),
		uintptr(1), // SDDL_REVISION_1
		uintptr(unsafe.Pointer(&sd)),
		0,
	)
	if r == 0 {
		return nil, nil, fmt.Errorf("ConvertStringSecurityDescriptorToSecurityDescriptor: %v", err)
	}

	sa := &windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: (*windows.SECURITY_DESCRIPTOR)(sd),
		InheritHandle:      0,
	}
	return sa, sd, nil
}

func currentLogonSID() (string, error) {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		return "", fmt.Errorf("OpenProcessToken: %w", err)
	}
	defer token.Close()

	groups, err := token.GetTokenGroups()
	if err != nil {
		return "", fmt.Errorf("GetTokenGroups: %w", err)
	}
	for _, group := range groups.AllGroups() {
		if group.Attributes&windows.SE_GROUP_LOGON_ID == windows.SE_GROUP_LOGON_ID {
			return group.Sid.String(), nil
		}
	}
	return "", fmt.Errorf("logon SID not found")
}

func Server(handler Handler) error {
	sa, sd, err := pipeSA()
	if err != nil {
		return err
	}
	defer windows.LocalFree(windows.Handle(sd))
	name, err := pipeName()
	if err != nil {
		return err
	}
	pipePath, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return fmt.Errorf("pipe path: %w", err)
	}

	for {
		openMode := uint32(pipeAccessDuplex)
		pipeMode := uint32(pipeTypeMessage | pipeReadModeMsg | pipeRejectRemote)

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
	if err := windows.WriteFile(h, []byte(out), nil, nil); err != nil {
		return
	}

	windows.FlushFileBuffers(h)
	windows.DisconnectNamedPipe(h)
}

func Send(cmd string) (string, bool) {
	name, err := pipeName()
	if err != nil {
		return "", false
	}
	pipePath, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return "", false
	}

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
	if err := windows.WriteFile(h, data, &written, nil); err != nil || written != uint32(len(data)) {
		return "", false
	}

	buf := make([]byte, 4096)
	var done uint32
	if err := windows.ReadFile(h, buf, &done, nil); err != nil {
		return "", false
	}

	return strings.TrimSpace(string(buf[:done])), true
}
