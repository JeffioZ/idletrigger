// Package popup implements the single-instance tray control panel.
package popup

import (
	"fmt"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/JeffioZ/idletrigger/internal/idlewarning"
	mylog "github.com/JeffioZ/idletrigger/internal/log"
	"github.com/JeffioZ/idletrigger/internal/systray"
	"github.com/JeffioZ/idletrigger/internal/uicolors"
	"github.com/JeffioZ/idletrigger/internal/uifont"
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
type point struct{ X, Y int32 }

type buttonRole uint8

const (
	buttonCommand buttonRole = iota
	buttonToggle
	buttonChoice
)

type buttonVisualState struct {
	Role                      buttonRole
	Active, Disabled          bool
	Hovered, Pressed, Focused bool
}

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

type toolInfo struct {
	Size     uint32
	Flags    uint32
	Hwnd     windows.Handle
	ID       uintptr
	Rect     rect
	Instance windows.Handle
	Text     *uint16
	LParam   uintptr
	Reserved uintptr
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
	ActIdleWarningToggle
	ActIdleEnhancedMonitorToggle
	ActThemeToggle
	ActBatteryToggle
	ActFullscreenToggle
	ActIPLocationToggle
	ActSwitchTheme
	ActRepairTheme
	ActHotkeyToggle
	ActAutostartToggle
	ActLoggingToggle
	ActLanguage
	ActConfig
	ActExit
)

type State struct {
	NoSleepEnabled, ProcessWatchEnabled, IdleEnabled, IdlePaused bool
	IdleWarningEnabled                                           bool
	IdleEnhancedMonitor                                          bool
	IdleTimeout                                                  int
	IdleWarningSeconds                                           int
	IdleAction                                                   string
	ProcessWatchList                                             []string
	ProcessWatchActive                                           bool
	ThemeSwitchEnabled, DarkOnBattery, SkipFullscreen            bool
	IPLocationEnabled                                            bool
	HotkeysEnabled, AutostartEnabled, LoggingEnabled             bool
	IsChinese                                                    bool
	ThemeSchedule                                                string
	IPLocationLabel                                              string
	AppVersion                                                   string
	Theme                                                        Theme
	Owner                                                        windows.Handle
	DeveloperCapturePanel, DeveloperWarningPreview               bool
}

type LangFunc func(key string) string
type OnAction func(action Action, value int)

const (
	panelClass = "IdleTriggerPopup"

	wmDestroy         = 0x0002
	wmClose           = 0x0010
	wmMouseMove       = 0x0200
	wmLButtonDown     = 0x0201
	wmLButtonUp       = 0x0202
	wmMouseWheel      = 0x020A
	wmMouseLeave      = 0x02A3
	wmParentNotify    = 0x0210
	wmEraseBkgnd      = 0x0014
	wmSysColorChange  = 0x0015
	wmSettingChange   = 0x001A
	wmDrawItem        = 0x002B
	wmCommand         = 0x0111
	wmCtlColorStatic  = 0x0138
	wmCtlColorEdit    = 0x0133
	wmCtlColorListBox = 0x0134
	wmThemeChanged    = 0x031A
	wmDpiChanged      = 0x02E0
	wmTimer           = 0x0113
	wmKeyDown         = 0x0100
	wmSysKeyDown      = 0x0104
	wmKillFocus       = 0x0008
	wmOpenChoice      = 0x8001
	wmSetFont         = 0x0030
	wmSetIcon         = 0x0080
	bnClicked         = 0

	wsOverlapped       = 0x00000000
	wsPopup            = 0x80000000
	wsCaption          = 0x00C00000
	wsSysMenu          = 0x00080000
	wsThickFrame       = 0x00040000
	wsMinimizeBox      = 0x00020000
	wsMaximizeBox      = 0x00010000
	wsClipChildren     = 0x02000000
	wsClipSiblings     = 0x04000000
	wsChild            = 0x40000000
	wsVisible          = 0x10000000
	wsTabStop          = 0x00010000
	wsVScroll          = 0x00200000
	wsOverlappedWindow = wsOverlapped | wsCaption | wsSysMenu | wsThickFrame | wsMinimizeBox | wsMaximizeBox
	bsOwnerDraw        = 0x0000000B
	ssOwnerDraw        = 0x0000000D
	ssLeft             = 0x00000000
	ssRight            = 0x00000002

	wsExToolWindow = 0x00000080
	wsExTopmost    = 0x00000008
	wsExComposited = 0x02000000
	wsExAppWindow  = 0x00040000

	swpNoSize         = 0x0001
	swpNoMove         = 0x0002
	swpNoZOrder       = 0x0004
	swpNoActivate     = 0x0010
	swpShowWindow     = 0x0040
	swHide            = 0
	swShow            = 5
	monitorNearest    = 2
	gwlpWndProc       = ^uintptr(3)
	quickMenuTimer    = 1
	languageMenuTimer = 2
	choiceMenuTimer   = 3
	vkUp              = 0x26
	vkDown            = 0x28
	vkHome            = 0x24
	vkEnd             = 0x23
	vkReturn          = 0x0D
	vkSpace           = 0x20
	vkF4              = 0x73
	vkEscape          = 0x1B

	odsSelected  = 0x0001
	odsDisabled  = 0x0004
	odsFocus     = 0x0010
	odsHotlight  = 0x0040
	psSolid      = 0
	dtCenter     = 0x00000001
	dtVCenter    = 0x00000004
	dtLeft       = 0x00000000
	dtWordBreak  = 0x00000010
	dtCalcRect   = 0x00000400
	dtSingleLine = 0x00000020
	transparent  = 1
	tmeLeave     = 0x00000002

	ttsAlwaysTip       = 0x0001
	ttsNoPrefix        = 0x0002
	ttfIDIsHwnd        = 0x0001
	ttfSubclass        = 0x0010
	ttmAddTool         = 0x0432
	ttmUpdateTipText   = 0x0439
	ttmSetTipBkColor   = 0x0413
	ttmSetTipTextColor = 0x0414
	ttmSetMaxTipWidth  = 0x0418
	cbAddString        = 0x0143
	cbSetCurSel        = 0x014E
	cbSetItemHeight    = 0x0153
)

const (
	idSleep = 1 + iota
	idHibernate
	idShutdown
	idLock
	idRestart
)

const (
	idQuickActions      = 6
	idQuickMenu         = 7
	idNoSleep           = 10
	idProcess           = 11
	idIdle              = 20
	idIdleWarning       = 21
	idIdleEnhanced      = 22
	idTheme             = 30
	idBattery           = 32
	idFullscreen        = 33
	idThemeSwitch       = 34
	idThemeRepair       = 35
	idIPLocation        = 36
	idHotkeys           = 40
	idAutostart         = 41
	idLogging           = 42
	idIdleTimeout       = 120
	idIdleAction        = 121
	idLanguage          = 150
	idLanguageMenu      = 151
	idLangEN            = 152
	idLangZH            = 153
	idConfig            = 500
	idExit              = 502
	idTestWarning       = 600
	idChoiceSurface     = 630
	idTimeoutOptionBase = 610
	// Keep the two dynamic option ranges disjoint. Timeout has 10 presets
	// (610-619), so action options must not start inside that range; otherwise
	// choiceOptionOwner can classify the same HWND ID as either selector.
	idActionOptionBase = 640
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	comctl   = windows.NewLazySystemDLL("comctl32.dll")
	dwmapi   = windows.NewLazySystemDLL("dwmapi.dll")
	uxtheme  = windows.NewLazySystemDLL("uxtheme.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	pCreateWindowEx        = user32.NewProc("CreateWindowExW")
	pLoadIcon              = user32.NewProc("LoadIconW")
	pLoadImage             = user32.NewProc("LoadImageW")
	pDestroyIcon           = user32.NewProc("DestroyIcon")
	pRegisterClassEx       = user32.NewProc("RegisterClassExW")
	pDestroyWindow         = user32.NewProc("DestroyWindow")
	pDefWindowProc         = user32.NewProc("DefWindowProcW")
	pCallWindowProc        = user32.NewProc("CallWindowProcW")
	pSendMessage           = user32.NewProc("SendMessageW")
	pSetWindowLong         = user32.NewProc("SetWindowLongW")
	pSetWindowLongPtr      = user32.NewProc("SetWindowLongPtrW")
	pSetWindowPos          = user32.NewProc("SetWindowPos")
	pPostMessage           = user32.NewProc("PostMessageW")
	pGetFocus              = user32.NewProc("GetFocus")
	pGetCursorPos          = user32.NewProc("GetCursorPos")
	pMonitorFromWindow     = user32.NewProc("MonitorFromWindow")
	pGetMonitorInfo        = user32.NewProc("GetMonitorInfoW")
	pAdjustWindowRect      = user32.NewProc("AdjustWindowRectEx")
	pGetDpiForWindow       = user32.NewProc("GetDpiForWindow")
	pGetDpiForSystem       = user32.NewProc("GetDpiForSystem")
	pSetForeground         = user32.NewProc("SetForegroundWindow")
	pUpdateWindow          = user32.NewProc("UpdateWindow")
	pSetFocus              = user32.NewProc("SetFocus")
	pShowWindow            = user32.NewProc("ShowWindow")
	pEnableWindow          = user32.NewProc("EnableWindow")
	pSetTimer              = user32.NewProc("SetTimer")
	pKillTimer             = user32.NewProc("KillTimer")
	pFillRect              = user32.NewProc("FillRect")
	pFrameRect             = user32.NewProc("FrameRect")
	pDrawText              = user32.NewProc("DrawTextW")
	pInvalidateRect        = user32.NewProc("InvalidateRect")
	pGetClientRect         = user32.NewProc("GetClientRect")
	pGetWindowRect         = user32.NewProc("GetWindowRect")
	pScreenToClient        = user32.NewProc("ScreenToClient")
	pGetDC                 = user32.NewProc("GetDC")
	pReleaseDC             = user32.NewProc("ReleaseDC")
	pTrackMouseEvent       = user32.NewProc("TrackMouseEvent")
	pDeleteObject          = gdi32.NewProc("DeleteObject")
	pCreateBrush           = gdi32.NewProc("CreateSolidBrush")
	pCreatePen             = gdi32.NewProc("CreatePen")
	pRoundRect             = gdi32.NewProc("RoundRect")
	pSetTextColor          = gdi32.NewProc("SetTextColor")
	pSetBkColor            = gdi32.NewProc("SetBkColor")
	pSetBkMode             = gdi32.NewProc("SetBkMode")
	pMoveToEx              = gdi32.NewProc("MoveToEx")
	pLineTo                = gdi32.NewProc("LineTo")
	pSelectObject          = gdi32.NewProc("SelectObject")
	pInitCommonControlsEx  = comctl.NewProc("InitCommonControlsEx")
	pDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
	pSetWindowTheme        = uxtheme.NewProc("SetWindowTheme")
	pGetModuleHandle       = kernel32.NewProc("GetModuleHandleW")

	classOnce sync.Once
	classErr  error
	panelMu   sync.Mutex
	active    *panel

	buttonProc = windows.NewCallback(buttonWndProc)
)

type staticKind uint8

const (
	staticNone staticKind = iota
	staticSection
	staticSubtitle
	staticQuickMenu
)

type trackMouseEvent struct {
	Size      uint32
	Flags     uint32
	HwndTrack windows.Handle
	HoverTime uint32
}

type initCommonControlsEx struct {
	Size uint32
	ICC  uint32
}

type panel struct {
	hwnd windows.Handle
	// panelResources owns every GDI and icon handle created for this panel.
	// It is embedded only to keep the drawing code compact; creation, refresh,
	// and destruction are centralized in resources.go.
	panelResources
	onAction                OnAction
	lang                    LangFunc
	theme                   Theme
	metrics                 popupMetrics
	clientH                 int
	appVersion              string
	idleTimeout             int
	idleWarningSeconds      int
	idleAction              string
	isChinese               bool
	owner                   windows.Handle
	iconThemeDark           bool
	iconsInitialized        bool
	tooltip                 windows.Handle
	palette                 uicolors.Palette
	fontChoice              uifont.Choice
	controls                map[uint16]windows.Handle
	labels                  map[uint16]string
	staticKinds             map[uint16]staticKind
	tooltips                map[uint16][]uint16
	toggles, selected       map[uint16]bool
	disabled                map[uint16]bool
	oldButtonProc           map[windows.Handle]uintptr
	hoverID                 uint16
	keyboardNavigation      bool
	nextStaticID            uint16
	themeScheduleID         uint16
	idlePaused              bool
	processWatchList        []string
	processWatchActive      bool
	developerCapturePanel   bool
	developerWarningPreview bool
	quickMenuOpen           bool
	languageMenuOpen        bool
	themeSchedule           string
	ipLocationLabel         string
	timeoutOptions          []timeoutChoice
	choice                  choiceSurface
	themeRefreshing         bool
	style, exStyle          uint32
	controlBounds           map[uint16]logicalBounds
	captureScale            float64
	captureHost             bool
}

// choiceSurface owns the transient controls used by the two value selectors.
// The panel remains responsible for business callbacks and the selector
// buttons; this type only owns option HWNDs and viewport/lifecycle state.
type choiceSurface struct {
	options        map[uint16][]string
	selected       map[uint16]int
	openID         uint16
	focusOnOpen    bool
	restoreFocus   bool
	optionIDs      map[uint16][]uint16
	optionControls map[uint16]windows.Handle
	scroll         map[uint16]int
	visible        map[uint16]int
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
	panelMu.Unlock()
	return createPanel(state, onAction, langFn)
}

// Refresh rebuilds an already visible panel instead of treating it as a
// toggle. Native controls store captions at creation time, so callers use
// this after a language change to update every caption immediately.
func Refresh(state State, onAction OnAction, langFn LangFunc) error {
	panelMu.Lock()
	old := active
	panelMu.Unlock()
	if old != nil && old.hwnd != 0 {
		pDestroyWindow.Call(uintptr(old.hwnd))
	}
	return createPanel(state, onAction, langFn)
}

func createPanel(state State, onAction OnAction, langFn LangFunc) error {
	_, err := createPanelForHost(state, onAction, langFn, 0, false)
	return err
}

// Capture creates a real panel without initializing the tray. The capture
// callback owns only its temporary resources; popup always destroys the HWND
// and its fonts, brushes, and icons before returning.
func Capture(state State, langFn LangFunc, scale float64, capture func(windows.Handle) error) error {
	p, err := createPanelForHost(state, nil, langFn, scale, true)
	if err != nil {
		return err
	}
	defer func() {
		if p.hwnd != 0 {
			pDestroyWindow.Call(uintptr(p.hwnd))
		}
	}()
	pUpdateWindow.Call(uintptr(p.hwnd))
	return capture(p.hwnd)
}

func createPanelForHost(state State, onAction OnAction, langFn LangFunc, captureScale float64, captureHost bool) (*panel, error) {
	p := &panel{
		onAction:                onAction,
		lang:                    langFn,
		theme:                   state.Theme,
		metrics:                 newPopupMetrics(defaultPopupStyle, 1),
		themeSchedule:           state.ThemeSchedule,
		ipLocationLabel:         state.IPLocationLabel,
		appVersion:              state.AppVersion,
		idlePaused:              state.IdlePaused,
		idleTimeout:             state.IdleTimeout,
		idleWarningSeconds:      state.IdleWarningSeconds,
		idleAction:              state.IdleAction,
		isChinese:               state.IsChinese,
		owner:                   state.Owner,
		processWatchList:        append([]string(nil), state.ProcessWatchList...),
		processWatchActive:      state.ProcessWatchActive,
		developerCapturePanel:   state.DeveloperCapturePanel,
		developerWarningPreview: state.DeveloperWarningPreview,
		controls:                make(map[uint16]windows.Handle),
		labels:                  make(map[uint16]string),
		staticKinds:             make(map[uint16]staticKind),
		nextStaticID:            700,
		tooltips:                make(map[uint16][]uint16),
		toggles: map[uint16]bool{
			idNoSleep: state.NoSleepEnabled, idProcess: state.ProcessWatchEnabled,
			idIdle: state.IdleEnabled && !state.IdlePaused, idIdleWarning: state.IdleWarningEnabled,
			idIdleEnhanced: state.IdleEnhancedMonitor,
			idTheme:        state.ThemeSwitchEnabled,
			idBattery:      state.DarkOnBattery,
			idFullscreen:   state.SkipFullscreen,
			idIPLocation:   state.IPLocationEnabled,
			idHotkeys:      state.HotkeysEnabled,
			idAutostart:    state.AutostartEnabled, idLogging: state.LoggingEnabled,
		},
		selected:      make(map[uint16]bool),
		disabled:      make(map[uint16]bool),
		oldButtonProc: make(map[windows.Handle]uintptr),
		controlBounds: make(map[uint16]logicalBounds),
		choice: choiceSurface{
			options:        make(map[uint16][]string),
			selected:       make(map[uint16]int),
			optionIDs:      make(map[uint16][]uint16),
			optionControls: make(map[uint16]windows.Handle),
			scroll:         make(map[uint16]int),
			visible:        make(map[uint16]int),
		},
		captureScale: captureScale,
		captureHost:  captureHost,
	}
	if state.IsChinese {
		p.setChoice(languageIDs(), idLangZH)
	} else {
		p.setChoice(languageIDs(), idLangEN)
	}
	panelMu.Lock()
	active = p
	panelMu.Unlock()

	if err := ensureClass(); err != nil {
		clearPanel(p, 0)
		return nil, err
	}
	if err := p.create(); err != nil {
		clearPanel(p, 0)
		return nil, err
	}
	return p, nil
}

// Hide destroys the currently visible panel and releases its native resources.
func Hide() {
	panelMu.Lock()
	p := active
	panelMu.Unlock()
	if p != nil && p.hwnd != 0 {
		pDestroyWindow.Call(uintptr(p.hwnd))
	}
}

// UpdateThemeSchedule refreshes the already visible Day/Night schedule line.
// It is intentionally layout-preserving; callers should still recreate the
// panel when a future change needs to add or remove controls.
func UpdateThemeSchedule(text, ipLocationLabel string) {
	panelMu.Lock()
	p := active
	if p == nil || p.themeScheduleID == 0 {
		panelMu.Unlock()
		return
	}
	id := p.themeScheduleID
	hwnd := p.controls[id]
	p.labels[id] = text
	p.ipLocationLabel = ipLocationLabel
	panelMu.Unlock()

	if hwnd != 0 {
		pInvalidateRect.Call(uintptr(hwnd), 0, 1)
	}
	p.refreshTooltip(idIPLocation)
}

func Destroy() {
	Hide()
}

func ensureClass() error {
	classOnce.Do(func() {
		name, err := windows.UTF16PtrFromString(panelClass)
		if err != nil {
			classErr = err
			return
		}
		module, _, _ := pGetModuleHandle.Call(0)
		fallbackIcon := classFallbackIcon(module)
		wc := wndClassExW{
			Size:      uint32(unsafe.Sizeof(wndClassExW{})),
			WndProc:   windows.NewCallback(wndProc),
			Instance:  windows.Handle(module),
			Icon:      fallbackIcon,
			ClassName: name,
			IconSm:    fallbackIcon,
		}
		if result, _, callErr := pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); result == 0 && callErr != syscall.Errno(1410) {
			classErr = fmt.Errorf("register popup class: %w", callErr)
		}
	})
	return classErr
}

const (
	appIconResourceID       = 2 // Main executable icon; shared class fallback only.
	trayDarkIconResourceID  = 3 // Dark mark for a light title bar.
	trayLightIconResourceID = 4 // Light mark for a dark title bar.
)

// classFallbackIcon is loaded through LoadIconW, whose resource handle is
// shared by the module and must not be destroyed. Every real panel instance
// immediately replaces it with the theme-specific, owned WM_SETICON handles.
func classFallbackIcon(module uintptr) windows.Handle {
	if module == 0 {
		return 0
	}
	icon, _, _ := pLoadIcon.Call(module, appIconResourceID)
	return windows.Handle(icon)
}

func windowIconResourceID(dark bool) uintptr {
	if dark {
		return trayLightIconResourceID
	}
	return trayDarkIconResourceID
}

func windowIcon(dark bool, size int) windows.Handle {
	module, _, _ := pGetModuleHandle.Call(0)
	if module == 0 {
		return 0
	}
	const imageIcon = 1
	icon, _, _ := pLoadImage.Call(module, windowIconResourceID(dark), imageIcon, uintptr(size), uintptr(size), 0)
	return windows.Handle(icon)
}

func shouldReloadWindowIcons(initialized, currentDark, requestedDark, force bool) bool {
	return !initialized || force || currentDark != requestedDark
}

func (p *panel) setWindowIcons(dark, force bool) {
	if p.hwnd == 0 || !shouldReloadWindowIcons(p.iconsInitialized, p.iconThemeDark, dark, force) {
		return
	}
	largeIcon := windowIcon(dark, p.sc(p.metrics.style.Control.IconLarge))
	smallIcon := windowIcon(dark, p.sc(p.metrics.style.Control.IconSmall))
	if largeIcon == 0 || smallIcon == 0 {
		if largeIcon != 0 {
			pDestroyIcon.Call(uintptr(largeIcon))
		}
		if smallIcon != 0 {
			pDestroyIcon.Call(uintptr(smallIcon))
		}
		mylog.Info("UI icon: surface=popup load failed theme_dark=%v dpi=%d", dark, int(p.metrics.scale*96+0.5))
		return
	}
	oldLargeIcon, oldSmallIcon := p.largeIcon, p.smallIcon
	p.largeIcon, p.smallIcon = largeIcon, smallIcon
	p.iconThemeDark, p.iconsInitialized = dark, true
	const (
		iconSmall = 0
		iconBig   = 1
	)
	pSendMessage.Call(uintptr(p.hwnd), wmSetIcon, iconBig, uintptr(p.largeIcon))
	pSendMessage.Call(uintptr(p.hwnd), wmSetIcon, iconSmall, uintptr(p.smallIcon))
	for _, icon := range []windows.Handle{oldLargeIcon, oldSmallIcon} {
		if icon != 0 {
			pDestroyIcon.Call(uintptr(icon))
		}
	}
}

func (p *panel) create() error {
	titleText := "IdleTrigger"
	if p.appVersion != "" && p.appVersion != "dev" {
		titleText += " v" + p.appVersion
	}
	title, _ := windows.UTF16PtrFromString(titleText)
	name, _ := windows.UTF16PtrFromString(panelClass)
	var cursor struct{ X, Y int32 }
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&cursor)))
	style := uint32(wsPopup | wsCaption | wsSysMenu | wsClipChildren)
	exStyle := uint32(wsExTopmost | wsExComposited)
	owner := uintptr(p.owner)
	if p.developerCapturePanel || p.captureHost {
		// Capture mode uses a normal app window shape so screenshot tools can
		// detect the whole panel instead of treating it as a transient tray popup.
		style = uint32(wsOverlappedWindow | wsClipChildren)
		exStyle = uint32(wsExAppWindow | wsExComposited)
		owner = 0
	}
	hwnd, _, callErr := pCreateWindowEx.Call(uintptr(exStyle), uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(title)), uintptr(style), uintptr(cursor.X), uintptr(cursor.Y), 1, 1, owner, 0, 0, 0)
	if hwnd == 0 {
		return fmt.Errorf("create control panel: %w", callErr)
	}
	p.hwnd = windows.Handle(hwnd)
	p.style, p.exStyle = style, exStyle
	scale := dpiForWindow(p.hwnd)
	if p.captureScale > 0 {
		scale = p.captureScale
	}
	p.metrics = newPopupMetrics(defaultPopupStyle, scale)
	p.font = p.makeFont(p.metrics.style.Fonts.BodySize, p.metrics.style.Fonts.BodyWeight)
	p.sectionFont = p.makeFont(p.metrics.style.Fonts.SectionSize, p.metrics.style.Fonts.SectionWeight)
	p.subtitleFont = p.makeFont(p.metrics.style.Fonts.SubtitleSize, p.metrics.style.Fonts.SubtitleWeight)
	p.createTooltip()
	if p.font == 0 || p.sectionFont == 0 || p.subtitleFont == 0 {
		pDestroyWindow.Call(uintptr(p.hwnd))
		return fmt.Errorf("create control panel fonts failed")
	}
	mylog.Info("UI font: surface=popup ui_language=%s system_language=%s system_locale=%s face=%q reason=%s dpi=%d body_px=%d", p.fontChoice.UILanguage, p.fontChoice.SystemLanguage, p.fontChoice.SystemLocale, p.fontChoice.Face, p.fontChoice.Reason, int(p.metrics.scale*96+0.5), p.sc(int(p.metrics.style.Fonts.BodySize)))
	p.refreshTheme(false)
	if err := p.build(); err != nil {
		pDestroyWindow.Call(uintptr(p.hwnd))
		return err
	}
	if !p.captureHost {
		systray.SetTabNavigationWindow(p.hwnd, p.enterKeyboardNavigation)
	}
	p.position(style, exStyle)
	if !p.captureHost {
		pSetForeground.Call(uintptr(p.hwnd))
	}
	return nil
}

func (p *panel) createTooltip() {
	icc := initCommonControlsEx{Size: uint32(unsafe.Sizeof(initCommonControlsEx{})), ICC: 0x000000FF}
	pInitCommonControlsEx.Call(uintptr(unsafe.Pointer(&icc)))
	class, err := windows.UTF16PtrFromString("tooltips_class32")
	if err != nil {
		return
	}
	hwnd, _, callErr := pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(class)), 0, uintptr(wsPopup|ttsAlwaysTip|ttsNoPrefix), 0, 0, 0, 0, uintptr(p.hwnd), 0, 0, 0)
	if hwnd == 0 {
		_ = callErr
		return
	}
	p.tooltip = windows.Handle(hwnd)
	pSendMessage.Call(hwnd, ttmSetMaxTipWidth, 0, uintptr(p.sc(360)))
	// Keep the native tooltip from flashing back immediately when the pointer
	// moves a few pixels across an owner-drawn control.
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

func (p *panel) makeFont(size int32, weight int32) windows.Handle {
	font, choice := uifont.New(int32(float64(size)*p.metrics.scale+0.5), weight, p.isChinese)
	if p.fontChoice.Face == "" {
		p.fontChoice = choice
	}
	return font
}

func (p *panel) sc(value int) int { return p.metrics.px(value) }
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
		p.controlBounds[id] = logicalBounds{x, y, width, height}
		p.labels[id] = text
		p.addTooltip(id, windows.Handle(hwnd))
	}
	return windows.Handle(hwnd), nil
}

func (p *panel) addTooltip(id uint16, hwnd windows.Handle) {
	if p.tooltip == 0 || hwnd == 0 {
		return
	}
	text := p.tooltipText(id)
	if text == "" {
		return
	}
	p.tooltips[id] = windows.StringToUTF16(text)
	ti := toolInfo{
		Size:  uint32(unsafe.Sizeof(toolInfo{})),
		Flags: ttfIDIsHwnd | ttfSubclass,
		Hwnd:  p.hwnd,
		ID:    uintptr(hwnd),
		Text:  &p.tooltips[id][0],
	}
	pSendMessage.Call(uintptr(p.tooltip), ttmAddTool, 0, uintptr(unsafe.Pointer(&ti)))
}

func (p *panel) refreshTooltip(id uint16) {
	if p.tooltip == 0 || p.controls[id] == 0 {
		return
	}
	text := p.tooltipText(id)
	if text == "" {
		return
	}
	p.tooltips[id] = windows.StringToUTF16(text)
	ti := toolInfo{
		Size:  uint32(unsafe.Sizeof(toolInfo{})),
		Flags: ttfIDIsHwnd,
		Hwnd:  p.hwnd,
		ID:    uintptr(p.controls[id]),
		Text:  &p.tooltips[id][0],
	}
	pSendMessage.Call(uintptr(p.tooltip), ttmUpdateTipText, 0, uintptr(unsafe.Pointer(&ti)))
}

func (p *panel) tooltipText(id uint16) string {
	key := ""
	switch id {
	case idQuickActions:
		key = "tip_quick_actions"
	case idLock:
		key = "tip_lock"
	case idSleep:
		key = "tip_sleep"
	case idHibernate:
		key = "tip_hibernate"
	case idShutdown:
		key = "tip_shutdown"
	case idRestart:
		key = "tip_restart"
	case idNoSleep:
		key = "tip_nosleep"
	case idProcess:
		return p.withStateTooltip(id, p.processWatchTooltip())
	case idIdle:
		key = "tip_idle"
	case idIdleWarning:
		key = "tip_idle_warning"
	case idIdleEnhanced:
		key = "tip_idle_enhanced"
	case idIdleTimeout:
		key = "tip_idle_timeout"
	case idIdleAction:
		key = "tip_idle_action"
	case idTheme:
		key = "tip_theme"
	case idFullscreen:
		key = "tip_fullscreen"
	case idBattery:
		key = "tip_battery_theme"
	case idIPLocation:
		body := p.text("tip_ip_location")
		if p.ipLocationLabel != "" {
			body = fmt.Sprintf(p.text("tip_ip_location_current"), p.ipLocationLabel, body)
		}
		return p.withStateTooltip(id, body)
	case idThemeSwitch:
		key = "tip_theme_switch"
	case idThemeRepair:
		key = "tip_theme_repair"
	case idHotkeys:
		key = "tip_hotkeys"
	case idAutostart:
		key = "tip_autostart"
	case idLogging:
		key = "tip_logging"
	case idLanguage:
		key = "tip_language"
	case idConfig:
		key = "tip_config"
	case idExit:
		key = "tip_exit"
	}
	if key == "" {
		return ""
	}
	return p.withStateTooltip(id, p.text(key))
}

func (p *panel) withStateTooltip(id uint16, body string) string {
	state := p.visualState(id)
	switch state.Role {
	case buttonToggle:
		key := "tip_state_disabled"
		if state.Active {
			key = "tip_state_enabled"
		}
		return fmt.Sprintf(p.text("tip_toggle_state"), p.text(key), body)
	case buttonChoice:
		key := "tip_state_not_selected"
		if state.Active {
			key = "tip_state_selected"
		}
		return fmt.Sprintf(p.text("tip_choice_state"), p.text(key), body)
	default:
		return body
	}
}

func (p *panel) processWatchTooltip() string {
	if len(p.processWatchList) == 0 {
		return p.text("tip_process_watch_empty")
	}
	status := p.text("tip_process_watch_waiting")
	if p.processWatchActive {
		status = p.text("tip_process_watch_active")
	}
	return fmt.Sprintf(p.text("tip_process_watch_configured"), status, strings.Join(p.processWatchList, ", "))
}

func (p *panel) build() error {
	layout := p.metrics.style.Layout
	baseW, pad, gap := layout.PanelWidth, layout.Padding, layout.Gap
	sectionGap, labelGap := layout.SectionGap, layout.LabelGap
	buttonH, sectionH, subtitleH := layout.ButtonHeight, layout.SectionHeight, layout.SubtitleHeight
	section := func(text string, y int) error {
		id := p.staticID(staticSection)
		_, err := p.child("STATIC", text, wsChild|wsVisible|ssOwnerDraw, pad, y, baseW-2*pad, sectionH, id, 0)
		return err
	}
	subtitle := func(text string, x, y, width int) error {
		id := p.staticID(staticSubtitle)
		_, err := p.child("STATIC", text, wsChild|wsVisible|ssOwnerDraw, x, y, width, subtitleH, id, 0)
		return err
	}
	button := func(text string, x, y, width, height int, id uint16) error {
		hwnd, err := p.child("BUTTON", text, wsChild|wsVisible|wsTabStop|bsOwnerDraw, x, y, width, height, id, p.font)
		if err != nil {
			return err
		}
		p.subclassButton(hwnd)
		return err
	}
	choice := func(id uint16, x, y, width, height int, options []string, current int) error {
		if err := button(options[current], x, y, width, height, id); err != nil {
			return err
		}
		p.choice.options[id] = append([]string(nil), options...)
		p.choice.selected[id] = current
		p.labels[id] = options[current]
		base := idTimeoutOptionBase
		if id == idIdleAction {
			base = idActionOptionBase
		}
		ids := make([]uint16, len(options))
		for i := range options {
			ids[i] = uint16(base + i)
			if err := createMenuOption(p, options[i], 0, 0, 1, 1, ids[i]); err != nil {
				return err
			}
			p.choice.optionControls[ids[i]] = p.controls[ids[i]]
		}
		p.choice.optionIDs[id] = ids
		return nil
	}
	choiceRow := func(x, y, totalW int, labels []string, ids []uint16) (int, error) {
		width := splitRow(totalW, len(ids), gap)
		height := p.rowHeight(labels, width)
		for i, id := range ids {
			if err := button(labels[i], x+i*(width+gap), y, width, height, id); err != nil {
				return 0, err
			}
		}
		return height, nil
	}
	choiceRowWidths := func(x, y int, labels []string, ids, widths []uint16) (int, error) {
		height := buttonH
		for i, width := range widths {
			if candidate := p.rowHeight([]string{labels[i]}, int(width)); candidate > height {
				height = candidate
			}
		}
		for i, id := range ids {
			if err := button(labels[i], x, y, int(widths[i]), height, id); err != nil {
				return 0, err
			}
			x += int(widths[i]) + gap
		}
		return height, nil
	}
	y := pad
	if err := section(p.text("menu_nosleep"), y); err != nil {
		return err
	}
	y += sectionH + labelGap
	height, err := choiceRow(pad, y, baseW-2*pad, []string{p.text("menu_nosleep_enable"), p.text("menu_process_watch")}, []uint16{idNoSleep, idProcess})
	if err != nil {
		return err
	}
	y += height + sectionGap
	if err := section(p.text("menu_idle_enable"), y); err != nil {
		return err
	}
	y += sectionH + labelGap
	childX := pad
	childW := baseW - 2*pad
	idleLabel := p.text("menu_idle_enable_short")
	if p.idlePaused {
		idleLabel = p.text("menu_idle_paused")
	}
	height, err = choiceRow(childX, y, childW, []string{idleLabel, p.text("menu_idle_warning"), p.text("menu_idle_enhanced")}, []uint16{idIdle, idIdleWarning, idIdleEnhanced})
	if err != nil {
		return err
	}
	y += height + gap
	fieldW := (childW - gap) / 2
	if p.developerWarningPreview {
		if err := button(p.text("msg_idle_warning_test"), childX, y, fieldW, buttonH, idTestWarning); err != nil {
			return err
		}
		y += buttonH + gap
	}
	if err := subtitle(p.text("menu_idle_timeout"), childX, y, fieldW); err != nil {
		return err
	}
	secondX := childX + fieldW + gap
	if err := subtitle(p.text("menu_idle_action"), secondX, y, fieldW); err != nil {
		return err
	}
	y += subtitleH + labelGap
	if err := choice(idIdleTimeout, childX, y, fieldW, buttonH, timeoutLabels(p.idleTimeout, p.isChinese, &p.timeoutOptions), timeoutIndex(p.timeoutOptions, p.idleTimeout)); err != nil {
		return err
	}
	if err := choice(idIdleAction, secondX, y, fieldW, buttonH, actionLabels(p), actionIndex(p.idleAction)); err != nil {
		return err
	}
	y += buttonH + sectionGap
	if err := section(p.text("menu_theme_switch"), y); err != nil {
		return err
	}
	y += sectionH + labelGap
	if p.themeSchedule != "" {
		id := p.staticID(staticSubtitle)
		p.themeScheduleID = id
		if _, err := p.child("STATIC", p.themeSchedule, wsChild|wsVisible|ssOwnerDraw, pad, y, baseW-2*pad, subtitleH, id, 0); err != nil {
			return err
		}
		y += subtitleH + labelGap
	}
	height, err = choiceRow(pad, y, baseW-2*pad, []string{p.text("menu_theme_enable"), p.text("menu_theme_skip_fullscreen")}, []uint16{idTheme, idFullscreen})
	if err != nil {
		return err
	}
	y += height + 4
	height, err = choiceRow(pad, y, baseW-2*pad, []string{p.text("menu_theme_battery_dark"), p.text("menu_theme_ip_location")}, []uint16{idBattery, idIPLocation})
	if err != nil {
		return err
	}
	y += height + gap
	height, err = choiceRow(pad, y, baseW-2*pad, []string{p.text("menu_theme_switch_now"), p.text("menu_theme_repair")}, []uint16{idThemeSwitch, idThemeRepair})
	if err != nil {
		return err
	}
	y += height + sectionGap
	if err := section(p.text("menu_preferences"), y); err != nil {
		return err
	}
	y += sectionH + labelGap
	generalLabels := []string{p.text("menu_hotkeys"), p.text("menu_autostart"), p.text("menu_logging")}
	generalIDs := []uint16{idHotkeys, idAutostart, idLogging}
	if p.isChinese {
		height, err = choiceRow(pad, y, baseW-2*pad, generalLabels, generalIDs)
	} else {
		// English labels are shorter than their Chinese counterparts. Keep this
		// row compact so the final two toggles read as one related group.
		height, err = choiceRowWidths(pad, y, generalLabels, generalIDs, []uint16{160, 126, 126})
	}
	if err != nil {
		return err
	}
	y += height + sectionGap
	bottomW := (baseW - 2*pad - gap) / 2
	bottomRow1 := []string{p.text("menu_system_controls"), p.text("menu_language_settings")}
	bottomRow2 := []string{p.text("menu_open_config"), p.text("menu_exit_panel")}
	bottomH := p.rowHeight(append(bottomRow1, bottomRow2...), bottomW)
	p.clientH = y + bottomH*2 + gap + pad
	if _, err = choiceRow(pad, y, baseW-2*pad, bottomRow1, []uint16{idQuickActions, idLanguage}); err != nil {
		return err
	}
	if _, err = choiceRow(pad, y+bottomH+gap, baseW-2*pad, bottomRow2, []uint16{idConfig, idExit}); err != nil {
		return err
	}
	quickW := bottomW
	menuRowH := layout.QuickMenuRowHeight
	menuH := 8 + len(quickActionIDs())*menuRowH
	menuY := y - menuH
	if err := createMenuSurface(p, idQuickMenu, pad, menuY, quickW, menuH); err != nil {
		return err
	}
	for i, id := range quickActionIDs() {
		if err := buttonHidden(p, p.text(quickActionTranslationKey(id)), pad+4, menuY+4+i*menuRowH, quickW-8, menuRowH, id); err != nil {
			return err
		}
	}
	languageX := pad + bottomW + gap
	languageMenuH := 8 + len(languageIDs())*menuRowH
	languageMenuY := y - languageMenuH
	if err := createMenuSurface(p, idLanguageMenu, languageX, languageMenuY, bottomW, languageMenuH); err != nil {
		return err
	}
	for i, id := range languageIDs() {
		if err := buttonHidden(p, []string{p.text("menu_lang_en"), p.text("menu_lang_zh")}[i], languageX+4, languageMenuY+4+i*menuRowH, bottomW-8, menuRowH, id); err != nil {
			return err
		}
	}
	p.applyDependentStates()
	return nil
}

func createMenuSurface(p *panel, id uint16, x, y, width, height int) error {
	p.staticKinds[id] = staticQuickMenu
	_, err := p.child("STATIC", "", wsChild|ssOwnerDraw, x, y, width, height, id, 0)
	return err
}

func buttonHidden(p *panel, text string, x, y, width, height int, id uint16) error {
	return createMenuOption(p, text, x, y, width, height, id)
}

// createMenuOption is shared by the fixed quick/language menus and the
// dynamic choice surface. Keeping creation in one path guarantees identical
// class, style, font, owner-draw and subclass behavior.
func createMenuOption(p *panel, text string, x, y, width, height int, id uint16) error {
	hwnd, err := p.child("BUTTON", text, wsChild|wsTabStop|bsOwnerDraw, x, y, width, height, id, p.font)
	if err != nil {
		return err
	}
	p.subclassButton(hwnd)
	return nil
}

func quickActionTranslationKey(id uint16) string {
	switch id {
	case idLock:
		return "menu_lock"
	case idSleep:
		return "menu_sleep"
	case idHibernate:
		return "menu_hibernate"
	case idShutdown:
		return "menu_shutdown"
	default:
		return "menu_restart"
	}
}

func (p *panel) staticID(kind staticKind) uint16 {
	id := p.nextStaticID
	p.nextStaticID++
	p.staticKinds[id] = kind
	return id
}

func (p *panel) rowHeight(labels []string, width int) int {
	if p.hwnd == 0 || p.font == 0 {
		return p.metrics.style.Layout.ButtonHeight
	}
	dc, _, _ := pGetDC.Call(uintptr(p.hwnd))
	if dc == 0 {
		return p.metrics.style.Layout.ButtonHeight
	}
	defer pReleaseDC.Call(uintptr(p.hwnd), dc)
	old, _, _ := pSelectObject.Call(dc, uintptr(p.font))
	defer pSelectObject.Call(dc, old)

	rowH := p.metrics.style.Layout.ButtonHeight
	availableW := int32(p.sc(width - 16))
	for _, label := range labels {
		text, err := windows.UTF16PtrFromString(label)
		if err != nil {
			continue
		}
		bounds := rect{Right: availableW}
		pDrawText.Call(dc, uintptr(unsafe.Pointer(text)), ^uintptr(0), uintptr(unsafe.Pointer(&bounds)), dtCenter|dtWordBreak|dtCalcRect)
		textH := int(float64(bounds.Bottom-bounds.Top)/p.metrics.scale + 0.999)
		if candidate := textH + 12; candidate > rowH {
			rowH = candidate
		}
	}
	return rowH
}

type timeoutChoice struct {
	minutes int
	label   string
}

// Keep the complete list short enough to fit in the popup without a special
// scrolling path. These are the currently supported minute/hour presets;
// unsupported persisted values normalize to the default 30-minute preset.
var timeoutMinutes = []int{1, 2, 3, 5, 10, 15, 30, 60, 120, 300}

func supportedTimeout(minutes int) int {
	for _, supported := range timeoutMinutes {
		if supported == minutes {
			return minutes
		}
	}
	return 30
}

func timeoutChoices(current int, chinese bool) ([]timeoutChoice, int) {
	current = supportedTimeout(current)
	choices := make([]timeoutChoice, 0, len(timeoutMinutes))
	selected := -1
	for _, minutes := range timeoutMinutes {
		choices = append(choices, timeoutChoice{minutes: minutes, label: formatTimeout(minutes, chinese)})
		if minutes == current {
			selected = len(choices) - 1
		}
	}
	if selected >= 0 {
		return choices, selected
	}
	return choices, timeoutIndex(choices, current)
}

func formatTimeout(minutes int, chinese bool) string {
	if minutes%60 == 0 {
		if chinese {
			return fmt.Sprintf("%d 小时", minutes/60)
		}
		if minutes == 60 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", minutes/60)
	}
	if chinese {
		return fmt.Sprintf("%d 分钟", minutes)
	}
	if minutes == 1 {
		return "1 minute"
	}
	return fmt.Sprintf("%d minutes", minutes)
}

func timeoutLabels(current int, chinese bool, options *[]timeoutChoice) []string {
	*options, _ = timeoutChoices(current, chinese)
	labels := make([]string, len(*options))
	for i, option := range *options {
		labels[i] = option.label
	}
	return labels
}

func timeoutIndex(options []timeoutChoice, current int) int {
	current = supportedTimeout(current)
	for i, option := range options {
		if option.minutes == current {
			return i
		}
	}
	return 0
}

func quickActionIDs() []uint16 {
	return []uint16{idLock, idSleep, idHibernate, idShutdown, idRestart}
}

func languageIDs() []uint16 { return []uint16{idLangEN, idLangZH} }

func actionIndex(value string) int {
	for i, action := range []string{"sleep", "hibernate", "shutdown", "lock"} {
		if value == action {
			return i
		}
	}
	return 0
}

func actionLabels(p *panel) []string {
	return []string{p.text("menu_action_sleep"), p.text("menu_action_hibernate"), p.text("menu_action_shutdown"), p.text("menu_action_lock")}
}

func roleForButton(id uint16) buttonRole {
	switch id {
	case idNoSleep, idProcess, idIdle, idIdleWarning, idIdleEnhanced,
		idTheme, idBattery, idFullscreen, idIPLocation, idHotkeys, idAutostart, idLogging:
		return buttonToggle
	case idLangEN, idLangZH:
		return buttonChoice
	default:
		return buttonCommand
	}
}

func visualStateForButton(id uint16, toggleOn, choiceSelected, disabled bool) buttonVisualState {
	role := roleForButton(id)
	active := false
	switch role {
	case buttonToggle:
		active = toggleOn
	case buttonChoice:
		active = choiceSelected
	}
	return buttonVisualState{Role: role, Active: active, Disabled: disabled}
}

func (p *panel) visualState(id uint16) buttonVisualState {
	return visualStateForButton(id, p.toggles[id], p.selected[id], p.disabled[id])
}

// controlState is the single source for owner-drawn button states. The
// semantic state comes from the panel model; transient state comes from the
// current Win32 draw notification and never escapes into tray business state.
func (p *panel) controlState(id uint16, itemState uint32) buttonVisualState {
	state := p.visualState(id)
	if owner, index, ok := choiceOptionOwner(p, id); ok {
		state.Active = p.choice.selected[owner] == index
	}
	state.Hovered = p.hoverID == id || itemState&odsHotlight != 0
	// Native BUTTON hotlight can outlive WM_MOUSELEAVE while the choice
	// surface is being hidden. Choice triggers use the panel's tracked hover
	// state exclusively; their open appearance is handled separately below.
	if id == idIdleTimeout || id == idIdleAction {
		state.Hovered = p.hoverID == id
	}
	state.Pressed = itemState&odsSelected != 0
	state.Focused = p.shouldDrawFocusOutline(itemState)
	return state
}

func focusOutlineUsesLightOnAccent(active bool) bool { return active }

func isMenuTrigger(id uint16) bool { return id == idQuickActions || id == idLanguage }

func actionTranslationKey(value string) string {
	switch value {
	case "sleep", "hibernate", "shutdown", "lock":
		return "menu_action_" + value
	default:
		return "menu_action_sleep"
	}
}

func (p *panel) setChoice(group []uint16, selected uint16) {
	for _, id := range group {
		p.selected[id] = id == selected
	}
}
func (p *panel) toggle(id uint16) {
	p.toggles[id] = !p.toggles[id]
	p.refreshTooltip(id)
	p.invalidate(id)
}
func (p *panel) setToggle(id uint16, value bool) {
	p.toggles[id] = value
	p.refreshTooltip(id)
	p.invalidate(id)
}
func (p *panel) choose(group []uint16, selected uint16) {
	p.setChoice(group, selected)
	for _, id := range group {
		p.refreshTooltip(id)
		p.invalidate(id)
	}
}
func (p *panel) invalidate(id uint16) {
	if hwnd := p.controls[id]; hwnd != 0 {
		pInvalidateRect.Call(uintptr(hwnd), 0, 1)
	}
}

func (p *panel) openQuickMenu(focusFirst bool) {
	if p.choice.openID != 0 {
		return
	}
	if p.quickMenuOpen {
		pKillTimer.Call(uintptr(p.hwnd), quickMenuTimer)
		if focusFirst {
			pSetFocus.Call(uintptr(p.controls[quickActionIDs()[0]]))
		}
		return
	}
	p.quickMenuOpen = true
	p.closeLanguageMenu()
	for _, id := range append([]uint16{idQuickMenu}, quickActionIDs()...) {
		if hwnd := p.controls[id]; hwnd != 0 {
			pSetWindowPos.Call(uintptr(hwnd), 0, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate|swpShowWindow)
		}
	}
	if focusFirst {
		pSetFocus.Call(uintptr(p.controls[quickActionIDs()[0]]))
	}
}

func (p *panel) scheduleQuickMenuClose() {
	if p.quickMenuOpen {
		pSetTimer.Call(uintptr(p.hwnd), quickMenuTimer, 220, 0)
	}
}

func (p *panel) closeQuickMenu() {
	if !p.quickMenuOpen {
		return
	}
	pKillTimer.Call(uintptr(p.hwnd), quickMenuTimer)
	p.quickMenuOpen = false
	for _, id := range append([]uint16{idQuickMenu}, quickActionIDs()...) {
		if hwnd := p.controls[id]; hwnd != 0 {
			pShowWindow.Call(uintptr(hwnd), swHide)
		}
	}
}

func (p *panel) openLanguageMenu(focusFirst bool) {
	if p.choice.openID != 0 {
		return
	}
	if p.languageMenuOpen {
		pKillTimer.Call(uintptr(p.hwnd), languageMenuTimer)
		if focusFirst {
			p.focusUnselectedLanguage()
		}
		return
	}
	p.closeQuickMenu()
	p.languageMenuOpen = true
	for _, id := range append([]uint16{idLanguageMenu}, languageIDs()...) {
		if hwnd := p.controls[id]; hwnd != 0 {
			pSetWindowPos.Call(uintptr(hwnd), 0, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate|swpShowWindow)
		}
	}
	if focusFirst {
		p.focusUnselectedLanguage()
	}
}

func (p *panel) focusUnselectedLanguage() {
	id := uint16(idLangEN)
	if p.selected[id] {
		id = idLangZH
	}
	pSetFocus.Call(uintptr(p.controls[id]))
}

func (p *panel) scheduleLanguageMenuClose() {
	if p.languageMenuOpen {
		pSetTimer.Call(uintptr(p.hwnd), languageMenuTimer, 220, 0)
	}
}

func (p *panel) closeLanguageMenu() {
	if !p.languageMenuOpen {
		return
	}
	pKillTimer.Call(uintptr(p.hwnd), languageMenuTimer)
	p.languageMenuOpen = false
	for _, id := range append([]uint16{idLanguageMenu}, languageIDs()...) {
		if hwnd := p.controls[id]; hwnd != 0 {
			pShowWindow.Call(uintptr(hwnd), swHide)
		}
	}
}

func (p *panel) setDisabled(id uint16, value bool) {
	if p.disabled[id] == value {
		return
	}
	p.disabled[id] = value
	if hwnd := p.controls[id]; hwnd != 0 {
		enabled := uintptr(1)
		if value {
			enabled = 0
		}
		pEnableWindow.Call(uintptr(hwnd), enabled)
	}
	p.refreshTooltip(id)
	p.invalidate(id)
}

func (p *panel) applyDependentStates() {
	p.setDisabled(idProcess, !p.toggles[idNoSleep])
	monitorEnabled := p.toggles[idIdle]
	p.setDisabled(idIdleWarning, !monitorEnabled)
	p.setDisabled(idIdleEnhanced, !monitorEnabled)
	p.setDisabled(idTestWarning, !monitorEnabled)
	themeEnabled := p.toggles[idTheme]
	p.setDisabled(idFullscreen, !themeEnabled)
	p.setDisabled(idBattery, !themeEnabled)
	p.setDisabled(idIPLocation, !themeEnabled)
}

func (p *panel) setKeyboardNavigation(active bool) {
	if p.keyboardNavigation == active {
		return
	}
	p.keyboardNavigation = active
	for id := range p.controls {
		p.invalidate(id)
	}
}

func (p *panel) enterKeyboardNavigation() { p.setKeyboardNavigation(true) }

func (p *panel) leaveKeyboardNavigation() { p.setKeyboardNavigation(false) }

func (p *panel) shouldDrawFocusOutline(itemState uint32) bool {
	return p.keyboardNavigation && itemState&odsFocus != 0
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
		case wmMouseWheel:
			if owner, _, ok := choiceOptionOwner(p, p.controlID(hwnd)); ok {
				delta := 1
				if int16(wp>>16) > 0 {
					delta = -1
				}
				p.scrollChoice(owner, delta)
				return 0
			}
		case wmMouseLeave:
			p.clearHover(hwnd)
		case wmKillFocus:
			if _, _, ok := choiceOptionOwner(p, p.controlID(hwnd)); ok {
				focus, _, _ := pGetFocus.Call()
				if !p.choiceFocusContains(windows.Handle(focus)) {
					p.closeChoice(false)
				}
			}
		case wmLButtonDown:
			id := p.controlID(hwnd)
			if id == idQuickActions || id == idLanguage {
				// These menu triggers intentionally open on hover only. Keep Space
				// available through WM_COMMAND for keyboard navigation.
				p.leaveKeyboardNavigation()
				return 0
			}
			if id == idIdleTimeout || id == idIdleAction {
				// Choice controls follow the existing hover-only menu semantics;
				// mouse clicks must not leave a pressed/focused blue frame behind.
				p.leaveKeyboardNavigation()
				p.requestChoice(id, false)
				return 0
			}
			p.leaveKeyboardNavigation()
		case wmLButtonUp:
			id := p.controlID(hwnd)
			if id == idQuickActions || id == idLanguage || id == idIdleTimeout || id == idIdleAction {
				return 0
			}
		case wmKeyDown:
			id := p.controlID(hwnd)
			if owner, index, ok := choiceOptionOwner(p, id); ok {
				switch wp {
				case vkUp:
					p.focusChoice(owner, index, -1)
					return 0
				case vkDown:
					p.focusChoice(owner, index, 1)
					return 0
				case vkHome:
					p.focusChoice(owner, index, -index)
					return 0
				case vkEnd:
					p.focusChoice(owner, index, len(p.choice.options[owner])-1-index)
					return 0
				case vkEscape:
					p.closeChoice(true)
					return 0
				case vkF4:
					p.closeChoice(true)
					return 0
				case vkReturn, vkSpace:
					p.applyChoice(id)
					return 0
				}
			}
			if id == idIdleTimeout || id == idIdleAction {
				switch wp {
				case vkReturn, vkSpace, vkUp, vkDown, vkF4:
					p.requestChoice(id, true)
					return 0
				}
			}
			if containsQuickAction(id) {
				switch wp {
				case vkUp:
					p.focusQuickAction(id, -1)
					return 0
				case vkDown:
					p.focusQuickAction(id, 1)
					return 0
				case vkEscape:
					p.closeQuickMenu()
					pSetFocus.Call(uintptr(p.controls[idQuickActions]))
					return 0
				}
			}
			if containsLanguageOption(id) {
				switch wp {
				case vkUp, vkDown:
					other := uint16(idLangEN)
					if id == idLangEN {
						other = uint16(idLangZH)
					}
					pSetFocus.Call(uintptr(p.controls[other]))
					return 0
				case vkEscape:
					p.closeLanguageMenu()
					pSetFocus.Call(uintptr(p.controls[idLanguage]))
					return 0
				}
			}
		case wmSysKeyDown:
			id := p.controlID(hwnd)
			if (wp == vkDown || wp == vkF4) && (id == idIdleTimeout || id == idIdleAction) {
				p.requestChoice(id, false)
				return 0
			}
		}
	}
	if old != 0 {
		result, _, _ := pCallWindowProc.Call(old, uintptr(hwnd), uintptr(msg), wp, lp)
		return result
	}
	result, _, _ := pDefWindowProc.Call(uintptr(hwnd), uintptr(msg), wp, lp)
	return result
}

func (p *panel) focusQuickAction(current uint16, delta int) {
	ids := quickActionIDs()
	for index, id := range ids {
		if id == current {
			next := (index + delta + len(ids)) % len(ids)
			pSetFocus.Call(uintptr(p.controls[ids[next]]))
			return
		}
	}
}

func (p *panel) focusChoice(owner uint16, current, delta int) {
	ids := p.choice.optionIDs[owner]
	if len(ids) == 0 {
		return
	}
	next := current + delta
	if next < 0 {
		next = 0
	}
	if next >= len(ids) {
		next = len(ids) - 1
	}
	visible := p.choice.visible[owner]
	if visible > 0 {
		start := p.choice.scroll[owner]
		if next < start {
			start = next
		} else if next >= start+visible {
			start = next - visible + 1
		}
		if start < 0 {
			start = 0
		}
		maxStart := len(ids) - visible
		if maxStart < 0 {
			maxStart = 0
		}
		if start > maxStart {
			start = maxStart
		}
		if start != p.choice.scroll[owner] {
			p.choice.scroll[owner] = start
			p.positionChoiceLayer(owner, len(ids))
		}
	}
	if hwnd := p.choice.optionControls[ids[next]]; hwnd != 0 {
		pSetFocus.Call(uintptr(hwnd))
	}
}

func (p *panel) scrollChoice(owner uint16, delta int) {
	ids := p.choice.optionIDs[owner]
	if len(ids) == 0 || p.choice.visible[owner] <= 0 {
		return
	}
	start := p.choice.scroll[owner] + delta
	maxStart := len(ids) - p.choice.visible[owner]
	if start < 0 {
		start = 0
	}
	if start > len(ids) {
		start = len(ids)
	}
	if start > maxStart {
		start = maxStart
	}
	if start == p.choice.scroll[owner] {
		return
	}
	p.choice.scroll[owner] = start
	p.positionChoiceLayer(owner, len(ids))
}

func (p *panel) choicePointerInside() bool {
	var cursor point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&cursor)))
	inside := func(hwnd windows.Handle) bool {
		if hwnd == 0 {
			return false
		}
		var bounds rect
		pGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&bounds)))
		return cursor.X >= bounds.Left && cursor.X < bounds.Right && cursor.Y >= bounds.Top && cursor.Y < bounds.Bottom
	}
	if inside(p.controls[p.choice.openID]) || inside(p.controls[idChoiceSurface]) {
		return true
	}
	ids := p.choice.optionIDs[p.choice.openID]
	start, count := p.choice.scroll[p.choice.openID], p.choice.visible[p.choice.openID]
	if start < 0 {
		start = 0
	}
	if count > len(ids)-start {
		count = len(ids) - start
	}
	for _, optionID := range ids[start : start+count] {
		if inside(p.choice.optionControls[optionID]) {
			return true
		}
	}
	return false
}

func (p *panel) choiceFocusContains(hwnd windows.Handle) bool {
	if hwnd == 0 || hwnd == p.hwnd {
		return hwnd != 0
	}
	for _, option := range p.choice.optionControls {
		if option == hwnd {
			return true
		}
	}
	return false
}

func choiceOptionOwner(p *panel, id uint16) (uint16, int, bool) {
	for owner, ids := range p.choice.optionIDs {
		for i, optionID := range ids {
			if optionID == id {
				return owner, i, true
			}
		}
	}
	return 0, 0, false
}

func (p *panel) openChoice(id uint16) {
	if p.disabled[id] {
		return
	}
	if p.choice.openID == id {
		return
	}
	if p.choice.openID != 0 {
		p.closeChoice(false)
	}
	p.closeQuickMenu()
	p.closeLanguageMenu()
	options := p.choice.options[id]
	if len(options) == 0 {
		return
	}
	if p.choice.selected[id] < 0 || p.choice.selected[id] >= len(options) {
		p.choice.selected[id] = 0
	}
	p.choice.openID = id
	p.choice.restoreFocus = p.choice.focusOnOpen
	p.choice.scroll[id] = 0
	if p.choice.focusOnOpen {
		p.choice.scroll[id] = p.choice.selected[id]
	}
	p.enterKeyboardNavigation()
	if p.controls[idChoiceSurface] == 0 {
		if err := createMenuSurface(p, idChoiceSurface, 0, 0, 1, 1); err != nil {
			p.choice.openID = 0
			return
		}
	}
	for i, optionID := range p.choice.optionIDs[id] {
		p.labels[optionID] = options[i]
		if hwnd := p.choice.optionControls[optionID]; hwnd != 0 {
			pShowWindow.Call(uintptr(hwnd), swHide)
		}
	}
	p.positionChoiceLayer(id, len(options))
	if p.choice.focusOnOpen {
		if ids := p.choice.optionIDs[id]; len(ids) > p.choice.selected[id] {
			pSetFocus.Call(uintptr(p.controls[ids[p.choice.selected[id]]]))
		}
	}
	p.choice.focusOnOpen = false
}

func (p *panel) requestChoice(id uint16, focus bool) {
	p.choice.focusOnOpen = focus
	pPostMessage.Call(uintptr(p.hwnd), wmOpenChoice, uintptr(id), 0)
}

func (p *panel) positionChoiceLayer(id uint16, count int) {
	var buttonRect rect
	pGetWindowRect.Call(uintptr(p.controls[id]), uintptr(unsafe.Pointer(&buttonRect)))
	menuWidth := p.controlBounds[id].width
	if menuWidth <= 0 {
		menuWidth = 200
	}
	// Match the trigger's outer border rather than its interior client pixels;
	// the shared token keeps this compensation DPI-scaled and reviewable.
	width := p.sc(menuWidth + p.metrics.style.Control.MenuSurfaceWidthCompensation)
	monitor, _, _ := pMonitorFromWindow.Call(uintptr(p.hwnd), monitorNearest)
	info := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	pGetMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info)))
	// The choice surface is flush with the trigger. The menu's own inset
	// supplies the visual breathing room, matching the existing quick/language
	// surfaces instead of leaving a gap between control and menu.
	margin := int32(0)
	x, y := buttonRect.Left, buttonRect.Bottom+margin
	if x+int32(width) > info.Work.Right {
		x = info.Work.Right - int32(width)
	}
	if x < info.Work.Left {
		x = info.Work.Left
	}
	origin := point{X: x, Y: y}
	pScreenToClient.Call(uintptr(p.hwnd), uintptr(unsafe.Pointer(&origin)))
	rowH := p.metrics.style.Layout.QuickMenuRowHeight
	var client rect
	pGetClientRect.Call(uintptr(p.hwnd), uintptr(unsafe.Pointer(&client)))
	availableDown := int(client.Bottom-origin.Y) - int(p.sc(8))
	availableUp := int(origin.Y) - int(p.sc(8))
	maxRows := availableDown / p.sc(rowH)
	if maxRows < 1 && availableUp > availableDown {
		maxRows = availableUp / p.sc(rowH)
		if maxRows < 1 {
			maxRows = 1
		}
		origin.Y = int32(origin.Y) - int32(p.sc(8+maxRows*rowH)) - margin
	} else if maxRows < 1 {
		maxRows = 1
	}
	if maxRows > count {
		maxRows = count
	}
	start := p.choice.scroll[id]
	if start < 0 {
		start = 0
	}
	if maxStart := count - maxRows; start > maxStart {
		start = maxStart
	}
	if start < 0 {
		start = 0
	}
	p.choice.scroll[id] = start
	p.choice.visible[id] = maxRows
	menuHeight := 8 + maxRows*rowH
	if surface := p.controls[idChoiceSurface]; surface != 0 {
		// Keep the surface above the panel's other child controls so its border
		// is not covered by the rows below/behind it.
		pSetWindowPos.Call(uintptr(surface), 0, uintptr(origin.X), uintptr(origin.Y), uintptr(width), uintptr(p.sc(menuHeight)), swpShowWindow)
		pUpdateWindow.Call(uintptr(surface))
	}
	for i, optionID := range p.choice.optionIDs[id] {
		if hwnd := p.choice.optionControls[optionID]; hwnd != 0 {
			if i < start || i >= start+maxRows {
				pShowWindow.Call(uintptr(hwnd), swHide)
				continue
			}
			ox := origin.X + int32(p.sc(4))
			oy := origin.Y + int32(p.sc(4+(i-start)*rowH))
			pSetWindowPos.Call(uintptr(hwnd), 0, uintptr(ox), uintptr(oy), uintptr(width-p.sc(8)), uintptr(p.sc(rowH)), swpShowWindow)
			pUpdateWindow.Call(uintptr(hwnd))
		}
	}
}

func (p *panel) closeChoice(returnFocus bool) {
	if p.choice.openID == 0 {
		return
	}
	openID := p.choice.openID
	if p.hoverID != 0 {
		previous := p.hoverID
		p.hoverID = 0
		p.invalidate(previous)
	}
	for _, hwnd := range p.choice.optionControls {
		if hwnd != 0 {
			pShowWindow.Call(uintptr(hwnd), swHide)
		}
	}
	delete(p.choice.scroll, openID)
	delete(p.choice.visible, openID)
	if surface := p.controls[idChoiceSurface]; surface != 0 {
		pShowWindow.Call(uintptr(surface), swHide)
	}
	p.choice.openID = 0
	p.choice.restoreFocus = false
	// Both triggers can have been painted with the open/hover surface before
	// the close timer runs. Repaint them after clearing openID so neither keeps
	// a stale hover background when the surface is hidden.
	for _, id := range []uint16{idIdleTimeout, idIdleAction} {
		p.invalidate(id)
		if hwnd := p.controls[id]; hwnd != 0 {
			pUpdateWindow.Call(uintptr(hwnd))
		}
	}
	if returnFocus && p.hwnd != 0 && openID != 0 {
		pSetFocus.Call(uintptr(p.controls[openID]))
	}
}

func (p *panel) applyChoice(optionID uint16) {
	owner, index, ok := choiceOptionOwner(p, optionID)
	if !ok {
		return
	}
	if p.choice.openID != owner {
		return
	}
	if p.choice.selected[owner] == index {
		// Match the existing menu semantics: clicking the already-applied
		// option is a no-op. Keep the surface open and do not move focus or
		// invoke the business callback.
		return
	}
	p.choice.selected[owner] = index
	p.labels[owner] = p.choice.options[owner][index]
	p.invalidate(owner)
	if owner == idIdleTimeout {
		p.applyTimeoutChoice(index)
	} else {
		p.applyActionChoice(index)
	}
	if p.hwnd != 0 {
		p.closeChoice(p.choice.restoreFocus)
	}
}

func (p *panel) applyTimeoutChoice(index int) {
	if index >= len(p.timeoutOptions) {
		return
	}
	p.setToggle(idNoSleep, false)
	p.setToggle(idIdle, true)
	p.applyDependentStates()
	if p.onAction != nil {
		p.onAction(ActIdleTimeout, p.timeoutOptions[index].minutes)
	}
}

func (p *panel) applyActionChoice(index int) {
	if index >= 4 {
		return
	}
	p.idleAction = []string{"sleep", "hibernate", "shutdown", "lock"}[index]
	if p.onAction != nil {
		p.onAction(ActIdleAction, index)
	}
}

func (p *panel) setHover(hwnd windows.Handle) {
	id := p.controlID(hwnd)
	if _, _, ok := choiceOptionOwner(p, id); ok {
		pKillTimer.Call(uintptr(p.hwnd), choiceMenuTimer)
	}
	if p.choice.openID == 0 && (id == idQuickActions || containsQuickAction(id)) {
		p.openQuickMenu(false)
	}
	if p.choice.openID == 0 && (id == idLanguage || containsLanguageOption(id)) {
		p.openLanguageMenu(false)
	}
	if id == idIdleTimeout || id == idIdleAction {
		p.requestChoice(id, false)
	}
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
	if id == idQuickActions || containsQuickAction(id) {
		p.scheduleQuickMenuClose()
	}
	if id == idLanguage || containsLanguageOption(id) {
		p.scheduleLanguageMenuClose()
	}
	if id == idIdleTimeout || id == idIdleAction {
		if p.choice.openID != 0 {
			pSetTimer.Call(uintptr(p.hwnd), choiceMenuTimer, 220, 0)
		}
	} else if _, _, ok := choiceOptionOwner(p, id); ok && p.choice.openID != 0 {
		// Leaving an option into the surface border or outside the menu should
		// close it just like leaving the trigger; entering another option cancels
		// this timer in setHover.
		pSetTimer.Call(uintptr(p.hwnd), choiceMenuTimer, 220, 0)
	}
}

func containsLanguageOption(id uint16) bool { return id == idLangEN || id == idLangZH }

func containsQuickAction(id uint16) bool {
	for _, quickID := range quickActionIDs() {
		if id == quickID {
			return true
		}
	}
	return false
}

func (p *panel) controlID(hwnd windows.Handle) uint16 {
	for id, control := range p.controls {
		if control == hwnd {
			return id
		}
	}
	return 0
}

func panelOrigin(work rect, width, height, margin int32) (int32, int32) {
	x := work.Right - width - margin
	y := work.Bottom - height - margin
	if x < work.Left {
		x = work.Left
	}
	if y < work.Top {
		y = work.Top
	}
	return x, y
}

func (p *panel) position(style, exStyle uint32) {
	r := rect{Right: int32(p.sc(p.metrics.style.Layout.PanelWidth)), Bottom: int32(p.sc(p.clientH))}
	pAdjustWindowRect.Call(uintptr(unsafe.Pointer(&r)), uintptr(style), 0, uintptr(exStyle))
	width, height := r.Right-r.Left, r.Bottom-r.Top
	monitor, _, _ := pMonitorFromWindow.Call(uintptr(p.hwnd), monitorNearest)
	info := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if monitor != 0 {
		pGetMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info)))
	} else {
		info.Work = rect{Right: width, Bottom: height}
	}
	x, y := panelOrigin(info.Work, width, height, int32(p.sc(16)))
	insertAfter := ^uintptr(0)
	if p.developerCapturePanel || p.captureHost {
		insertAfter = 0
	}
	pSetWindowPos.Call(uintptr(p.hwnd), insertAfter, uintptr(x), uintptr(y), uintptr(width), uintptr(height), swpShowWindow)
}

func wndProc(hwnd windows.Handle, msg uint32, wp, lp uintptr) uintptr {
	p := panelFor(hwnd)
	if p != nil {
		switch msg {
		case wmClose:
			Hide()
			return 0
		case wmLButtonDown:
			p.leaveKeyboardNavigation()
		case wmParentNotify:
			if uint16(wp) == wmLButtonDown {
				p.leaveKeyboardNavigation()
			}
		case wmOpenChoice:
			p.openChoice(uint16(wp))
			return 0
		case wmDestroy:
			clearPanel(p, hwnd)
		case wmEraseBkgnd:
			p.fill(windows.Handle(wp), p.backgroundBrush)
			return 1
		case wmCtlColorStatic:
			pSetTextColor.Call(wp, uintptr(p.palette.PrimaryText))
			pSetBkMode.Call(wp, transparent)
			return uintptr(p.backgroundBrush)
		case wmCtlColorEdit, wmCtlColorListBox:
			pSetTextColor.Call(wp, uintptr(p.palette.PrimaryText))
			pSetBkColor.Call(wp, uintptr(p.palette.Surface))
			return uintptr(p.surfaceBrush)
		case wmDrawItem:
			if lp != 0 {
				item := drawItemFromLParam(lp)
				if p.staticKinds[uint16(item.CtlID)] != staticNone {
					p.drawStatic(item)
				} else {
					p.drawButton(item)
				}
				return 1
			}
		case wmSettingChange, wmSysColorChange, wmThemeChanged:
			p.refreshTheme(true)
		case wmDpiChanged:
			p.refreshFontsForDPI()
			p.position(p.style, p.exStyle)
			mylog.Info("UI font: surface=popup rebuilt reason=dpi-change dpi=%d face=%q client_px=%dx%d", int(p.metrics.scale*96+0.5), p.fontChoice.Face, p.sc(p.metrics.style.Layout.PanelWidth), p.sc(p.clientH))
			return 0
		case wmTimer:
			if wp == quickMenuTimer {
				p.closeQuickMenu()
				return 0
			}
			if wp == languageMenuTimer {
				p.closeLanguageMenu()
				return 0
			}
			if wp == choiceMenuTimer {
				pKillTimer.Call(uintptr(p.hwnd), choiceMenuTimer)
				if p.choice.openID != 0 && p.choicePointerInside() {
					pSetTimer.Call(uintptr(p.hwnd), choiceMenuTimer, 220, 0)
					return 0
				}
				p.closeChoice(false)
				return 0
			}
		case wmCommand:
			id, notification := uint16(wp), uint16(wp>>16)
			if notification == bnClicked {
				if id == idIdleTimeout || id == idIdleAction {
					p.choice.focusOnOpen = true
					p.openChoice(id)
					return 0
				}
				if _, _, ok := choiceOptionOwner(p, id); ok {
					p.applyChoice(id)
					return 0
				}
				p.handleCommand(id)
				return 0
			}
		}
	}
	result, _, _ := pDefWindowProc.Call(uintptr(hwnd), uintptr(msg), wp, lp)
	return result
}

func (p *panel) refreshFontsForDPI() {
	p.closeChoice(false)
	newScale := dpiForWindow(p.hwnd)
	if p.captureScale > 0 {
		// Screenshot hosts deliberately keep a logical 96-DPI panel even when
		// Windows notifies the app about the monitor's physical DPI.
		newScale = p.captureScale
	}
	if newScale <= 0 || newScale == p.metrics.scale {
		return
	}
	oldMetrics, oldChoice := p.metrics, p.fontChoice
	p.metrics = newPopupMetrics(p.metrics.style, newScale)
	p.fontChoice = uifont.Choice{}
	newFont := p.makeFont(p.metrics.style.Fonts.BodySize, p.metrics.style.Fonts.BodyWeight)
	newSectionFont := p.makeFont(p.metrics.style.Fonts.SectionSize, p.metrics.style.Fonts.SectionWeight)
	newSubtitleFont := p.makeFont(p.metrics.style.Fonts.SubtitleSize, p.metrics.style.Fonts.SubtitleWeight)
	if newFont == 0 || newSectionFont == 0 || newSubtitleFont == 0 {
		for _, font := range []windows.Handle{newFont, newSectionFont, newSubtitleFont} {
			if font != 0 {
				pDeleteObject.Call(uintptr(font))
			}
		}
		p.metrics, p.fontChoice = oldMetrics, oldChoice
		return
	}
	oldFont, oldSection, oldSubtitle := p.font, p.sectionFont, p.subtitleFont
	p.font, p.sectionFont, p.subtitleFont = newFont, newSectionFont, newSubtitleFont
	p.setWindowIcons(p.resolveTheme(), true)
	for id, hwnd := range p.controls {
		if bounds, ok := p.controlBounds[id]; ok {
			pSetWindowPos.Call(uintptr(hwnd), 0, uintptr(p.sc(bounds.x)), uintptr(p.sc(bounds.y)), uintptr(p.sc(bounds.width)), uintptr(p.sc(bounds.height)), 0x0004|0x0010)
		}
		if p.staticKinds[id] == staticNone {
			pSendMessage.Call(uintptr(hwnd), wmSetFont, uintptr(p.font), 1)
		}
	}
	if p.tooltip != 0 {
		pSendMessage.Call(uintptr(p.tooltip), ttmSetMaxTipWidth, 0, uintptr(p.sc(360)))
	}
	for _, font := range []windows.Handle{oldFont, oldSection, oldSubtitle} {
		if font != 0 {
			pDeleteObject.Call(uintptr(font))
		}
	}
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
	switch id {
	case idQuickActions:
		p.openQuickMenu(true)
		return
	case idLanguage:
		p.openLanguageMenu(true)
		return
	case idLangEN:
		if p.selected[idLangEN] {
			return
		}
		p.choose(languageIDs(), idLangEN)
		p.closeLanguageMenu()
		action = ActLanguage
		value = 0
	case idLangZH:
		if p.selected[idLangZH] {
			return
		}
		p.choose(languageIDs(), idLangZH)
		p.closeLanguageMenu()
		action = ActLanguage
		value = 1
	case idSleep:
		action = ActSleep
	case idHibernate:
		action = ActHibernate
	case idShutdown:
		action = ActShutdown
	case idLock:
		action = ActLock
	case idRestart:
		action = ActRestart
	case idNoSleep:
		p.toggle(id)
		if p.toggles[idNoSleep] {
			p.setToggle(idIdle, false)
		}
		p.applyDependentStates()
		action = ActNoSleepToggle
	case idProcess:
		p.toggle(id)
		action = ActProcessWatchToggle
	case idIdle:
		p.toggle(id)
		if p.toggles[idIdle] {
			p.setToggle(idNoSleep, false)
		}
		p.applyDependentStates()
		action = ActIdleToggle
	case idIdleWarning:
		p.toggle(id)
		action = ActIdleWarningToggle
	case idIdleEnhanced:
		p.toggle(id)
		action = ActIdleEnhancedMonitorToggle
	case idTheme:
		p.toggle(id)
		p.applyDependentStates()
		action = ActThemeToggle
	case idBattery:
		p.toggle(id)
		action = ActBatteryToggle
	case idFullscreen:
		p.toggle(id)
		action = ActFullscreenToggle
	case idIPLocation:
		p.toggle(id)
		action = ActIPLocationToggle
	case idThemeSwitch:
		action = ActSwitchTheme
	case idThemeRepair:
		action = ActRepairTheme
	case idHotkeys:
		p.toggle(id)
		action = ActHotkeyToggle
	case idAutostart:
		p.toggle(id)
		action = ActAutostartToggle
	case idLogging:
		p.toggle(id)
		action = ActLoggingToggle
	case idTestWarning:
		Hide()
		idlewarning.SetLanguage(p.isChinese)
		seconds := p.idleWarningSeconds
		if seconds <= 0 {
			seconds = 30
		}
		actionName := p.text(actionTranslationKey(p.idleAction))
		title := p.text("idle_warning_title")
		idlewarning.ShowCountdown(title, seconds, func(remaining int) string {
			if remaining < 0 {
				remaining = 0
			}
			return fmt.Sprintf(p.text("msg_idle_warning"), actionName, remaining)
		})
		return
	case idConfig:
		action = ActConfig
	case idExit:
		action = ActExit
	default:
		return
	}
	if actionClosesPanel(action) {
		Hide()
	}
	if p.onAction != nil {
		p.onAction(action, value)
	}
}

func actionClosesPanel(action Action) bool {
	return action <= ActRestart || action == ActConfig || action == ActExit || action == ActSwitchTheme || action == ActRepairTheme
}

func (p *panel) refreshTheme(invalidate bool) {
	if p.themeRefreshing {
		return
	}
	p.themeRefreshing = true
	defer func() { p.themeRefreshing = false }()
	p.closeChoice(false)
	dark := p.resolveTheme()
	p.setWindowIcons(dark, false)
	p.palette = uicolors.ForTheme(dark)
	p.rebuildBrushes(p.palette)
	p.applyFrameTheme(dark)
	p.applyTooltipTheme(dark)
	if invalidate && p.hwnd != 0 {
		pInvalidateRect.Call(uintptr(p.hwnd), 0, 1)
		for id := range p.controls {
			p.invalidate(id)
		}
	}
}

func (p *panel) applyTooltipTheme(dark bool) {
	if p.tooltip == 0 {
		return
	}
	if dark {
		if name, err := windows.UTF16PtrFromString("DarkMode_Explorer"); err == nil {
			pSetWindowTheme.Call(uintptr(p.tooltip), uintptr(unsafe.Pointer(name)), 0)
		}
	} else {
		pSetWindowTheme.Call(uintptr(p.tooltip), 0, 0)
	}
	pSendMessage.Call(uintptr(p.tooltip), ttmSetTipBkColor, uintptr(p.palette.TooltipBackground), 0)
	pSendMessage.Call(uintptr(p.tooltip), ttmSetTipTextColor, uintptr(p.palette.TooltipText), 0)
}

func (p *panel) applyFrameTheme(dark bool) {
	if p.hwnd == 0 {
		return
	}
	// Win11 uses attribute 20. Windows 10 1809 exposed the same preference as
	// attribute 19; earlier systems ignore both calls and keep their native
	// light/classic caption safely.
	value := uint32(0)
	if dark {
		value = 1
	}
	const (
		dwmwaUseImmersiveDarkMode       = 20
		dwmwaUseImmersiveDarkModeLegacy = 19
	)
	result, _, _ := pDwmSetWindowAttribute.Call(
		uintptr(p.hwnd),
		dwmwaUseImmersiveDarkMode,
		uintptr(unsafe.Pointer(&value)),
		unsafe.Sizeof(value),
	)
	if result != 0 {
		pDwmSetWindowAttribute.Call(
			uintptr(p.hwnd),
			dwmwaUseImmersiveDarkModeLegacy,
			uintptr(unsafe.Pointer(&value)),
			unsafe.Sizeof(value),
		)
	}
}

func (p *panel) drawButton(item *drawItem) {
	id := uint16(item.CtlID)
	state := p.controlState(id, item.ItemState)
	// Choice rows are transiently created after the panel is built. Reassert
	// their semantic selection here so a late owner-draw notification cannot
	// fall back to the generic command-button state.
	if owner, index, ok := choiceOptionOwner(p, id); ok {
		state.Active = p.choice.selected[owner] == index
	}
	if id == idIdleTimeout || id == idIdleAction {
		p.drawChoiceButton(item, state)
		return
	}
	if state.Role == buttonToggle {
		p.drawToggle(item)
		return
	}
	if isMenuTrigger(id) {
		p.drawMenuTrigger(item)
		return
	}
	selected := state.Active
	brush := p.surfaceBrush
	borderColor := p.palette.Border
	textColor := p.palette.PrimaryText
	if state.Hovered {
		brush = p.hoverBrush
	}
	if id == idExit {
		brush = p.dangerBrush
		borderColor = p.palette.DangerBorder
		textColor = p.palette.DangerText
		if state.Hovered {
			brush = p.dangerHoverBrush
			borderColor = p.palette.DangerHoverBorder
		}
	}
	if selected {
		brush = p.selectedBrush
		textColor = p.palette.AccentText
		if state.Hovered {
			brush = p.selectedHoverBrush
		}
	}
	if state.Pressed {
		brush = p.pressedBrush
		textColor = p.palette.AccentText
		if id == idExit {
			brush = p.dangerPressedBrush
			borderColor = p.palette.DangerPressedBorder
			textColor = p.palette.DangerText
		}
	}
	if state.Disabled || item.ItemState&odsDisabled != 0 {
		brush = p.disabledBrush
		borderColor = p.palette.SubtleBorder
		textColor = p.palette.DisabledText
	}
	// RoundRect intentionally leaves its exterior corner pixels untouched.
	// Clear the whole owner-draw item first so the panel, rather than the
	// native BUTTON erase background, remains visible around the radius.
	pFillRect.Call(uintptr(item.HDC), uintptr(unsafe.Pointer(&item.Rect)), uintptr(p.backgroundBrush))
	p.roundRect(item.HDC, item.Rect, brush, borderColor, p.sc(p.metrics.style.Control.CornerRadius))
	// Owner-drawn buttons do not get the native focus cue automatically. Keep
	// a visible inset outline so keyboard navigation remains discoverable in
	// both themes without changing the selected/on color semantics.
	if state.Focused {
		focus := item.Rect
		inset := int32(p.sc(p.metrics.style.Control.FocusInset))
		focus.Left += inset
		focus.Top += inset
		focus.Right -= inset
		focus.Bottom -= inset
		if focus.Left < focus.Right && focus.Top < focus.Bottom {
			focusBrush := p.focusBrush
			if id == idExit {
				focusBrush = p.dangerFocusBrush
			} else if focusOutlineUsesLightOnAccent(selected) {
				// Selected controls use a dedicated pale focus ring rather than their
				// surface color, keeping keyboard focus independent from selection.
				focusBrush = p.focusOnAccentBrush
			}
			pFrameRect.Call(uintptr(item.HDC), uintptr(unsafe.Pointer(&focus)), uintptr(focusBrush))
		}
	}
	pSetTextColor.Call(uintptr(item.HDC), uintptr(textColor))
	pSetBkMode.Call(uintptr(item.HDC), transparent)
	old, _, _ := pSelectObject.Call(uintptr(item.HDC), uintptr(p.font))
	defer pSelectObject.Call(uintptr(item.HDC), old)
	text, _ := windows.UTF16PtrFromString(p.labels[id])
	r := item.Rect
	r.Left += int32(p.sc(p.metrics.style.Control.ButtonTextInset))
	r.Right -= int32(p.sc(p.metrics.style.Control.ButtonTextInset))
	drawTextCentered(item.HDC, text, r)
}

func (p *panel) drawChoiceButton(item *drawItem, state buttonVisualState) {
	brush := p.surfaceBrush
	border := p.palette.Border
	textColor := p.palette.PrimaryText
	arrowColor := p.palette.SecondaryText
	if state.Hovered || p.choice.openID == uint16(item.CtlID) {
		brush = p.hoverBrush
		arrowColor = p.palette.Accent
	}
	if state.Pressed {
		brush = p.pressedBrush
		border = p.palette.AccentPressed
		textColor = p.palette.AccentText
		arrowColor = p.palette.AccentText
	}
	if state.Disabled {
		brush, border = p.disabledBrush, p.palette.SubtleBorder
		textColor, arrowColor = p.palette.DisabledText, p.palette.DisabledText
	}
	pFillRect.Call(uintptr(item.HDC), uintptr(unsafe.Pointer(&item.Rect)), uintptr(p.backgroundBrush))
	p.roundRect(item.HDC, item.Rect, brush, border, p.sc(p.metrics.style.Control.CornerRadius))
	arrowX := item.Rect.Right - int32(p.sc(18))
	pen, _, _ := pCreatePen.Call(psSolid, uintptr(p.sc(1)), uintptr(arrowColor))
	if pen != 0 {
		old, _, _ := pSelectObject.Call(uintptr(item.HDC), pen)
		mid := (item.Rect.Top + item.Rect.Bottom) / 2
		pMoveToEx.Call(uintptr(item.HDC), uintptr(arrowX-4), uintptr(mid-2), 0)
		pLineTo.Call(uintptr(item.HDC), uintptr(arrowX), uintptr(mid+2))
		pLineTo.Call(uintptr(item.HDC), uintptr(arrowX+4), uintptr(mid-2))
		pSelectObject.Call(uintptr(item.HDC), old)
		pDeleteObject.Call(pen)
	}
	text, _ := windows.UTF16PtrFromString(p.labels[uint16(item.CtlID)])
	old, _, _ := pSelectObject.Call(uintptr(item.HDC), uintptr(p.font))
	pSetTextColor.Call(uintptr(item.HDC), uintptr(textColor))
	pSetBkMode.Call(uintptr(item.HDC), transparent)
	r := item.Rect
	r.Left += int32(p.sc(8))
	r.Right = arrowX - int32(p.sc(8))
	pDrawText.Call(uintptr(item.HDC), uintptr(unsafe.Pointer(text)), ^uintptr(0), uintptr(unsafe.Pointer(&r)), dtLeft|dtVCenter|dtSingleLine)
	pSelectObject.Call(uintptr(item.HDC), old)
	if state.Focused {
		focus := item.Rect
		inset := int32(p.sc(p.metrics.style.Control.FocusInset))
		focus.Left += inset
		focus.Top += inset
		focus.Right -= inset
		focus.Bottom -= inset
		if focus.Left < focus.Right && focus.Top < focus.Bottom {
			pFrameRect.Call(uintptr(item.HDC), uintptr(unsafe.Pointer(&focus)), uintptr(p.focusBrush))
		}
	}
}

// drawMenuTrigger keeps hover-only menus visually distinct from commands that
// execute immediately: a quieter rounded card at rest, with accent treatment
// reserved for hover.
func (p *panel) drawMenuTrigger(item *drawItem) {
	id := uint16(item.CtlID)
	state := p.controlState(id, item.ItemState)
	brush := p.elevatedBrush
	borderColor := p.palette.SubtleBorder
	textColor := p.palette.SecondaryText
	hintBrush := p.borderBrush
	if state.Hovered {
		brush = p.hoverBrush
		borderColor = p.palette.Accent
		textColor = p.palette.PrimaryText
		hintBrush = p.accentBrush
	}
	pFillRect.Call(uintptr(item.HDC), uintptr(unsafe.Pointer(&item.Rect)), uintptr(p.backgroundBrush))
	p.roundRect(item.HDC, item.Rect, brush, borderColor, p.sc(p.metrics.style.Control.CornerRadius))
	hint := item.Rect
	hintWidth := int32(p.sc(p.metrics.style.Control.MenuHintWidth))
	hint.Left += (hint.Right - hint.Left - hintWidth) / 2
	hint.Right = hint.Left + hintWidth
	hint.Top = hint.Bottom - int32(p.sc(p.metrics.style.Control.MenuHintHeight+4))
	hint.Bottom = hint.Top + int32(p.sc(p.metrics.style.Control.MenuHintHeight))
	pFillRect.Call(uintptr(item.HDC), uintptr(unsafe.Pointer(&hint)), uintptr(hintBrush))

	if state.Focused {
		focus := item.Rect
		inset := int32(p.sc(p.metrics.style.Control.MenuFocusInset))
		focus.Left += inset
		focus.Top += inset
		focus.Right -= inset
		focus.Bottom -= inset
		if focus.Left < focus.Right && focus.Top < focus.Bottom {
			pFrameRect.Call(uintptr(item.HDC), uintptr(unsafe.Pointer(&focus)), uintptr(p.focusBrush))
		}
	}
	pSetTextColor.Call(uintptr(item.HDC), uintptr(textColor))
	pSetBkMode.Call(uintptr(item.HDC), transparent)
	old, _, _ := pSelectObject.Call(uintptr(item.HDC), uintptr(p.font))
	defer pSelectObject.Call(uintptr(item.HDC), old)
	text, _ := windows.UTF16PtrFromString(p.labels[id])
	bounds := item.Rect
	bounds.Left += int32(p.sc(p.metrics.style.Control.ButtonTextInset))
	bounds.Right -= int32(p.sc(p.metrics.style.Control.ButtonTextInset))
	drawTextCentered(item.HDC, text, bounds)
}

func (p *panel) roundRect(dc windows.Handle, bounds rect, brush windows.Handle, borderColor uint32, radius int) {
	pen, _, _ := pCreatePen.Call(psSolid, 1, uintptr(borderColor))
	if pen == 0 {
		pFillRect.Call(uintptr(dc), uintptr(unsafe.Pointer(&bounds)), uintptr(brush))
		return
	}
	oldBrush, _, _ := pSelectObject.Call(uintptr(dc), uintptr(brush))
	oldPen, _, _ := pSelectObject.Call(uintptr(dc), pen)
	pRoundRect.Call(uintptr(dc), uintptr(bounds.Left), uintptr(bounds.Top), uintptr(bounds.Right), uintptr(bounds.Bottom), uintptr(radius), uintptr(radius))
	pSelectObject.Call(uintptr(dc), oldPen)
	pSelectObject.Call(uintptr(dc), oldBrush)
	pDeleteObject.Call(pen)
}

func (p *panel) drawToggle(item *drawItem) {
	state := p.controlState(uint16(item.CtlID), item.ItemState)
	pFillRect.Call(uintptr(item.HDC), uintptr(unsafe.Pointer(&item.Rect)), uintptr(p.backgroundBrush))
	boxSize := int32(p.sc(p.metrics.style.Control.ToggleBoxSize))
	box := rect{
		Left:   item.Rect.Left + int32(p.sc(p.metrics.style.Control.ToggleLeftInset)),
		Top:    item.Rect.Top + (item.Rect.Bottom-item.Rect.Top-boxSize)/2,
		Right:  item.Rect.Left + int32(p.sc(p.metrics.style.Control.ToggleLeftInset)) + boxSize,
		Bottom: item.Rect.Top + (item.Rect.Bottom-item.Rect.Top-boxSize)/2 + boxSize,
	}
	brush, border, textColor := p.surfaceBrush, p.borderBrush, p.palette.PrimaryText
	if state.Hovered {
		brush = p.hoverBrush
		textColor = p.palette.AccentHover
	}
	if state.Active {
		brush, border = p.accentBrush, p.accentBrush
	}
	if state.Pressed {
		textColor = p.palette.AccentPressed
	}
	if state.Disabled || item.ItemState&odsDisabled != 0 {
		brush, border, textColor = p.disabledBrush, p.subtleBorderBrush, p.palette.DisabledText
	}
	pFillRect.Call(uintptr(item.HDC), uintptr(unsafe.Pointer(&box)), uintptr(brush))
	pFrameRect.Call(uintptr(item.HDC), uintptr(unsafe.Pointer(&box)), uintptr(border))
	old, _, _ := pSelectObject.Call(uintptr(item.HDC), uintptr(p.font))
	defer pSelectObject.Call(uintptr(item.HDC), old)
	pSetBkMode.Call(uintptr(item.HDC), transparent)
	if state.Active {
		check, _ := windows.UTF16PtrFromString("✓")
		checkColor := p.palette.AccentText
		if state.Disabled || item.ItemState&odsDisabled != 0 {
			// White on the light disabled surface has insufficient contrast.
			checkColor = p.palette.MutedText
		}
		pSetTextColor.Call(uintptr(item.HDC), uintptr(checkColor))
		pDrawText.Call(uintptr(item.HDC), uintptr(unsafe.Pointer(check)), ^uintptr(0), uintptr(unsafe.Pointer(&box)), dtCenter|dtVCenter|dtSingleLine)
	}
	if state.Focused {
		focus := item.Rect
		inset := int32(p.sc(p.metrics.style.Control.FocusInset))
		focus.Left += inset
		focus.Top += inset
		focus.Right -= inset
		focus.Bottom -= inset
		if focus.Left < focus.Right && focus.Top < focus.Bottom {
			pFrameRect.Call(uintptr(item.HDC), uintptr(unsafe.Pointer(&focus)), uintptr(p.focusBrush))
		}
	}
	text, _ := windows.UTF16PtrFromString(p.labels[uint16(item.CtlID)])
	bounds := item.Rect
	bounds.Left = box.Right + int32(p.sc(p.metrics.style.Control.ToggleTextGap))
	bounds.Right -= int32(p.sc(p.metrics.style.Control.MenuSurfaceInset))
	pSetTextColor.Call(uintptr(item.HDC), uintptr(textColor))
	drawTextLeftCentered(item.HDC, text, bounds)
}

func (p *panel) drawStatic(item *drawItem) {
	id := uint16(item.CtlID)
	kind := p.staticKinds[id]
	if kind == staticQuickMenu {
		p.roundRect(item.HDC, item.Rect, p.elevatedBrush, p.palette.SubtleBorder, p.sc(p.metrics.style.Control.CornerRadius))
		return
	}
	pFillRect.Call(uintptr(item.HDC), uintptr(unsafe.Pointer(&item.Rect)), uintptr(p.backgroundBrush))
	pSetBkMode.Call(uintptr(item.HDC), transparent)
	text, err := windows.UTF16PtrFromString(p.labels[id])
	if err != nil {
		return
	}
	bounds := item.Rect
	if kind == staticSection {
		accent := bounds
		accent.Right = accent.Left + int32(p.sc(3))
		// Match the rendered title glyph height while keeping the accent centered
		// in its row at every DPI scale.
		accentHeight := int32(p.sc(16))
		accent.Top += (accent.Bottom - accent.Top - accentHeight) / 2
		accent.Bottom = accent.Top + accentHeight
		pFillRect.Call(uintptr(item.HDC), uintptr(unsafe.Pointer(&accent)), uintptr(p.accentBrush))
		bounds.Left += int32(p.sc(10))
		pSetTextColor.Call(uintptr(item.HDC), uintptr(p.palette.PrimaryText))
		old, _, _ := pSelectObject.Call(uintptr(item.HDC), uintptr(p.sectionFont))
		defer pSelectObject.Call(uintptr(item.HDC), old)

		// Measure the translated title so the divider follows its actual width.
		// This keeps Chinese, English, and future locales equally balanced.
		measured := bounds
		pDrawText.Call(uintptr(item.HDC), uintptr(unsafe.Pointer(text)), ^uintptr(0), uintptr(unsafe.Pointer(&measured)), dtLeft|dtVCenter|dtCalcRect)
		separator := bounds
		separator.Left = measured.Right + int32(p.sc(14))
		separator.Top += int32(p.sc(10))
		separator.Bottom = separator.Top + 1
		if separator.Left < separator.Right {
			pFillRect.Call(uintptr(item.HDC), uintptr(unsafe.Pointer(&separator)), uintptr(p.subtleBorderBrush))
		}
	} else {
		pSetTextColor.Call(uintptr(item.HDC), uintptr(p.palette.MutedText))
		old, _, _ := pSelectObject.Call(uintptr(item.HDC), uintptr(p.subtitleFont))
		defer pSelectObject.Call(uintptr(item.HDC), old)
	}
	pDrawText.Call(uintptr(item.HDC), uintptr(unsafe.Pointer(text)), ^uintptr(0), uintptr(unsafe.Pointer(&bounds)), dtLeft|dtVCenter|dtWordBreak)
}

func drawTextCentered(dc windows.Handle, text *uint16, bounds rect) {
	measure := rect{Left: bounds.Left, Top: bounds.Top, Right: bounds.Right, Bottom: bounds.Bottom}
	pDrawText.Call(uintptr(dc), uintptr(unsafe.Pointer(text)), ^uintptr(0), uintptr(unsafe.Pointer(&measure)), dtCenter|dtWordBreak|dtCalcRect)
	textH := measure.Bottom - measure.Top
	if textH < bounds.Bottom-bounds.Top {
		bounds.Top += ((bounds.Bottom - bounds.Top) - textH) / 2
	}
	// bounds.Top already centers the measured text block. Applying DT_VCENTER
	// again would shift single-line labels downward within the reduced bounds.
	pDrawText.Call(uintptr(dc), uintptr(unsafe.Pointer(text)), ^uintptr(0), uintptr(unsafe.Pointer(&bounds)), dtCenter|dtWordBreak)
}

func drawTextLeftCentered(dc windows.Handle, text *uint16, bounds rect) {
	measure := bounds
	pDrawText.Call(uintptr(dc), uintptr(unsafe.Pointer(text)), ^uintptr(0), uintptr(unsafe.Pointer(&measure)), dtLeft|dtWordBreak|dtCalcRect)
	textH := measure.Bottom - measure.Top
	if textH < bounds.Bottom-bounds.Top {
		bounds.Top += ((bounds.Bottom - bounds.Top) - textH) / 2
	}
	pDrawText.Call(uintptr(dc), uintptr(unsafe.Pointer(text)), ^uintptr(0), uintptr(unsafe.Pointer(&bounds)), dtLeft|dtWordBreak)
}

func (p *panel) fill(dc, brush windows.Handle) {
	var r rect
	pGetClientRect.Call(uintptr(p.hwnd), uintptr(unsafe.Pointer(&r)))
	pFillRect.Call(uintptr(dc), uintptr(unsafe.Pointer(&r)), uintptr(brush))
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
	p.closeChoice(false)
	systray.ClearTabNavigationWindow(p.hwnd)
	for hwnd, old := range p.oldButtonProc {
		if hwnd != 0 && old != 0 {
			setWindowProc(hwnd, old)
		}
	}
	p.releaseNativeResources()
	p.hwnd = 0
}
