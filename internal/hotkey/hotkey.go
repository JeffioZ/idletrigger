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
	Label    string // "Win+Shift+S"
}

type Manager struct {
	bindings  []Binding
	hwnd      windows.Handle
	mu        sync.Mutex
	threadID  uint32
	doneCh    chan struct{}
	started   bool
	callbacks Callbacks
}

type Callbacks struct {
	OnSleep         func()
	OnLock          func()
	OnToggleNoSleep func()
}

type Failed []string

const (
	modAlt    = 0x0001
	modCtrl   = 0x0002
	modShift  = 0x0004
	modWin    = 0x0008
	wmHotkey  = 0x0312
	className = "IdleTriggerHotkey"
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
		{VK: 'S', Modifier: modWin | modShift, Action: ActionSleep, Label: "Win+Shift+S"},
		{VK: 'L', Modifier: modWin | modShift, Action: ActionLock, Label: "Win+Shift+L"},
		{VK: 'N', Modifier: modWin | modShift, Action: ActionToggleNoSleep, Label: "Win+Shift+N"},
	}
}

func NewManager(bindings []Binding, cbs Callbacks) *Manager {
	return &Manager{
		bindings:  bindings,
		callbacks: cbs,
	}
}

// Register creates the message-only window and calls RegisterHotKey for each
// binding.  Returns labels for any that failed (conflict with another app).
func (m *Manager) Register() Failed {
	user32 := windows.NewLazySystemDLL("user32.dll")

	classNamePtr, _ := syscall.UTF16PtrFromString(className)
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	instance := kernel32.NewProc("GetModuleHandleW")
	hInst, _, _ := instance.Call(0)

	var wc wndClassExW
	wc.Size = uint32(unsafe.Sizeof(wc))
	wc.WndProc = windows.NewCallback(m.wndProc)
	wc.Instance = windows.Handle(hInst)
	wc.ClassName = classNamePtr

	reg := user32.NewProc("RegisterClassExW")
	reg.Call(uintptr(unsafe.Pointer(&wc)))

	create := user32.NewProc("CreateWindowExW")
	const wsOverlapped = 0
	const hwndMessage = ^uintptr(0)
	ret, _, _ := create.Call(
		0, uintptr(unsafe.Pointer(classNamePtr)), 0,
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
		// GetMessage blocks until a message arrives. Stop sends WM_QUIT
		// to this thread via PostThreadMessage, which makes GetMessage
		// return 0 and exit the loop naturally.
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
func (m *Manager) Start() Failed {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return nil
	}
	m.started = true
	m.doneCh = make(chan struct{})
	doneCh := m.doneCh
	m.mu.Unlock()

	resultCh := make(chan Failed, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(doneCh)

		kernel32 := windows.NewLazySystemDLL("kernel32.dll")
		getCurrentThreadID := kernel32.NewProc("GetCurrentThreadId")
		tid, _, _ := getCurrentThreadID.Call()
		m.mu.Lock()
		m.threadID = uint32(tid)
		m.mu.Unlock()

		failed := m.Register()
		resultCh <- failed

		// Run message pump on this same thread, then clean up.
		if m.hwnd != 0 {
			m.Run()

			// Unregister hotkeys and destroy window on this thread.
			user32 := windows.NewLazySystemDLL("user32.dll")
			for i := range m.bindings {
				unreg := user32.NewProc("UnregisterHotKey")
				unreg.Call(uintptr(m.hwnd), uintptr(i))
			}
			destroy := user32.NewProc("DestroyWindow")
			destroy.Call(uintptr(m.hwnd))
			m.hwnd = 0
		}
		classNamePtr, _ := syscall.UTF16PtrFromString(className)
		getModuleHandle := kernel32.NewProc("GetModuleHandleW")
		hInst, _, _ := getModuleHandle.Call(0)
		user32 := windows.NewLazySystemDLL("user32.dll")
		unregisterClass := user32.NewProc("UnregisterClassW")
		unregisterClass.Call(uintptr(unsafe.Pointer(classNamePtr)), hInst)
		m.mu.Lock()
		m.threadID = 0
		m.started = false
		m.mu.Unlock()
	}()
	return <-resultCh
}

// Stop signals the hotkey goroutine to exit. Cleanup is handled inside
// Start()'s goroutine on the same OS thread.
func (m *Manager) Stop() {
	m.mu.Lock()
	tid := m.threadID
	doneCh := m.doneCh
	started := m.started
	m.mu.Unlock()
	if !started || doneCh == nil {
		return
	}
	if tid != 0 {
		user32 := windows.NewLazySystemDLL("user32.dll")
		postThreadMessage := user32.NewProc("PostThreadMessageW")
		const wmQuit = 0x0012
		postThreadMessage.Call(uintptr(tid), uintptr(wmQuit), 0, 0)
	}
	<-doneCh
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
