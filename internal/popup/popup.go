package popup

import (
	"unsafe"
	"golang.org/x/sys/windows"
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

type Action int

const (
	ActNoSleepToggle Action = iota
	ActProcessWatchToggle
	ActIdleToggle
	ActThemeToggle
	ActSunriseToggle
	ActBatteryToggle
	ActFullscreenToggle
	ActHotkeyToggle
	ActAutostartToggle
	ActSwitchTheme
	ActRepairTheme
	ActConfig
	ActAbout
	ActExit
)

type State struct {
	NoSleepEnabled      bool
	ProcessWatchEnabled bool
	IdleEnabled         bool
	ThemeSwitchEnabled  bool
	SunriseMode         bool
	DarkOnBattery       bool
	SkipFullscreen      bool
	HotkeysEnabled      bool
	AutostartEnabled    bool
}

type LangFunc func(key string) string
type OnAction func(action Action)

var (
	onAction OnAction
	lang     LangFunc
	hwndPanel windows.Handle
)

const (
	popupW  = 260
	popupH  = 400
	checkH  = 24
	btnH    = 28
	pad     = 12
	indent  = 16
	wndClass = "IdleTriggerPopup"
)

func Show(s State, onAct OnAction, langFn LangFunc) {
	if hwndPanel != 0 { hide(); return }
	onAction = onAct
	lang = langFn
	createWindow(s)
}

func Hide() { hide() }

func hide() {
	if hwndPanel != 0 {
		user32 := windows.NewLazySystemDLL("user32.dll")
		user32.NewProc("DestroyWindow").Call(uintptr(hwndPanel))
		hwndPanel = 0
	}
}

func createWindow(s State) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	className, _ := windows.UTF16PtrFromString(wndClass)

	var wc wndClassExW
	wc.Size = uint32(unsafe.Sizeof(wc))
	wc.WndProc = windows.NewCallback(wndProc)
	wc.ClassName = className
	user32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc)))

	title, _ := windows.UTF16PtrFromString("IdleTrigger")
	const wsPop = 0x80000000; const wsVis = 0x10000000; const wsCap = 0x00C00000
	const wsExDlg = 0x00040000; const wsExTop = 0x00000008

	hwnd, _, _ := user32.NewProc("CreateWindowExW").Call(
		uintptr(wsExDlg|wsExTop), uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)), uintptr(wsPop|wsVis|wsCap),
		100, 100, popupW, popupH, 0, 0, 0, 0,
	)
	hwndPanel = windows.Handle(hwnd)
	buildControls(s)
	positionWindow()
}

func buildControls(s State) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	btnClass, _ := windows.UTF16PtrFromString("BUTTON")
	staticCls, _ := windows.UTF16PtrFromString("STATIC")

	const wsChild = 0x40000000; const wsVis = 0x10000000
	const bsCheck = 0x00000003; const bsPush = 0x00000000
	const ssLeft = 0x00000000

	label := func(text string, x, y, w int) {
		t, _ := windows.UTF16PtrFromString(text)
		user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(staticCls)), uintptr(unsafe.Pointer(t)),
			uintptr(wsChild|wsVis|ssLeft), uintptr(x), uintptr(y), uintptr(w), uintptr(checkH),
			uintptr(hwndPanel), 0, 0, 0)
	}
	checkbox := func(text string, x, y int, checked bool, id uintptr) {
		t, _ := windows.UTF16PtrFromString(text)
		h, _, _ := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(btnClass)), uintptr(unsafe.Pointer(t)),
			uintptr(wsChild|wsVis|bsCheck), uintptr(x), uintptr(y), uintptr(200), uintptr(checkH),
			uintptr(hwndPanel), id, 0, 0)
		if checked {
			user32.NewProc("SendMessageW").Call(h, 0x00F1, 1, 0)
		}
	}
	button := func(text string, x, y, w int, id uintptr) {
		t, _ := windows.UTF16PtrFromString(text)
		user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(btnClass)), uintptr(unsafe.Pointer(t)),
			uintptr(wsChild|wsVis|bsPush), uintptr(x), uintptr(y), uintptr(w), uintptr(btnH),
			uintptr(hwndPanel), id, 0, 0)
	}

	y := pad
	label(lang("menu_nosleep"), pad, y, 200)
	y += checkH
	checkbox(lang("menu_nosleep_enable"), pad+indent, y, s.NoSleepEnabled, 100)
	y += checkH
	checkbox(lang("menu_process_watch"), pad+indent, y, s.ProcessWatchEnabled, 101)
	y += checkH + pad

	label(lang("menu_idle_enable"), pad, y, 200)
	y += checkH
	checkbox(lang("menu_idle_enable"), pad+indent, y, s.IdleEnabled, 200)
	y += checkH + pad

	label(lang("menu_theme_switch"), pad, y, 200)
	y += checkH
	checkbox(lang("menu_theme_enable"), pad+indent, y, s.ThemeSwitchEnabled, 300)
	y += checkH
	checkbox(lang("menu_theme_sunrise"), pad+indent*2, y, s.SunriseMode, 301)
	y += checkH
	checkbox(lang("menu_theme_battery_dark"), pad+indent*2, y, s.DarkOnBattery, 302)
	y += checkH
	checkbox(lang("menu_theme_skip_fullscreen"), pad+indent*2, y, s.SkipFullscreen, 303)
	y += checkH
	button(lang("menu_theme_switch_now"), pad+indent, y, 90, 310)
	button(lang("menu_theme_repair"), pad+indent+100, y, 90, 311)
	y += btnH + pad

	label("System", pad, y, 200)
	y += checkH
	checkbox(lang("menu_hotkeys"), pad+indent, y, s.HotkeysEnabled, 400)
	y += checkH
	checkbox(lang("menu_autostart"), pad+indent, y, s.AutostartEnabled, 401)
	y += checkH + pad

	button(lang("menu_open_config"), pad, popupH-btnH-pad-8, 70, 500)
	button(lang("menu_about"), pad+80, popupH-btnH-pad-8, 70, 501)
	button(lang("menu_exit"), popupW-pad-60, popupH-btnH-pad-8, 60, 502)
}

func positionWindow() {
	user32 := windows.NewLazySystemDLL("user32.dll")
	var pt struct{ X, Y int32 }
	user32.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&pt)))
	x := int32(pt.X) - popupW/2
	y := int32(pt.Y) - popupH - 20
	if x < 0 { x = 0 }
	if y < 0 { y = int32(pt.Y) + 20 }
	user32.NewProc("SetWindowPos").Call(uintptr(hwndPanel), 0, uintptr(x), uintptr(y), 0, 0, 0x0001)
}

func wndProc(hwnd windows.Handle, msg uint32, wParam, lParam uintptr) uintptr {
	const wmActivate = 0x0006; const wmCommand = 0x0111
	if msg == wmActivate && uint16(wParam) == 0 { hide(); return 0 }
	if msg == wmCommand {
		a := onAction
		switch uint16(wParam) {
		case 100: a(ActNoSleepToggle)
		case 101: a(ActProcessWatchToggle)
		case 200: a(ActIdleToggle)
		case 300: a(ActThemeToggle)
		case 301: a(ActSunriseToggle)
		case 302: a(ActBatteryToggle)
		case 303: a(ActFullscreenToggle)
		case 310: a(ActSwitchTheme)
		case 311: a(ActRepairTheme)
		case 400: a(ActHotkeyToggle)
		case 401: a(ActAutostartToggle)
		case 500: a(ActConfig)
		case 501: a(ActAbout)
		case 502: a(ActExit)
		}
		return 0
	}
	user32 := windows.NewLazySystemDLL("user32.dll")
	ret, _, _ := user32.NewProc("DefWindowProcW").Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return ret
}
