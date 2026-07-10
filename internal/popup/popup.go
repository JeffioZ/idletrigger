package popup

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

type wndClassExW struct {
	Size, Style       uint32
	WndProc           uintptr
	ClsExtra, WndExtra int32
	Instance          windows.Handle
	Icon, Cursor, Background windows.Handle
	MenuName, ClassName *uint16
	IconSm            windows.Handle
}

type Action int

const (
	ActSleep Action = iota; ActHibernate; ActShutdown; ActLock
	ActNoSleepToggle; ActProcessWatchToggle; ActIdleToggle
	ActIdleTimeout Action = 100 + iota; ActIdleAction
	ActThemeToggle; ActSunriseToggle; ActBatteryToggle; ActFullscreenToggle
	ActSwitchTheme; ActRepairTheme
	ActHotkeyToggle; ActAutostartToggle; ActLanguage; ActConfig; ActAbout; ActExit
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

var (onAction OnAction; lang LangFunc; hwndPanel windows.Handle; sysFont windows.Handle; scale float64)

const baseW, baseH = 360, 640
const pad, indent, rowH, btnH, gap = 12, 18, 24, 32, 8
const wndName = "IdleTriggerPopup"

func sc(v int) int { return int(float64(v)*scale + 0.5) }
func T(k string) string { return lang(k) }

func Show(s State, onAct OnAction, langFn LangFunc) {
	if hwndPanel != 0 { hide(); return }
	onAction = onAct; lang = langFn
	scale = dpiScale()
	makeSysFont()
	createWindow(s)
}
func Hide() { hide() }
func hide() {
	if hwndPanel != 0 {
		u := windows.NewLazySystemDLL("user32.dll")
		u.NewProc("DestroyWindow").Call(uintptr(hwndPanel))
		hwndPanel = 0
	}
}

func dpiScale() float64 {
	u := windows.NewLazySystemDLL("user32.dll")
	dc, _, _ := u.NewProc("GetDC").Call(0)
	if dc == 0 { return 1.0 }
	gdi32 := windows.NewLazySystemDLL("gdi32.dll")
	dpi, _, _ := gdi32.NewProc("GetDeviceCaps").Call(dc, 90)
	u.NewProc("ReleaseDC").Call(0, dc)
	if dpi == 0 { return 1.0 }
	return float64(dpi) / 96.0
}

func makeSysFont() {
	
	type logFont struct {
		Height, Width, Escapement, Orientation, Weight int32
		Italic, Underline, StrikeOut, CharSet byte
		OutPrecision, ClipPrecision, Quality, PitchAndFamily byte
		FaceName [32]uint16
	}
	var lf logFont
	lf.Height = -int32(sc(11))
	lf.Weight = 400
	lf.CharSet = 1 // DEFAULT_CHARSET
	copy(lf.FaceName[:], windows.StringToUTF16("Segoe UI"))
	gdi32 := windows.NewLazySystemDLL("gdi32.dll")
	cf, _, _ := gdi32.NewProc("CreateFontIndirectW").Call(uintptr(unsafe.Pointer(&lf)))
	sysFont = windows.Handle(cf)
}

func setFont(h uintptr) {
	if sysFont != 0 {
		u := windows.NewLazySystemDLL("user32.dll")
		u.NewProc("SendMessageW").Call(h, 0x0030, uintptr(sysFont), 1)
	}
}

func createWindow(s State) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	cn, _ := windows.UTF16PtrFromString(wndName)
	var wc wndClassExW
	wc.Size = uint32(unsafe.Sizeof(wc))
	wc.WndProc = windows.NewCallback(wndProc)
	wc.Background = windows.Handle(2) // COLOR_BTNFACE - matches system theme
	wc.ClassName = cn
	user32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc)))

	tl, _ := windows.UTF16PtrFromString("IdleTrigger")
	const wp=0x80000000;const wv=0x10000000;const cap=0x00C00000
	const xd=0x00040000;const xt=0x00000008
	w, h := sc(baseW), sc(baseH)
	hw, _, _ := user32.NewProc("CreateWindowExW").Call(
		uintptr(xd|xt), uintptr(unsafe.Pointer(cn)), uintptr(unsafe.Pointer(tl)),
		uintptr(wp|wv|cap), 100, 100, uintptr(w), uintptr(h), 0, 0, 0, 0)
	hwndPanel = windows.Handle(hw)
	build(s)
	position()
	setFont(uintptr(hwndPanel))
}

func build(s State) {
	u := windows.NewLazySystemDLL("user32.dll")
	bc, _ := windows.UTF16PtrFromString("BUTTON")
	st, _ := windows.UTF16PtrFromString("STATIC")
	cb, _ := windows.UTF16PtrFromString("COMBOBOX")
	const wc=0x40000000;const wv=0x10000000
	const bch=0x00000003;const bpb=0x00000000;const sl=0x00000000;const cbd=0x00000003

	lab := func(t string, x, y, w int) {
		tt, _ := windows.UTF16PtrFromString(t)
		h, _, _ := u.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(st)), uintptr(unsafe.Pointer(tt)),
			uintptr(wc|wv|sl), uintptr(sc(x)), uintptr(sc(y)), uintptr(sc(w)), uintptr(sc(rowH)),
			uintptr(hwndPanel), 0, 0, 0)
		setFont(h)
	}
	chk := func(t string, x, y int, v bool, id uintptr) {
		tt, _ := windows.UTF16PtrFromString(t)
		h, _, _ := u.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(bc)), uintptr(unsafe.Pointer(tt)),
			uintptr(wc|wv|bch), uintptr(sc(x)), uintptr(sc(y)), uintptr(sc(210)), uintptr(sc(rowH)),
			uintptr(hwndPanel), id, 0, 0)
		if v { u.NewProc("SendMessageW").Call(h, 0x00F1, 1, 0) }
		setFont(h)
	}
	btn := func(t string, x, y, w int, id uintptr) {
		tt, _ := windows.UTF16PtrFromString(t)
		h, _, _ := u.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(bc)), uintptr(unsafe.Pointer(tt)),
			uintptr(wc|wv|bpb), uintptr(sc(x)), uintptr(sc(y)), uintptr(sc(w)), uintptr(sc(btnH)),
			uintptr(hwndPanel), id, 0, 0)
		setFont(h)
	}
	combo := func(x, y, w int, items []string, sel int, id uintptr) {
		tt, _ := windows.UTF16PtrFromString("")
		h, _, _ := u.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(cb)), uintptr(unsafe.Pointer(tt)),
			uintptr(wc|wv|cbd), uintptr(sc(x)), uintptr(sc(y)), uintptr(sc(w)), uintptr(sc(rowH*8)),
			uintptr(hwndPanel), id, 0, 0)
		for _, it := range items {
			it16, _ := windows.UTF16PtrFromString(it)
			u.NewProc("SendMessageW").Call(h, 0x0143, 0, uintptr(unsafe.Pointer(it16)))
		}
		u.NewProc("SendMessageW").Call(h, 0x014E, uintptr(sel), 0)
		setFont(h)
	}

	y := pad

	// Actions row
	btn(T("menu_sleep"), pad, y, 70, 1)
	btn(T("menu_hibernate"), pad+80, y, 70, 2)
	btn(T("menu_shutdown"), pad+160, y, 70, 3)
	btn(T("menu_lock"), pad+240, y, 70, 4)
	y += btnH + gap + pad

	// Stay Awake
	lab(T("menu_nosleep"), pad, y, 200)
	y += rowH
	chk(T("menu_nosleep_enable"), pad+indent, y, s.NoSleepEnabled, 10)
	y += rowH
	chk(T("menu_process_watch"), pad+indent, y, s.ProcessWatchEnabled, 11)
	y += rowH + gap

	// Idle Monitor
	lab(T("menu_idle_enable"), pad, y, 200)
	y += rowH
	chk(T("menu_idle_enable"), pad+indent, y, s.IdleEnabled, 20)
	y += rowH
	lab(T("menu_idle_timeout"), pad+indent, y, 60)
	timeouts := []string{"5 min","10 min","30 min","60 min","120 min"}
	combo(pad+indent+65, y, 75, timeouts, timeoutIdx(s.IdleTimeout), 21)
	y += rowH
	lab(T("menu_idle_action"), pad+indent, y, 60)
	acts := []string{T("menu_action_sleep"),T("menu_action_hibernate"),T("menu_action_shutdown"),T("menu_action_lock")}
	combo(pad+indent+65, y, 85, acts, actionIdx(s.IdleAction), 22)
	y += rowH + gap

	// Day/Night
	lab(T("menu_theme_switch"), pad, y, 200)
	y += rowH
	chk(T("menu_theme_enable"), pad+indent, y, s.ThemeSwitchEnabled, 30)
	y += rowH
	chk(T("menu_theme_sunrise"), pad+indent*2, y, s.SunriseMode, 31)
	y += rowH
	chk(T("menu_theme_skip_fullscreen"), pad+indent*2, y, s.SkipFullscreen, 33)
	y += rowH
	chk(T("menu_theme_battery_dark"), pad+indent*2, y, s.DarkOnBattery, 32)
	y += rowH
	btn(T("menu_theme_switch_now"), pad+indent, y, 90, 34)
	btn(T("menu_theme_repair"), pad+indent+100, y, 90, 35)
	y += btnH + gap + pad

	// System
	chk(T("menu_hotkeys"), pad, y, s.HotkeysEnabled, 40)
	chk(T("menu_autostart"), pad+140, y, s.AutostartEnabled, 41)
	y += rowH + pad
	lab(T("menu_language"), pad, y, 60)
	langs := []string{"English","简体中文"}
	li := 0; if s.IsChinese { li = 1 }
	combo(pad+65, y, 90, langs, li, 50)
	y += rowH + pad

	// Bottom buttons — fixed position at bottom of window
	bottomY := sc(baseH) - sc(btnH) - sc(pad) - 4
	btn(T("menu_open_config"), pad, bottomY, 80, 500)
	btn(T("menu_about"), pad+90, bottomY, 80, 501)
	btn(T("menu_exit"), sc(baseW)-sc(pad)-80, bottomY, 80, 502)
}

func timeoutIdx(v int) int {
	switch v { case 5: return 0; case 10: return 1; case 30: return 2; case 60: return 3; case 120: return 4 }
	return 2
}
func actionIdx(a string) int {
	switch a { case "sleep": return 0; case "hibernate": return 1; case "shutdown": return 2; case "lock": return 3 }
	return 0
}

func position() {
	u := windows.NewLazySystemDLL("user32.dll")
	var pt struct{ X, Y int32 }
	u.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&pt)))
	w, h := sc(baseW), sc(baseH)
	x := int32(pt.X) - int32(w)/2
	y := int32(int64(pt.Y) - int64(h) - int64(sc(20)))
	if x < 0 { x = 0 }
	if y < 0 { y = int32(int64(pt.Y) + int64(sc(20))) }
	u.NewProc("SetWindowPos").Call(uintptr(hwndPanel), 0, uintptr(x), uintptr(y), 0, 0, 0x0001)
}

func wndProc(hwnd windows.Handle, msg uint32, wp, lp uintptr) uintptr {
	const wa=0x0006;const wc=0x0111
	if msg == wa && uint16(wp) == 0 { hide(); return 0 }
	if msg == wc {
		id := uint16(wp); a := onAction
		switch {
		case id==1: a(ActSleep,0); case id==2: a(ActHibernate,0); case id==3: a(ActShutdown,0); case id==4: a(ActLock,0)
		case id==10: a(ActNoSleepToggle,0); case id==11: a(ActProcessWatchToggle,0)
		case id==20: a(ActIdleToggle,0)
		case id==21: a(ActIdleTimeout,getCBSel(lp)); case id==22: a(ActIdleAction,getCBSel(lp))
		case id==30: a(ActThemeToggle,0); case id==31: a(ActSunriseToggle,0)
		case id==32: a(ActBatteryToggle,0); case id==33: a(ActFullscreenToggle,0)
		case id==34: a(ActSwitchTheme,0); case id==35: a(ActRepairTheme,0)
		case id==40: a(ActHotkeyToggle,0); case id==41: a(ActAutostartToggle,0)
		case id==50: a(ActLanguage,getCBSel(lp))
		case id==500: a(ActConfig,0); case id==501: a(ActAbout,0); case id==502: a(ActExit,0)
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
		r, _, _ := u.NewProc("SendMessageW").Call(uintptr(windows.Handle(lp&0xFFFF)), 0x0147, 0, 0)
		return int(r)
	}
	return -1
}
