//go:build windows && devtools

package inputdiag

import (
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	mylog "github.com/JeffioZ/idletrigger/internal/log"
)

const (
	whKeyboardLL = 13
	whMouseLL    = 14
	wmQuit       = 0x0012

	llkhfLowerILInjected = 0x00000002
	llkhfInjected        = 0x00000010
	llkhfUp              = 0x00000080

	llmhfInjected        = 0x00000001
	llmhfLowerILInjected = 0x00000002
)

type point struct {
	X int32
	Y int32
}

type kbdLLHookStruct struct {
	VKCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

type msLLHookStruct struct {
	Pt          point
	MouseData   uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

type msg struct {
	Hwnd    windows.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

var (
	user32              = windows.NewLazySystemDLL("user32.dll")
	kernel32            = windows.NewLazySystemDLL("kernel32.dll")
	pSetWindowsHookEx   = user32.NewProc("SetWindowsHookExW")
	pUnhookWindowsHook  = user32.NewProc("UnhookWindowsHookEx")
	pCallNextHookEx     = user32.NewProc("CallNextHookEx")
	pGetMessage         = user32.NewProc("GetMessageW")
	pPostThreadMessage  = user32.NewProc("PostThreadMessageW")
	pRtlMoveMemory      = kernel32.NewProc("RtlMoveMemory")
	keyboardCallbackPtr = windows.NewCallback(keyboardProc)
	mouseCallbackPtr    = windows.NewCallback(mouseProc)
)

type session struct {
	mu       sync.Mutex
	threadID uint32
	hooks    []windows.Handle
	ready    chan struct{}
	done     chan struct{}
}

var active *session

// Start enables developer-tools input tracing when requested by the central
// startup resolver.
func Start(enabled bool) func() {
	if !enabled {
		return nil
	}
	s := &session{ready: make(chan struct{}), done: make(chan struct{})}
	active = s
	go s.run()
	<-s.ready
	if len(s.hooks) == 0 {
		s.stop()
		return nil
	}
	mylog.Info("Developer tools input trace enabled: recording keyboard key codes, injection flags, and mouse metadata to the debug log")
	return s.stop
}

func (s *session) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(s.done)

	s.threadID = windows.GetCurrentThreadId()
	keyboardHook, _, keyboardErr := pSetWindowsHookEx.Call(whKeyboardLL, keyboardCallbackPtr, 0, 0)
	mouseHook, _, mouseErr := pSetWindowsHookEx.Call(whMouseLL, mouseCallbackPtr, 0, 0)
	if keyboardHook != 0 {
		s.hooks = append(s.hooks, windows.Handle(keyboardHook))
	} else {
		mylog.Info("Input diagnostics keyboard hook failed: %v", keyboardErr)
	}
	if mouseHook != 0 {
		s.hooks = append(s.hooks, windows.Handle(mouseHook))
	} else {
		mylog.Info("Input diagnostics mouse hook failed: %v", mouseErr)
	}
	close(s.ready)

	var m msg
	for {
		ret, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
	}
}

func (s *session) stop() {
	s.mu.Lock()
	hooks := append([]windows.Handle(nil), s.hooks...)
	s.hooks = nil
	threadID := s.threadID
	s.mu.Unlock()

	for _, hook := range hooks {
		pUnhookWindowsHook.Call(uintptr(hook))
	}
	if threadID != 0 {
		pPostThreadMessage.Call(uintptr(threadID), wmQuit, 0, 0)
	}
	<-s.done
	if active == s {
		active = nil
	}
	mylog.Info("Developer tools input trace stopped")
}

func keyboardProc(nCode int, wParam, lParam uintptr) uintptr {
	if nCode >= 0 && lParam != 0 {
		var info kbdLLHookStruct
		copyHookStruct(uintptr(unsafe.Pointer(&info)), lParam, unsafe.Sizeof(info))
		injected := info.Flags&llkhfInjected != 0
		lowerIL := info.Flags&llkhfLowerILInjected != 0
		keyUp := info.Flags&llkhfUp != 0
		mylog.Info("Input diagnostics event: type=keyboard message=0x%X vk_code=%d scan_code=%d injected=%v lower_il_injected=%v key_up=%v event_time=%d",
			wParam, info.VKCode, info.ScanCode, injected, lowerIL, keyUp, info.Time)
	}
	ret, _, _ := pCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return ret
}

func mouseProc(nCode int, wParam, lParam uintptr) uintptr {
	if nCode >= 0 && lParam != 0 {
		var info msLLHookStruct
		copyHookStruct(uintptr(unsafe.Pointer(&info)), lParam, unsafe.Sizeof(info))
		injected := info.Flags&llmhfInjected != 0
		lowerIL := info.Flags&llmhfLowerILInjected != 0
		mylog.Info("Input diagnostics event: type=mouse message=0x%X x=%d y=%d mouse_data=%d injected=%v lower_il_injected=%v event_time=%d",
			wParam, info.Pt.X, info.Pt.Y, info.MouseData, injected, lowerIL, info.Time)
	}
	ret, _, _ := pCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return ret
}

func copyHookStruct(dst, src uintptr, size uintptr) {
	pRtlMoveMemory.Call(dst, src, size)
}
