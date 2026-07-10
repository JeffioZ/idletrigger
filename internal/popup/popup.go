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
	ActSleep Action = iota
	ActHibernate
	ActShutdown
	ActLock
	ActNoSleepToggle
	ActProcessWatchToggle
	ActIdleToggle
	ActIdleTimeout  Action = 100 + iota
	ActIdleAction
	ActThemeToggle
	ActSunriseToggle
	ActBatteryToggle
	ActFullscreenToggle
	ActSwitchTheme
	ActRepairTheme
	ActHotkeyToggle
	ActAutostartToggle
	ActLanguage
	ActConfig
	ActAbout
	ActExit
)

type State struct {
	NoSleepEnabled, ProcessWatchEnabled, IdleEnabled bool
	IdleTimeout                                      int
	IdleAction                                       string
	ThemeSwitchEnabled, SunriseMode, DarkOnBattery, SkipFullscreen bool
	HotkeysEnabled, AutostartEnabled                                bool
	IsChinese                                                      bool
}

type LangFunc func(key string) string
type OnAction func(action Action, value int)

var (
	onAction  OnAction
	lang      LangFunc
	hwndPanel windows.Handle
)

const popupW, popupH = 300, 540
const checkH, btnH, pad, indent = 22, 28, 10, 16
const wndClass = "IdleTriggerPopup"

func T(k string) string { return lang(k) }

func Show(s State, onAct OnAction, langFn LangFunc) {
	if hwndPanel != 0 {
		hide()
		return
	}
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
	cn, _ := windows.UTF16PtrFromString(wndClass)
	var wc wndClassExW
	wc.Size = uint32(unsafe.Sizeof(wc))
	wc.WndProc = windows.NewCallback(wndProc)
	wc.ClassName = cn
	user32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc)))
	tl, _ := windows.UTF16PtrFromString("IdleTrigger")
	const wp = 0x80000000
	const wv = 0x10000000
	const wc2 = 0x00C00000
	const wxd = 0x00040000
	const wxt = 0x00000008
	hw, _, _ := user32.NewProc("CreateWindowExW").Call(
		uintptr(wxd|wxt), uintptr(unsafe.Pointer(cn)), uintptr(unsafe.Pointer(tl)),
		uintptr(wp|wv|wc2), 100, 100, popupW, popupH, 0, 0, 0, 0)
	hwndPanel = windows.Handle(hw)
	build(s)
	position()
}

func build(s State) {
	u := windows.NewLazySystemDLL("user32.dll")
	bc, _ := windows.UTF16PtrFromString("BUTTON")
	sc, _ := windows.UTF16PtrFromString("STATIC")
	cc, _ := windows.UTF16PtrFromString("COMBOBOX")
	const wc = 0x40000000
	const wv = 0x10000000
	const bch = 0x00000003
	const bpb = 0x00000000
	const sl = 0x00000000
	const cbd = 0x00000003

	lab := func(t string, x, y, w int) {
		tt, _ := windows.UTF16PtrFromString(t)
		u.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(sc)), uintptr(unsafe.Pointer(tt)),
			uintptr(wc|wv|sl), uintptr(x), uintptr(y), uintptr(w), uintptr(checkH), uintptr(hwndPanel), 0, 0, 0)
	}
	chk := func(t string, x, y int, v bool, id uintptr) {
		tt, _ := windows.UTF16PtrFromString(t)
		h, _, _ := u.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(bc)), uintptr(unsafe.Pointer(tt)),
			uintptr(wc|wv|bch), uintptr(x), uintptr(y), uintptr(190), uintptr(checkH), uintptr(hwndPanel), id, 0, 0)
		if v {
			u.NewProc("SendMessageW").Call(h, 0x00F1, 1, 0)
		}
	}
	btn := func(t string, x, y, w int, id uintptr) {
		tt, _ := windows.UTF16PtrFromString(t)
		u.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(bc)), uintptr(unsafe.Pointer(tt)),
			uintptr(wc|wv|bpb), uintptr(x), uintptr(y), uintptr(w), uintptr(btnH), uintptr(hwndPanel), id, 0, 0)
	}
	combo := func(x, y, w int, items []string, sel int, id uintptr) {
		tt, _ := windows.UTF16PtrFromString("")
		h, _, _ := u.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(cc)), uintptr(unsafe.Pointer(tt)),
			uintptr(wc|wv|cbd), uintptr(x), uintptr(y), uintptr(w), uintptr(checkH*8), uintptr(hwndPanel), id, 0, 0)
		for _, it := range items {
			it16, _ := windows.UTF16PtrFromString(it)
			u.NewProc("SendMessageW").Call(h, 0x0143, 0, uintptr(unsafe.Pointer(it16)))
		}
		u.NewProc("SendMessageW").Call(h, 0x014E, uintptr(sel), 0)
	}

	y := pad

	lab(T("menu_sleep"), pad, y, 50)
	btn(T("menu_sleep"), 55, y-2, 55, 1)
	btn(T("menu_hibernate"), 115, y-2, 55, 2)
	btn(T("menu_shutdown"), 175, y-2, 55, 3)
	btn(T("menu_lock"), 235, y-2, 55, 4)
	y += btnH + pad + 4

	lab(T("menu_nosleep"), pad, y, 200)
	y += checkH
	chk(T("menu_nosleep_enable"), pad+indent, y, s.NoSleepEnabled, 10)
	y += checkH
	chk(T("menu_process_watch"), pad+indent, y, s.ProcessWatchEnabled, 11)
	y += checkH + pad

	lab(T("menu_idle_enable"), pad, y, 200)
	y += checkH
	chk(T("menu_idle_enable"), pad+indent, y, s.IdleEnabled, 20)
	y += checkH
	lab(T("menu_idle_timeout"), pad+indent, y, 60)
	timeouts := []string{"5 min", "10 min", "30 min", "60 min", "120 min"}
	combo(pad+indent+65, y-2, 70, timeouts, timeoutIdx(s.IdleTimeout), 21)
	y += checkH
	lab(T("menu_idle_action"), pad+indent, y, 60)
	acts := []string{T("menu_action_sleep"), T("menu_action_hibernate"), T("menu_action_shutdown"), T("menu_action_lock")}
	combo(pad+indent+65, y-2, 80, acts, actionIdx(s.IdleAction), 22)
	y += checkH + pad

	lab(T("menu_theme_switch"), pad, y, 200)
	y += checkH
	chk(T("menu_theme_enable"), pad+indent, y, s.ThemeSwitchEnabled, 30)
	y += checkH
	chk(T("menu_theme_sunrise"), pad+indent*2, y, s.SunriseMode, 31)
	y += checkH
	chk(T("menu_theme_skip_fullscreen"), pad+indent*2, y, s.SkipFullscreen, 33)
	y += checkH
	chk(T("menu_theme_battery_dark"), pad+indent*2, y, s.DarkOnBattery, 32)
	y += checkH
	btn(T("menu_theme_switch_now"), pad+indent, y, 80, 34)
	btn(T("menu_theme_repair"), pad+indent+90, y, 80, 35)
	y += btnH + pad

	chk(T("menu_hotkeys"), pad, y, s.HotkeysEnabled, 40)
	chk(T("menu_autostart"), pad+120, y, s.AutostartEnabled, 41)
	y += checkH + pad
	lab(T("menu_language"), pad, y, 50)
	langs := []string{"English", "\u7B80\u4F53\u4E2D\u6587"}
	li := 0
	if s.IsChinese {
		li = 1
	}
	combo(pad+55, y-2, 80, langs, li, 50)
	y += checkH + pad + 8

	btn(T("menu_open_config"), pad, popupH-btnH-pad-8, 70, 500)
	btn(T("menu_about"), pad+80, popupH-btnH-pad-8, 70, 501)
	btn(T("menu_exit"), popupW-pad-60, popupH-btnH-pad-8, 60, 502)
}

func timeoutIdx(v int) int {
	switch v {
	case 5:
		return 0
	case 10:
		return 1
	case 30:
		return 2
	case 60:
		return 3
	case 120:
		return 4
	}
	return 2
}
func actionIdx(a string) int {
	switch a {
	case "sleep":
		return 0
	case "hibernate":
		return 1
	case "shutdown":
		return 2
	case "lock":
		return 3
	}
	return 0
}

func position() {
	u := windows.NewLazySystemDLL("user32.dll")
	var pt struct{ X, Y int32 }
	u.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&pt)))
	x := int32(pt.X) - popupW/2
	y := int32(pt.Y) - popupH - 20
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = int32(pt.Y) + 20
	}
	u.NewProc("SetWindowPos").Call(uintptr(hwndPanel), 0, uintptr(x), uintptr(y), 0, 0, 0x0001)
}

func wndProc(hwnd windows.Handle, msg uint32, wp, lp uintptr) uintptr {
	const wa = 0x0006
	const wc = 0x0111
	if msg == wa && uint16(wp) == 0 {
		hide()
		return 0
	}
	if msg == wc {
		id := uint16(wp)
		a := onAction
		switch {
		case id == 1:
			a(ActSleep, 0)
		case id == 2:
			a(ActHibernate, 0)
		case id == 3:
			a(ActShutdown, 0)
		case id == 4:
			a(ActLock, 0)
		case id == 10:
			a(ActNoSleepToggle, 0)
		case id == 11:
			a(ActProcessWatchToggle, 0)
		case id == 20:
			a(ActIdleToggle, 0)
		case id == 21:
			a(ActIdleTimeout, getCBSel(lp))
		case id == 22:
			a(ActIdleAction, getCBSel(lp))
		case id == 30:
			a(ActThemeToggle, 0)
		case id == 31:
			a(ActSunriseToggle, 0)
		case id == 32:
			a(ActBatteryToggle, 0)
		case id == 33:
			a(ActFullscreenToggle, 0)
		case id == 34:
			a(ActSwitchTheme, 0)
		case id == 35:
			a(ActRepairTheme, 0)
		case id == 40:
			a(ActHotkeyToggle, 0)
		case id == 41:
			a(ActAutostartToggle, 0)
		case id == 50:
			a(ActLanguage, getCBSel(lp))
		case id == 500:
			a(ActConfig, 0)
		case id == 501:
			a(ActAbout, 0)
		case id == 502:
			a(ActExit, 0)
		}
		return 0
	}
	u := windows.NewLazySystemDLL("user32.dll")
	r, _, _ := u.NewProc("DefWindowProcW").Call(uintptr(hwnd), uintptr(msg), wp, lp)
	return r
}

func getCBSel(lp uintptr) int {
	if uint16(lp>>16) == 1 {
		u := windows.NewLazySystemDLL("user32.dll")
		h := windows.Handle(lp & 0xFFFF)
		r, _, _ := u.NewProc("SendMessageW").Call(uintptr(h), 0x0147, 0, 0)
		return int(r)
	}
	return -1
}
