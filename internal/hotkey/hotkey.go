package hotkey

import (
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
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

// Register creates the message-only window and calls RegisterHotKey for each
// binding.  Returns labels for any that failed (conflict with another app).
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
	return failed
}

// Run enters the message pump; blocks until Stop().
func (m *Manager) Run() {
	user32 := windows.NewLazySystemDLL("user32.dll")
	msg2 := &msg{}
	getMsg := user32.NewProc("GetMessageW")
	for {
		// GetMessage blocks until a message arrives. To stop, we send
		// WM_QUIT via PostMessage from Stop(), which makes GetMessage
		// return 0 and exit the loop naturally.
		// GetMessage 阻塞等待消息。Stop() 通过 PostMessage(hwnd, WM_QUIT)
		// 发送退出消息，GetMessage 返回 0 后自然退出循环。
		r, _, _ := getMsg.Call(uintptr(unsafe.Pointer(msg2)), 0, 0, 0)
		if r == 0 || r == ^uintptr(0) {
			return
		}
		dispatch := user32.NewProc("DispatchMessageW")
		dispatch.Call(uintptr(unsafe.Pointer(msg2)))
	}
}

// Start launches the hotkey goroutine, which locks the OS thread and does
// Register + message loop + cleanup all on the same thread. Returns the
// Failed list from registration.
// 在锁定 OS 线程的 goroutine 中完成 注册→消息循环→清理 全流程，返回注册失败列表。
func (m *Manager) Start() Failed {
	resultCh := make(chan Failed, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		failed := m.Register()
		resultCh <- failed

		// Run message pump on this same thread, then clean up.
		// 在同一线程运行消息循环，然后清理。
		if m.hwnd != 0 {
			m.Run()

			// Unregister hotkeys and destroy window on this thread.
			// 在本线程注销热键并销毁窗口。
			user32 := windows.NewLazySystemDLL("user32.dll")
			for i := range m.bindings {
				unreg := user32.NewProc("UnregisterHotKey")
				unreg.Call(uintptr(m.hwnd), uintptr(i))
			}
			destroy := user32.NewProc("DestroyWindow")
			destroy.Call(uintptr(m.hwnd))
			m.hwnd = 0
		}
	}()
	return <-resultCh
}

// Stop signals the hotkey goroutine to exit. Cleanup is handled inside
// Start()'s goroutine on the same OS thread.
// 发送退出信号，清理由 Start() 的 goroutine 在同一线程处理。
func (m *Manager) Stop() {
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

var _ = unsafe.Sizeof(0)
