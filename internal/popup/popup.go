// Package popup implements the single-instance tray control panel.
package popup

import (
	"fmt"
	"sync"
	"syscall"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/JeffioZ/idletrigger/internal/themeswitch"
)

type wndClassExW struct {
	Size, Style              uint32
	WndProc                  uintptr
	ClsExtra, WndExtra       int32
	Instance                 windows.Handle
	Icon, Cursor, Background windows.Handle
	MenuName, ClassName      *uint16
	IconSm                   windows.Handle
}

type rect struct{ Left, Top, Right, Bottom int32 }

type monitorInfo struct {
	Size          uint32
	Monitor, Work rect
	Flags         uint32
}

type drawItem struct {
	CtlType, CtlID, ItemID, ItemAction, ItemState uint32
	HwndItem, HDC                                 windows.Handle
	Rect                                          rect
	ItemData                                      uintptr
}

type Action int

const (
	ActSleep Action = iota
	ActHibernate
	ActShutdown
	ActLock
	ActRestart
	ActNoSleepToggle
	ActProcessWatchToggle
	ActIdleToggle
	ActIdleTimeout Action = 100 + iota
	ActIdleAction
	ActThemeToggle
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
	NoSleepEnabled, ProcessWatchEnabled, IdleEnabled  bool
	IdleTimeout                                       int
	IdleAction                                        string
	ThemeSwitchEnabled, DarkOnBattery, SkipFullscreen bool
	HotkeysEnabled, AutostartEnabled                  bool
	IsChinese                                         bool
	ThemeSchedule                                     string
}

type LangFunc func(key string) string
type OnAction func(action Action, value int)

const (
	panelClass = "IdleTriggerPopup"
	baseW      = 456
	pad        = 16
	gap        = 8
	buttonH    = 36
	sectionH   = 22
	subtitleH  = 18

	wmDestroy        = 0x0002
	wmClose          = 0x0010
	wmActivate       = 0x0006
	wmMouseMove      = 0x0200
	wmMouseLeave     = 0x02A3
	wmEraseBkgnd     = 0x0014
	wmSysColorChange = 0x0015
	wmSettingChange  = 0x001A
	wmDrawItem       = 0x002B
	wmCommand        = 0x0111
	wmCtlColorStatic = 0x0138
	wmThemeChanged   = 0x031A
	wmSetFont        = 0x0030
	bnClicked        = 0

	wsPopup        = 0x80000000
	wsCaption      = 0x00C00000
	wsSysMenu      = 0x00080000
	wsClipChildren = 0x02000000
	wsChild        = 0x40000000
	wsVisible      = 0x10000000
	bsOwnerDraw    = 0x0000000B
	ssLeft         = 0x00000000

	wsExToolWindow = 0x00000080
	wsExTopmost    = 0x00000008
	wsExComposited = 0x02000000

	swpShowWindow  = 0x0040
	monitorNearest = 2
	gwlpWndProc    = ^uintptr(3)

	odsSelected = 0x0001
	odsDisabled = 0x0004
	odsHotlight = 0x0040
	dtCenter    = 0x00000001
	dtVCenter   = 0x00000004
	dtWordBreak = 0x00000010
	dtCalcRect  = 0x00000400
	transparent = 1
	waInactive  = 0
	tmeLeave    = 0x00000002
)

const (
	idSleep = 1 + iota
	idHibernate
	idShutdown
	idLock
	idRestart
)

const (
	idNoSleep         = 10
	idProcess         = 11
	idIdle            = 20
	idTheme           = 30
	idBattery         = 32
	idFullscreen      = 33
	idThemeSwitch     = 34
	idThemeRepair     = 35
	idHotkeys         = 40
	idAutostart       = 41
	idTime5           = 121
	idTime10          = 122
	idTime30          = 123
	idTime60          = 124
	idTime120         = 125
	idActionSleep     = 131
	idActionHibernate = 132
	idActionShutdown  = 133
	idActionLock      = 134
	idLangEN          = 151
	idLangZH          = 152
	idConfig          = 500
	idAbout           = 501
	idExit            = 502
)

var (
	user32 = windows.NewLazySystemDLL("user32.dll")
	gdi32  = windows.NewLazySystemDLL("gdi32.dll")

	pCreateWindowEx    = user32.NewProc("CreateWindowExW")
	pRegisterClassEx   = user32.NewProc("RegisterClassExW")
	pDestroyWindow     = user32.NewProc("DestroyWindow")
	pDefWindowProc     = user32.NewProc("DefWindowProcW")
	pCallWindowProc    = user32.NewProc("CallWindowProcW")
	pSendMessage       = user32.NewProc("SendMessageW")
	pSetWindowLong     = user32.NewProc("SetWindowLongW")
	pSetWindowLongPtr  = user32.NewProc("SetWindowLongPtrW")
	pSetWindowPos      = user32.NewProc("SetWindowPos")
	pGetCursorPos      = user32.NewProc("GetCursorPos")
	pMonitorFromWindow = user32.NewProc("MonitorFromWindow")
	pGetMonitorInfo    = user32.NewProc("GetMonitorInfoW")
	pAdjustWindowRect  = user32.NewProc("AdjustWindowRectEx")
	pGetDpiForWindow   = user32.NewProc("GetDpiForWindow")
	pGetDpiForSystem   = user32.NewProc("GetDpiForSystem")
	pSetForeground     = user32.NewProc("SetForegroundWindow")
	pFillRect          = user32.NewProc("FillRect")
	pFrameRect         = user32.NewProc("FrameRect")
	pDrawText          = user32.NewProc("DrawTextW")
	pInvalidateRect    = user32.NewProc("InvalidateRect")
	pGetClientRect     = user32.NewProc("GetClientRect")
	pTrackMouseEvent   = user32.NewProc("TrackMouseEvent")
	pDeleteObject      = gdi32.NewProc("DeleteObject")
	pCreateFont        = gdi32.NewProc("CreateFontIndirectW")
	pCreateBrush       = gdi32.NewProc("CreateSolidBrush")
	pSetTextColor      = gdi32.NewProc("SetTextColor")
	pSetBkMode         = gdi32.NewProc("SetBkMode")
	pSelectObject      = gdi32.NewProc("SelectObject")

	classOnce sync.Once
	classErr  error
	panelMu   sync.Mutex
	active    *panel

	buttonProc = windows.NewCallback(buttonWndProc)
)

type colors struct {
	background, surface, hover, border, accent, pressed, text, mutedText, accentText, disabled uint32
	danger, dangerHover, dangerPressed, dangerText                                             uint32
}

type trackMouseEvent struct {
	Size      uint32
	Flags     uint32
	HwndTrack windows.Handle
	HoverTime uint32
}

type panel struct {
	hwnd                            windows.Handle
	font, sectionFont, subtitleFont windows.Handle
	backgroundBrush                 windows.Handle
	surfaceBrush                    windows.Handle
	hoverBrush                      windows.Handle
	borderBrush                     windows.Handle
	accentBrush                     windows.Handle
	pressedBrush                    windows.Handle
	disabledBrush                   windows.Handle
	dangerBrush                     windows.Handle
	dangerHoverBrush                windows.Handle
	dangerPressedBrush              windows.Handle
	onAction                        OnAction
	lang                            LangFunc
	scale                           float64
	clientH                         int
	palette                         colors
	controls                        map[uint16]windows.Handle
	labels                          map[uint16]string
	toggles, selected               map[uint16]bool
	disabled                        map[uint16]bool
	oldButtonProc                   map[windows.Handle]uintptr
	hoverID                         uint16
	themeSchedule                   string
}

// Show opens the panel or closes the currently open panel. It must be called
// from the thread that owns the application's Win32 message loop.
func Show(state State, onAction OnAction, langFn LangFunc) error {
	panelMu.Lock()
	if active != nil {
		hwnd := active.hwnd
		panelMu.Unlock()
		pDestroyWindow.Call(uintptr(hwnd))
		return nil
	}
	p := &panel{
		onAction:      onAction,
		lang:          langFn,
		scale:         1,
		themeSchedule: state.ThemeSchedule,
		controls:      make(map[uint16]windows.Handle),
		labels:        make(map[uint16]string),
		toggles: map[uint16]bool{
			idNoSleep: state.NoSleepEnabled, idProcess: state.ProcessWatchEnabled,
			idIdle: state.IdleEnabled, idTheme: state.ThemeSwitchEnabled,
			idBattery:    state.DarkOnBattery,
			idFullscreen: state.SkipFullscreen, idHotkeys: state.HotkeysEnabled,
			idAutostart: state.AutostartEnabled,
		},
		selected:      make(map[uint16]bool),
		disabled:      make(map[uint16]bool),
		oldButtonProc: make(map[windows.Handle]uintptr),
	}
	p.setChoice(timeIDs(), timeID(state.IdleTimeout))
	p.setChoice(actionIDs(), actionID(state.IdleAction))
	if state.IsChinese {
		p.setChoice(languageIDs(), idLangZH)
	} else {
		p.setChoice(languageIDs(), idLangEN)
	}
	active = p
	panelMu.Unlock()

	if err := ensureClass(); err != nil {
		clearPanel(p, 0)
		return err
	}
	if err := p.create(); err != nil {
		clearPanel(p, 0)
		return err
	}
	return nil
}

// Hide closes the currently visible panel.
func Hide() {
	panelMu.Lock()
	p := active
	panelMu.Unlock()
	if p != nil && p.hwnd != 0 {
		pDestroyWindow.Call(uintptr(p.hwnd))
	}
}

func ensureClass() error {
	classOnce.Do(func() {
		name, err := windows.UTF16PtrFromString(panelClass)
		if err != nil {
			classErr = err
			return
		}
		wc := wndClassExW{Size: uint32(unsafe.Sizeof(wndClassExW{})), WndProc: windows.NewCallback(wndProc), ClassName: name}
		if result, _, callErr := pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); result == 0 && callErr != syscall.Errno(1410) {
			classErr = fmt.Errorf("register popup class: %w", callErr)
		}
	})
	return classErr
}

func (p *panel) create() error {
	title, _ := windows.UTF16PtrFromString("IdleTrigger")
	name, _ := windows.UTF16PtrFromString(panelClass)
	var cursor struct{ X, Y int32 }
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&cursor)))
	style := uint32(wsPopup | wsCaption | wsSysMenu | wsClipChildren)
	exStyle := uint32(wsExToolWindow | wsExTopmost | wsExComposited)
	hwnd, _, callErr := pCreateWindowEx.Call(uintptr(exStyle), uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(title)), uintptr(style), uintptr(cursor.X), uintptr(cursor.Y), 1, 1, 0, 0, 0, 0)
	if hwnd == 0 {
		return fmt.Errorf("create control panel: %w", callErr)
	}
	p.hwnd = windows.Handle(hwnd)
	p.scale = dpiForWindow(p.hwnd)
	p.font = makeFont(p.scale, 13, 400)
	p.sectionFont = makeFont(p.scale, 13, 700)
	p.subtitleFont = makeFont(p.scale, 12, 600)
	p.refreshTheme(false)
	if err := p.build(); err != nil {
		pDestroyWindow.Call(uintptr(p.hwnd))
		return err
	}
	p.position(style, exStyle, cursor)
	pSetForeground.Call(uintptr(p.hwnd))
	return nil
}

func dpiForWindow(hwnd windows.Handle) float64 {
	dpi, _, _ := pGetDpiForWindow.Call(uintptr(hwnd))
	if dpi == 0 {
		dpi, _, _ = pGetDpiForSystem.Call()
	}
	if dpi == 0 {
		return 1
	}
	return float64(dpi) / 96
}

func makeFont(scale float64, size int32, weight int32) windows.Handle {
	type logFont struct {
		Height, Width, Escapement, Orientation, Weight       int32
		Italic, Underline, StrikeOut, CharSet                byte
		OutPrecision, ClipPrecision, Quality, PitchAndFamily byte
		FaceName                                             [32]uint16
	}
	lf := logFont{Height: -int32(float64(size)*scale + 0.5), Weight: weight, CharSet: 1}
	copy(lf.FaceName[:], windows.StringToUTF16("Microsoft YaHei UI"))
	result, _, _ := pCreateFont.Call(uintptr(unsafe.Pointer(&lf)))
	return windows.Handle(result)
}

func (p *panel) sc(value int) int { return int(float64(value)*p.scale + 0.5) }
func (p *panel) text(key string) string {
	if p.lang == nil {
		return key
	}
	return p.lang(key)
}

func (p *panel) child(className, text string, style uint32, x, y, width, height int, id uint16, font windows.Handle) (windows.Handle, error) {
	class, err := windows.UTF16PtrFromString(className)
	if err != nil {
		return 0, err
	}
	caption, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return 0, err
	}
	hwnd, _, callErr := pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(caption)), uintptr(style), uintptr(p.sc(x)), uintptr(p.sc(y)), uintptr(p.sc(width)), uintptr(p.sc(height)), uintptr(p.hwnd), uintptr(id), 0, 0)
	if hwnd == 0 {
		return 0, fmt.Errorf("create %s control: %w", className, callErr)
	}
	if font != 0 {
		pSendMessage.Call(hwnd, wmSetFont, uintptr(font), 1)
	}
	if id != 0 {
		p.controls[id] = windows.Handle(hwnd)
		p.labels[id] = text
	}
	return windows.Handle(hwnd), nil
}

func (p *panel) build() error {
	section := func(text string, y int) error {
		_, err := p.child("STATIC", text, wsChild|wsVisible|ssLeft, pad, y, baseW-2*pad, sectionH, 0, p.sectionFont)
		return err
	}
	subtitle := func(text string, x, y, width int) error {
		_, err := p.child("STATIC", text, wsChild|wsVisible|ssLeft, x, y, width, subtitleH, 0, p.subtitleFont)
		return err
	}
	button := func(text string, x, y, width, height int, id uint16) error {
		hwnd, err := p.child("BUTTON", text, wsChild|wsVisible|bsOwnerDraw, x, y, width, height, id, p.font)
		if err != nil {
			return err
		}
		p.subclassButton(hwnd)
		return err
	}
	choiceRow := func(x, y, totalW int, labels []string, ids []uint16) (int, error) {
		width := (totalW - (len(ids)-1)*gap) / len(ids)
		height := p.rowHeight(labels, width)
		for i, id := range ids {
			if err := button(labels[i], x+i*(width+gap), y, width, height, id); err != nil {
				return 0, err
			}
		}
		return height, nil
	}
	y := pad
	if err := section(p.text("menu_quick_actions"), y); err != nil {
		return err
	}
	y += sectionH + gap
	height, err := choiceRow(pad, y, baseW-2*pad, []string{p.text("menu_lock"), p.text("menu_sleep"), p.text("menu_hibernate"), p.text("menu_shutdown"), p.text("menu_restart")}, []uint16{idLock, idSleep, idHibernate, idShutdown, idRestart})
	if err != nil {
		return err
	}
	y += height + 2*gap
	if err := section(p.text("menu_nosleep"), y); err != nil {
		return err
	}
	y += sectionH + gap
	height, err = choiceRow(pad, y, baseW-2*pad, []string{p.text("menu_nosleep_enable"), p.text("menu_process_watch")}, []uint16{idNoSleep, idProcess})
	if err != nil {
		return err
	}
	y += height + 2*gap
	if err := section(p.text("menu_idle_enable"), y); err != nil {
		return err
	}
	y += sectionH + gap
	childX := pad
	childW := baseW - 2*pad
	if err := subtitle(p.text("menu_idle_basic"), childX, y, childW); err != nil {
		return err
	}
	y += subtitleH
	height = p.rowHeight([]string{p.text("menu_idle_enable")}, childW)
	if err := button(p.text("menu_idle_enable"), childX, y, childW, height, idIdle); err != nil {
		return err
	}
	y += height + gap
	if err := subtitle(p.text("menu_idle_timeout"), childX, y, childW); err != nil {
		return err
	}
	y += subtitleH
	height, err = choiceRow(childX, y, childW, timeoutLabels(p.lang), timeIDs())
	if err != nil {
		return err
	}
	y += height + gap
	if err := subtitle(p.text("menu_idle_action"), childX, y, childW); err != nil {
		return err
	}
	y += subtitleH
	height, err = choiceRow(childX, y, childW, []string{p.text("menu_action_sleep"), p.text("menu_action_hibernate"), p.text("menu_action_shutdown"), p.text("menu_action_lock")}, actionIDs())
	if err != nil {
		return err
	}
	y += height + 2*gap
	if err := section(p.text("menu_theme_switch"), y); err != nil {
		return err
	}
	y += sectionH + gap
	if p.themeSchedule != "" {
		if err := subtitle(p.themeSchedule, pad, y, baseW-2*pad); err != nil {
			return err
		}
		y += subtitleH + gap
	}
	height, err = choiceRow(pad, y, baseW-2*pad, []string{p.text("menu_theme_enable"), p.text("menu_theme_skip_fullscreen"), p.text("menu_theme_battery_dark")}, []uint16{idTheme, idFullscreen, idBattery})
	if err != nil {
		return err
	}
	y += height + gap
	height, err = choiceRow(pad, y, baseW-2*pad, []string{p.text("menu_theme_switch_now"), p.text("menu_theme_repair")}, []uint16{idThemeSwitch, idThemeRepair})
	if err != nil {
		return err
	}
	y += height + 2*gap
	if err := section(p.text("menu_preferences"), y); err != nil {
		return err
	}
	y += sectionH + gap
	height, err = choiceRow(pad, y, baseW-2*pad, []string{p.text("menu_hotkeys"), p.text("menu_autostart")}, []uint16{idHotkeys, idAutostart})
	if err != nil {
		return err
	}
	y += height + gap
	if err := subtitle(p.text("menu_language"), pad, y, baseW-2*pad); err != nil {
		return err
	}
	y += subtitleH
	height, err = choiceRow(pad, y, baseW-2*pad, []string{p.text("menu_lang_en"), p.text("menu_lang_zh")}, languageIDs())
	if err != nil {
		return err
	}
	y += height + gap
	if err := subtitle(p.text("menu_more"), pad, y, baseW-2*pad); err != nil {
		return err
	}
	y += subtitleH
	bottomLabels := []string{p.text("menu_open_config"), p.text("menu_about"), p.text("menu_exit")}
	bottomH := p.rowHeight(bottomLabels, (baseW-2*pad-2*gap)/3)
	p.clientH = y + bottomH + pad
	_, err = choiceRow(pad, y, baseW-2*pad, bottomLabels, []uint16{idConfig, idAbout, idExit})
	return err
}

func (p *panel) rowHeight(labels []string, width int) int {
	capacity := width / 7
	for _, label := range labels {
		if utf8.RuneCountInString(label) > capacity {
			return buttonH + 18
		}
	}
	return buttonH
}

func timeoutLabels(lang LangFunc) []string {
	if lang == nil {
		return []string{"5", "10", "30", "60", "120"}
	}
	return []string{lang("menu_timeout_5"), lang("menu_timeout_10"), lang("menu_timeout_30"), lang("menu_timeout_60"), lang("menu_timeout_120")}
}
func timeIDs() []uint16 { return []uint16{idTime5, idTime10, idTime30, idTime60, idTime120} }
func actionIDs() []uint16 {
	return []uint16{idActionSleep, idActionHibernate, idActionShutdown, idActionLock}
}
func languageIDs() []uint16 { return []uint16{idLangEN, idLangZH} }
func timeID(value int) uint16 {
	for i, timeout := range []int{5, 10, 30, 60, 120} {
		if value == timeout {
			return timeIDs()[i]
		}
	}
	return idTime30
}
func actionID(value string) uint16 {
	for i, action := range []string{"sleep", "hibernate", "shutdown", "lock"} {
		if value == action {
			return actionIDs()[i]
		}
	}
	return idActionSleep
}
func timeoutIndex(value int) int {
	for i, timeout := range []int{5, 10, 30, 60, 120} {
		if value == timeout {
			return i
		}
	}
	return 2
}
func actionIndex(value string) int {
	for i, action := range []string{"sleep", "hibernate", "shutdown", "lock"} {
		if value == action {
			return i
		}
	}
	return 0
}

func (p *panel) setChoice(group []uint16, selected uint16) {
	for _, id := range group {
		p.selected[id] = id == selected
	}
}
func (p *panel) toggle(id uint16) { p.toggles[id] = !p.toggles[id]; p.invalidate(id) }
func (p *panel) setToggle(id uint16, value bool) {
	p.toggles[id] = value
	p.invalidate(id)
}
func (p *panel) choose(group []uint16, selected uint16) {
	p.setChoice(group, selected)
	for _, id := range group {
		p.invalidate(id)
	}
}
func (p *panel) invalidate(id uint16) {
	if hwnd := p.controls[id]; hwnd != 0 {
		pInvalidateRect.Call(uintptr(hwnd), 0, 1)
	}
}

func (p *panel) subclassButton(hwnd windows.Handle) {
	if hwnd == 0 {
		return
	}
	old, _, _ := setWindowProc(hwnd, buttonProc)
	if old != 0 {
		p.oldButtonProc[hwnd] = old
	}
}

func setWindowProc(hwnd windows.Handle, proc uintptr) (uintptr, uintptr, error) {
	if unsafe.Sizeof(uintptr(0)) == 4 {
		return pSetWindowLong.Call(uintptr(hwnd), gwlpWndProc, proc)
	}
	return pSetWindowLongPtr.Call(uintptr(hwnd), gwlpWndProc, proc)
}

func buttonWndProc(hwnd windows.Handle, msg uint32, wp, lp uintptr) uintptr {
	p := panelForButton(hwnd)
	var old uintptr
	if p != nil {
		old = p.oldButtonProc[hwnd]
		switch msg {
		case wmMouseMove:
			p.setHover(hwnd)
		case wmMouseLeave:
			p.clearHover(hwnd)
		}
	}
	if old != 0 {
		result, _, _ := pCallWindowProc.Call(old, uintptr(hwnd), uintptr(msg), wp, lp)
		return result
	}
	result, _, _ := pDefWindowProc.Call(uintptr(hwnd), uintptr(msg), wp, lp)
	return result
}

func (p *panel) setHover(hwnd windows.Handle) {
	id := p.controlID(hwnd)
	if id == 0 {
		return
	}
	if p.hoverID != id {
		previous := p.hoverID
		p.hoverID = id
		if previous != 0 {
			p.invalidate(previous)
		}
		p.invalidate(id)
	}
	tme := trackMouseEvent{Size: uint32(unsafe.Sizeof(trackMouseEvent{})), Flags: tmeLeave, HwndTrack: hwnd}
	pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
}

func (p *panel) clearHover(hwnd windows.Handle) {
	id := p.controlID(hwnd)
	if id != 0 && p.hoverID == id {
		p.hoverID = 0
		p.invalidate(id)
	}
}

func (p *panel) controlID(hwnd windows.Handle) uint16 {
	for id, control := range p.controls {
		if control == hwnd {
			return id
		}
	}
	return 0
}

func (p *panel) position(style, exStyle uint32, cursor struct{ X, Y int32 }) {
	r := rect{Right: int32(p.sc(baseW)), Bottom: int32(p.sc(p.clientH))}
	pAdjustWindowRect.Call(uintptr(unsafe.Pointer(&r)), uintptr(style), 0, uintptr(exStyle))
	width, height := r.Right-r.Left, r.Bottom-r.Top
	monitor, _, _ := pMonitorFromWindow.Call(uintptr(p.hwnd), monitorNearest)
	info := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if monitor != 0 {
		pGetMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info)))
	} else {
		info.Work = rect{Right: width, Bottom: height}
	}
	x, y := cursor.X-width/2, cursor.Y-height-int32(p.sc(gap))
	if y < info.Work.Top {
		y = cursor.Y + int32(p.sc(gap))
	}
	if x < info.Work.Left {
		x = info.Work.Left
	}
	if x+width > info.Work.Right {
		x = info.Work.Right - width
	}
	if y < info.Work.Top {
		y = info.Work.Top
	}
	if y+height > info.Work.Bottom {
		y = info.Work.Bottom - height
	}
	pSetWindowPos.Call(uintptr(p.hwnd), ^uintptr(0), uintptr(x), uintptr(y), uintptr(width), uintptr(height), swpShowWindow)
}

func wndProc(hwnd windows.Handle, msg uint32, wp, lp uintptr) uintptr {
	p := panelFor(hwnd)
	if p != nil {
		switch msg {
		case wmActivate:
			if uint16(wp) == waInactive {
				Hide()
				return 0
			}
		case wmClose:
			Hide()
			return 0
		case wmDestroy:
			clearPanel(p, hwnd)
		case wmEraseBkgnd:
			p.fill(windows.Handle(wp), p.backgroundBrush)
			return 1
		case wmCtlColorStatic:
			pSetTextColor.Call(wp, uintptr(p.palette.text))
			pSetBkMode.Call(wp, transparent)
			return uintptr(p.backgroundBrush)
		case wmDrawItem:
			if lp != 0 {
				p.drawButton(drawItemFromLParam(lp))
				return 1
			}
		case wmSettingChange, wmSysColorChange, wmThemeChanged:
			p.refreshTheme(true)
		case wmCommand:
			if uint16(wp>>16) == bnClicked {
				p.handleCommand(uint16(wp))
				return 0
			}
		}
	}
	result, _, _ := pDefWindowProc.Call(uintptr(hwnd), uintptr(msg), wp, lp)
	return result
}

type drawItemPointer *drawItem

func drawItemFromLParam(lp uintptr) *drawItem {
	return *(*drawItemPointer)(unsafe.Pointer(&lp))
}

func (p *panel) handleCommand(id uint16) {
	if p.disabled[id] {
		return
	}
	var action Action
	value := 0
	switch {
	case id == idSleep:
		action = ActSleep
	case id == idHibernate:
		action = ActHibernate
	case id == idShutdown:
		action = ActShutdown
	case id == idLock:
		action = ActLock
	case id == idRestart:
		action = ActRestart
	case id == idNoSleep:
		p.toggle(id)
		if p.toggles[idNoSleep] {
			p.setToggle(idIdle, false)
		}
		action = ActNoSleepToggle
	case id == idProcess:
		p.toggle(id)
		action = ActProcessWatchToggle
	case id == idIdle:
		p.toggle(id)
		if p.toggles[idIdle] {
			p.setToggle(idNoSleep, false)
		}
		action = ActIdleToggle
	case id >= idTime5 && id <= idTime120:
		p.choose(timeIDs(), id)
		p.setToggle(idNoSleep, false)
		p.setToggle(idIdle, true)
		action = ActIdleTimeout
		value = int(id - idTime5)
	case id >= idActionSleep && id <= idActionLock:
		p.choose(actionIDs(), id)
		action = ActIdleAction
		value = int(id - idActionSleep)
	case id == idTheme:
		p.toggle(id)
		action = ActThemeToggle
	case id == idBattery:
		p.toggle(id)
		action = ActBatteryToggle
	case id == idFullscreen:
		p.toggle(id)
		action = ActFullscreenToggle
	case id == idThemeSwitch:
		action = ActSwitchTheme
	case id == idThemeRepair:
		action = ActRepairTheme
	case id == idHotkeys:
		p.toggle(id)
		action = ActHotkeyToggle
	case id == idAutostart:
		p.toggle(id)
		action = ActAutostartToggle
	case id == idLangEN:
		p.choose(languageIDs(), id)
		action = ActLanguage
		value = 0
	case id == idLangZH:
		p.choose(languageIDs(), id)
		action = ActLanguage
		value = 1
	case id == idConfig:
		action = ActConfig
	case id == idAbout:
		action = ActAbout
	case id == idExit:
		action = ActExit
	default:
		return
	}
	if action <= ActRestart || action == ActLanguage || action == ActConfig || action == ActAbout || action == ActExit || action == ActSwitchTheme || action == ActRepairTheme {
		Hide()
	}
	if p.onAction != nil {
		p.onAction(action, value)
	}
}

func (p *panel) refreshTheme(invalidate bool) {
	if themeswitch.Current() == themeswitch.ModeDark {
		p.palette = colors{background: rgb(28, 30, 33), surface: rgb(44, 47, 52), hover: rgb(55, 59, 65), border: rgb(76, 82, 90), accent: rgb(56, 139, 224), pressed: rgb(38, 108, 181), text: rgb(242, 244, 247), mutedText: rgb(169, 176, 185), accentText: rgb(255, 255, 255), disabled: rgb(39, 42, 46), danger: rgb(100, 45, 49), dangerHover: rgb(122, 52, 57), dangerPressed: rgb(83, 37, 42), dangerText: rgb(255, 239, 239)}
	} else {
		p.palette = colors{background: rgb(247, 249, 251), surface: rgb(255, 255, 255), hover: rgb(239, 244, 250), border: rgb(205, 213, 222), accent: rgb(0, 103, 192), pressed: rgb(0, 84, 156), text: rgb(26, 30, 36), mutedText: rgb(100, 108, 118), accentText: rgb(255, 255, 255), disabled: rgb(238, 241, 245), danger: rgb(255, 243, 243), dangerHover: rgb(255, 232, 232), dangerPressed: rgb(255, 214, 214), dangerText: rgb(159, 42, 42)}
	}
	p.releaseBrushes()
	p.backgroundBrush = makeBrush(p.palette.background)
	p.surfaceBrush = makeBrush(p.palette.surface)
	p.hoverBrush = makeBrush(p.palette.hover)
	p.borderBrush = makeBrush(p.palette.border)
	p.accentBrush = makeBrush(p.palette.accent)
	p.pressedBrush = makeBrush(p.palette.pressed)
	p.disabledBrush = makeBrush(p.palette.disabled)
	p.dangerBrush = makeBrush(p.palette.danger)
	p.dangerHoverBrush = makeBrush(p.palette.dangerHover)
	p.dangerPressedBrush = makeBrush(p.palette.dangerPressed)
	if invalidate && p.hwnd != 0 {
		pInvalidateRect.Call(uintptr(p.hwnd), 0, 1)
		for id := range p.controls {
			p.invalidate(id)
		}
	}
}

func (p *panel) drawButton(item *drawItem) {
	id := uint16(item.CtlID)
	selected := p.toggles[id] || p.selected[id]
	brush := p.surfaceBrush
	textColor := p.palette.text
	if p.hoverID == id || item.ItemState&odsHotlight != 0 {
		brush = p.hoverBrush
	}
	if id == idExit {
		brush = p.dangerBrush
		textColor = p.palette.dangerText
		if p.hoverID == id || item.ItemState&odsHotlight != 0 {
			brush = p.dangerHoverBrush
		}
	}
	if selected {
		brush = p.accentBrush
		textColor = p.palette.accentText
	}
	if item.ItemState&odsSelected != 0 {
		brush = p.pressedBrush
		textColor = p.palette.accentText
		if id == idExit {
			brush = p.dangerPressedBrush
			textColor = p.palette.dangerText
		}
	}
	if p.disabled[id] || item.ItemState&odsDisabled != 0 {
		brush = p.disabledBrush
		textColor = p.palette.mutedText
	}
	pFillRect.Call(uintptr(item.HDC), uintptr(unsafe.Pointer(&item.Rect)), uintptr(brush))
	pFrameRect.Call(uintptr(item.HDC), uintptr(unsafe.Pointer(&item.Rect)), uintptr(p.borderBrush))
	pSetTextColor.Call(uintptr(item.HDC), uintptr(textColor))
	pSetBkMode.Call(uintptr(item.HDC), transparent)
	old, _, _ := pSelectObject.Call(uintptr(item.HDC), uintptr(p.font))
	defer pSelectObject.Call(uintptr(item.HDC), old)
	text, _ := windows.UTF16PtrFromString(p.labels[id])
	r := item.Rect
	r.Left += int32(p.sc(8))
	r.Right -= int32(p.sc(8))
	drawTextCentered(item.HDC, text, r)
}

func drawTextCentered(dc windows.Handle, text *uint16, bounds rect) {
	measure := rect{Left: bounds.Left, Top: bounds.Top, Right: bounds.Right, Bottom: bounds.Bottom}
	pDrawText.Call(uintptr(dc), uintptr(unsafe.Pointer(text)), ^uintptr(0), uintptr(unsafe.Pointer(&measure)), dtCenter|dtWordBreak|dtCalcRect)
	textH := measure.Bottom - measure.Top
	if textH < bounds.Bottom-bounds.Top {
		bounds.Top += ((bounds.Bottom - bounds.Top) - textH) / 2
	}
	pDrawText.Call(uintptr(dc), uintptr(unsafe.Pointer(text)), ^uintptr(0), uintptr(unsafe.Pointer(&bounds)), dtCenter|dtVCenter|dtWordBreak)
}

func (p *panel) fill(dc, brush windows.Handle) {
	var r rect
	pGetClientRect.Call(uintptr(p.hwnd), uintptr(unsafe.Pointer(&r)))
	pFillRect.Call(uintptr(dc), uintptr(unsafe.Pointer(&r)), uintptr(brush))
}
func (p *panel) releaseBrushes() {
	for _, brush := range []windows.Handle{p.backgroundBrush, p.surfaceBrush, p.hoverBrush, p.borderBrush, p.accentBrush, p.pressedBrush, p.disabledBrush, p.dangerBrush, p.dangerHoverBrush, p.dangerPressedBrush} {
		if brush != 0 {
			pDeleteObject.Call(uintptr(brush))
		}
	}
	p.backgroundBrush = 0
	p.surfaceBrush = 0
	p.hoverBrush = 0
	p.borderBrush = 0
	p.accentBrush = 0
	p.pressedBrush = 0
	p.disabledBrush = 0
	p.dangerBrush = 0
	p.dangerHoverBrush = 0
	p.dangerPressedBrush = 0
}
func rgb(r, g, b byte) uint32 { return uint32(r) | uint32(g)<<8 | uint32(b)<<16 }
func makeBrush(color uint32) windows.Handle {
	result, _, _ := pCreateBrush.Call(uintptr(color))
	return windows.Handle(result)
}

func panelFor(hwnd windows.Handle) *panel {
	panelMu.Lock()
	defer panelMu.Unlock()
	if active != nil && active.hwnd == hwnd {
		return active
	}
	return nil
}

func panelForButton(hwnd windows.Handle) *panel {
	panelMu.Lock()
	defer panelMu.Unlock()
	if active == nil {
		return nil
	}
	for _, control := range active.controls {
		if control == hwnd {
			return active
		}
	}
	return nil
}

func clearPanel(p *panel, hwnd windows.Handle) {
	panelMu.Lock()
	if active != p || (hwnd != 0 && p.hwnd != hwnd) {
		panelMu.Unlock()
		return
	}
	active = nil
	panelMu.Unlock()
	for hwnd, old := range p.oldButtonProc {
		if hwnd != 0 && old != 0 {
			setWindowProc(hwnd, old)
		}
	}
	if p.font != 0 {
		pDeleteObject.Call(uintptr(p.font))
	}
	if p.sectionFont != 0 {
		pDeleteObject.Call(uintptr(p.sectionFont))
	}
	if p.subtitleFont != 0 {
		pDeleteObject.Call(uintptr(p.subtitleFont))
	}
	p.releaseBrushes()
	p.font = 0
	p.sectionFont = 0
	p.subtitleFont = 0
	p.hwnd = 0
}
