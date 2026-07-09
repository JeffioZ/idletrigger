package hotkey

import (
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/JeffioZ/idletrigger/internal/log"
)

type Action int

const (
	ActionSleep Action = iota
	ActionLock
	ActionToggleNoSleep
)

type Binding struct {
	VK       uint32
	Modifier uint32
	Action   Action
	Label    string // "Win+Shift+S (Sleep)"
}

type Manager struct {
	bindings  []Binding
	hwnd      windows.Handle
	mu        sync.Mutex
	stopCh    chan struct{}
	callbacks Callbacks
}

type Callbacks struct {
	OnSleep         func()
	OnLock          func()
	OnToggleNoSleep func()
}

type Failed []string

const (
	modAlt   = 0x0001
	modCtrl  = 0x0002
	modShift = 0x0004
	modWin   = 0x0008
	wmHotkey = 0x0312
)

type wndClassExW struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     windows.Handle
}

type msg struct {
	Hwnd    windows.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

func DefaultBindings() []Binding {
	return []Binding{
		{VK: 'S', Modifier: modWin | modShift, Action: ActionSleep, Label: "Win+Shift+S (Sleep)"},
		{VK: 'L', Modifier: modWin | modShift, Action: ActionLock, Label: "Win+Shift+L (Lock)"},
		{VK: 'N', Modifier: modWin | modShift, Action: ActionToggleNoSleep, Label: "Win+Shift+N (Toggle NoSleep)"},
	}
}

func NewManager(bindings []Binding, cbs Callbacks) *Manager {
	return &Manager{
		bindings:  bindings,
		callbacks: cbs,
		stopCh:    make(chan struct{}),
	}
}

// Register creates a hidden message-only window and registers each hotkey.
// Returns Labels of bindings that failed (already in use by another app).
// 创建隐藏的消息窗口并注册每个热键，返回注册失败的绑定标签（被其他应用占用）。
func (m *Manager) Register() Failed {
	user32 := windows.NewLazySystemDLL("user32.dll")

	className, _ := syscall.UTF16PtrFromString("IdleTriggerHotkey")
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	instance := kernel32.NewProc("GetModuleHandleW")
	hInst, _, _ := instance.Call(0)

	var wc wndClassExW
	wc.Size = uint32(unsafe.Sizeof(wc))
	wc.WndProc = windows.NewCallback(m.wndProc)
	wc.Instance = windows.Handle(hInst)
	wc.ClassName = className

	reg := user32.NewProc("RegisterClassExW")
	reg.Call(uintptr(unsafe.Pointer(&wc)))

	create := user32.NewProc("CreateWindowExW")
	const wsOverlapped = 0
	// HWND_MESSAGE (−1): creates a message-only window (invisible, no Z-order).
	// HWND_MESSAGE（−1）：创建仅消息窗口，不可见，无 Z 序。
	const hwndMessage = ^uintptr(0)
	ret, _, _ := create.Call(
		0, uintptr(unsafe.Pointer(className)), 0,
		uintptr(wsOverlapped), 0, 0, 0, 0,
		hwndMessage, 0, hInst, 0,
	)
	m.hwnd = windows.Handle(ret)
	if m.hwnd == 0 {
		var failed Failed
		for _, b := range m.bindings {
			failed = append(failed, b.Label)
		}
		return failed
	}

	var failed Failed
	for i, b := range m.bindings {
		regHK := user32.NewProc("RegisterHotKey")
		r, _, _ := regHK.Call(uintptr(m.hwnd), uintptr(i), uintptr(b.Modifier), uintptr(b.VK))
		if r == 0 {
			failed = append(failed, b.Label)
		}
	}
	if len(failed) > 0 {
		log.Info("Hotkey registration: %d succeeded, %d failed: %v", len(m.bindings)-len(failed), len(failed), failed)
	} else {
		log.Info("Hotkey registration: all %d succeeded", len(m.bindings))
	}
	return failed
}

// Run enters the message pump; blocks until Stop().
func (m *Manager) Run() {
	user32 := windows.NewLazySystemDLL("user32.dll")
	msg2 := &msg{}
	getMsg := user32.NewProc("GetMessageW")
	for {
		select {
		case <-m.stopCh:
			return
		default:
		}
		r, _, _ := getMsg.Call(uintptr(unsafe.Pointer(msg2)), 0, 0, 0)
		if r == 0 || r == ^uintptr(0) {
			break
		}
		dispatch := user32.NewProc("DispatchMessageW")
		dispatch.Call(uintptr(unsafe.Pointer(msg2)))
	}
}

// Start is a convenience: Register, then Run in a new goroutine.
// Returns the Failed list from Register so the caller can notify the user.
func (m *Manager) Start() Failed {
	failed := m.Register()
	go m.Run()
	return failed
}

func (m *Manager) Stop() {
	user32 := windows.NewLazySystemDLL("user32.dll")
	post := user32.NewProc("PostQuitMessage")
	post.Call(0)
	close(m.stopCh)
}

func (m *Manager) wndProc(hwnd windows.Handle, msg2 uint32, wParam, lParam uintptr) uintptr {
	if msg2 == wmHotkey {
		id := int(wParam)
		if id >= 0 && id < len(m.bindings) {
			switch m.bindings[id].Action {
			case ActionSleep:
				if m.callbacks.OnSleep != nil {
					m.callbacks.OnSleep()
				}
			case ActionLock:
				if m.callbacks.OnLock != nil {
					m.callbacks.OnLock()
				}
			case ActionToggleNoSleep:
				if m.callbacks.OnToggleNoSleep != nil {
					m.callbacks.OnToggleNoSleep()
				}
			}
		}
		return 0
	}
	user32 := windows.NewLazySystemDLL("user32.dll")
	defProc := user32.NewProc("DefWindowProcW")
	r, _, _ := defProc.Call(uintptr(hwnd), uintptr(msg2), wParam, lParam)
	return r
}

