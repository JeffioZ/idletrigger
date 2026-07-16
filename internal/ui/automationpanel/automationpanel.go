// Package automationpanel implements the native automatic-task manager and
// editor. It uses the existing compact palette and system UI fonts; business
// state is returned to the app through one save callback.
package automationpanel

import (
	"fmt"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/JeffioZ/idletrigger/internal/automation"
	"github.com/JeffioZ/idletrigger/internal/feature/theme"
	"github.com/JeffioZ/idletrigger/internal/platform/windows/processcatalog"
	"github.com/JeffioZ/idletrigger/internal/ui/colors"
	"github.com/JeffioZ/idletrigger/internal/ui/font"
	"github.com/JeffioZ/idletrigger/internal/ui/nativeform"
	"github.com/JeffioZ/idletrigger/internal/ui/processpicker"
	"github.com/JeffioZ/idletrigger/internal/ui/trayicon"
)

type State struct {
	Rules    []automation.Rule
	Issues   []automation.RuleIssue
	Revision string
	Chinese  bool
	Owner    windows.Handle
	NextText string
}

type SaveRequest struct {
	BaseRevision string
	Rules        []automation.Rule
}

type SaveResult struct {
	State State
	Error string
}

type OnSave func(SaveRequest) SaveResult
type TextFunc func(string) string

type rect struct{ Left, Top, Right, Bottom int32 }
type point struct{ X, Y int32 }
type wndClassEx struct {
	Size, Style              uint32
	WndProc                  uintptr
	ClsExtra, WndExtra       int32
	Instance                 windows.Handle
	Icon, Cursor, Background windows.Handle
	MenuName, ClassName      *uint16
	IconSm                   windows.Handle
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
type drawItem struct {
	CtlType, CtlID, ItemID, ItemAction, ItemState uint32
	HwndItem, HDC                                 windows.Handle
	Rect                                          rect
	ItemData                                      uintptr
}

type view uint8

const (
	managerView view = iota
	editorView
)

type panel struct {
	hwnd                windows.Handle
	state               State
	onSave              OnSave
	text                TextFunc
	view                view
	controls            map[uint16]windows.Handle
	anonymous           []windows.Handle
	tooltip             windows.Handle
	tooltipText         [][]uint16
	font                windows.Handle
	sectionFont         windows.Handle
	windowBrush         windows.Handle
	surfaceBrush        windows.Handle
	disabledBrush       windows.Handle
	palette             colors.Palette
	themeDark           bool
	style, exStyle      uintptr
	ownerDisabled       bool
	captureHost         bool
	captureScale        float64
	themeOverride       *bool
	icons               nativeform.WindowIcons
	rules               []automation.Rule
	editing             int
	draft               automation.Rule
	originalDraft       automation.Rule
	processDescriptions map[string]string
	selectedRuleID      string
	listTopIndex        int
	triggerOptions      []automation.Trigger
	labels              map[uint16]string
	bounds              map[uint16]logicalBounds
	fieldSurfaces       map[uint16]uint16
	surfaceFields       map[uint16]uint16
	choices             map[uint16]*choiceField
	checks              map[uint16]bool
	choiceOpen          uint16
	choicePopup         *nativeform.ChoicePopup
	interaction         nativeform.InteractionTracker
	clientWidth         int
	clientHeight        int
	viewportWidth       int
	viewportHeight      int
	contentOffset       int
	dpiScale            float64
	pendingSuggested    *nativeform.Rect
	layoutWorkArea      *nativeform.Rect
	layoutErr           error
	layoutingEditor     bool
	managerReady        bool
	editorReady         bool
	rebuildSuspended    bool
	managerScroll       *nativeform.ListboxScrollbar
	contentScroll       *nativeform.Scrollbar
	nameCue             *nativeform.CueBanner
	pendingState        *State
	managerNotice       string
}

type logicalBounds struct{ X, Y, Width, Height int }

type choiceField struct {
	labels   []string
	selected int
}

const (
	windowClass         = "IdleTriggerAutomationPanel"
	managerWidth        = 600
	managerHeight       = 380
	editorWidth         = 680
	editorHeight        = 520
	idTitle             = 100
	idList              = 102
	idNext              = 103
	idNew               = 104
	idEdit              = 105
	idToggle            = 106
	idDelete            = 107
	idEmptyTitle        = 108
	idEmptyBody         = 109
	idListSurface       = 110
	idName              = 200
	idAction            = 201
	idTrigger           = 202
	idDate              = 203
	idTime              = 204
	idEndTime           = 205
	idProcessLogic      = 207
	idChooseProcesses   = 208
	idProcessSummary    = 209
	idKeepScreen        = 210
	idIdleMinutes       = 211
	idWarningSeconds    = 212
	idBlockedPolicy     = 213
	idMaxWait           = 214
	idSave              = 215
	idCancel            = 216
	idBasicsTitle       = 217
	idTriggerTitle      = 218
	idOptionsTitle      = 219
	idNameLabel         = 220
	idActionLabel       = 221
	idTriggerLabel      = 222
	idDateLabel         = 223
	idTimeLabel         = 224
	idEndTimeLabel      = 225
	idDaysLabel         = 226
	idProcessLogicLabel = 227
	idIdleMinutesLabel  = 228
	idWarningLabel      = 229
	idBlockedLabel      = 230
	idMaxWaitLabel      = 231
	idNoOptions         = 232
	idNameHint          = 233
	idRuntimeNote       = 234
	idValidation        = 235
	idProcessInfo       = 236
	idDaysWorkdays      = 237
	idDaysEveryday      = 238
	idWeekdayBase       = 240
	idFieldSurfaceBase  = 500
	wmDestroy           = 0x0002
	wmPaint             = 0x000F
	wmClose             = 0x0010
	wmEraseBkgnd        = 0x0014
	wmDrawItem          = 0x002B
	wmCommand           = 0x0111
	wmCtlColorStatic    = 0x0138
	wmCtlColorEdit      = 0x0133
	wmCtlColorList      = 0x0134
	wmCtlColorButton    = 0x0135
	wmSettingChange     = 0x001A
	wmSysColorChange    = 0x0015
	wmThemeChanged      = 0x031A
	wmDpiChanged        = 0x02E0
	wmSetFont           = 0x0030
	wmSetRedraw         = 0x000B
	wmLButtonDown       = 0x0201
	wmMouseWheel        = 0x020A
	wmKeyDown           = 0x0100
	wmNCDestroy         = 0x0082
	wmOpenChoice        = 0x8001
	wmPrewarmEditor     = 0x8002
	wsOverlapped        = 0
	wsPopup             = 0x80000000
	wsCaption           = 0x00C00000
	wsSysMenu           = 0x00080000
	wsThickFrame        = 0x00040000
	wsMinimizeBox       = 0x00020000
	wsMaximizeBox       = 0x00010000
	wsClipChildren      = 0x02000000
	wsChild             = 0x40000000
	wsVisible           = 0x10000000
	wsTabStop           = 0x00010000
	wsVScroll           = 0x00200000
	wsExTopmost         = 0x00000008
	wsExComposited      = 0x02000000
	wsExAppWindow       = 0x00040000
	esAutoHScroll       = 0x0080
	lbsNotify           = 0x0001
	lbsNoIntegralHeight = 0x0100
	bsOwnerDraw         = 0x0000000B
	ssLeft              = 0
	ssOwnerDraw         = 0x0000000D
	lbAddString         = 0x0180
	lbResetContent      = 0x0184
	lbGetCurSel         = 0x0188
	lbSetCurSel         = 0x0186
	lbGetTopIndex       = 0x018E
	lbSetTopIndex       = 0x0197
	wmGetTextLength     = 0x000E
	wmGetText           = 0x000D
	emSetMargins        = 0x00D3
	bnClicked           = 0
	lbnDblClk           = 2
	lbnSelChange        = 1
	ttfIDIsHwnd         = 0x0001
	ttfSubclass         = 0x0010
	ttmAddTool          = 0x0432
	ttmDelTool          = 0x0433
	ttmSetMaxTipWidth   = 0x0418
	ttsAlwaysTip        = 0x01
	ttsNoPrefix         = 0x02
	bnSetFocus          = 6
	enSetFocus          = 0x0100
	lbnSetFocus         = 4
	transparent         = 1
	opaque              = 2
	swpNoZOrder         = 0x0004
	swpNoSize           = 0x0001
	swpNoMove           = 0x0002
	swpNoActivate       = 0x0010
	swpShowWindow       = 0x0040
	odsSelected         = 0x0001
	odsDisabled         = 0x0004
	odsFocus            = 0x0010
	vkLeft              = 0x25
	vkRight             = 0x27
	vkHome              = 0x24
	vkEnd               = 0x23
	weekdaySubclassID   = 0x49545744
)

var (
	user32             = windows.NewLazySystemDLL("user32.dll")
	gdi32              = windows.NewLazySystemDLL("gdi32.dll")
	pCreateWindowEx    = user32.NewProc("CreateWindowExW")
	pDestroyWindow     = user32.NewProc("DestroyWindow")
	pDefWindowProc     = user32.NewProc("DefWindowProcW")
	pRegisterClassEx   = user32.NewProc("RegisterClassExW")
	pSendMessage       = user32.NewProc("SendMessageW")
	pSetWindowText     = user32.NewProc("SetWindowTextW")
	pPostMessage       = user32.NewProc("PostMessageW")
	pSetWindowPos      = user32.NewProc("SetWindowPos")
	pShowWindow        = user32.NewProc("ShowWindow")
	pUpdateWindow      = user32.NewProc("UpdateWindow")
	pEnableWindow      = user32.NewProc("EnableWindow")
	pIsWindow          = user32.NewProc("IsWindow")
	pIsWindowEnabled   = user32.NewProc("IsWindowEnabled")
	pIsWindowVisible   = user32.NewProc("IsWindowVisible")
	pSetForeground     = user32.NewProc("SetForegroundWindow")
	pSetFocus          = user32.NewProc("SetFocus")
	pGetClientRect     = user32.NewProc("GetClientRect")
	pFillRect          = user32.NewProc("FillRect")
	pInvalidateRect    = user32.NewProc("InvalidateRect")
	pRedrawWindow      = user32.NewProc("RedrawWindow")
	pGetDpiForWindow   = user32.NewProc("GetDpiForWindow")
	pGetCursorPos      = user32.NewProc("GetCursorPos")
	pSetTextColor      = gdi32.NewProc("SetTextColor")
	pSetBkColor        = gdi32.NewProc("SetBkColor")
	pSetBkMode         = gdi32.NewProc("SetBkMode")
	pCreateBrush       = gdi32.NewProc("CreateSolidBrush")
	pDeleteObject      = gdi32.NewProc("DeleteObject")
	pSetWindowSubclass = windows.NewLazySystemDLL("comctl32.dll").NewProc("SetWindowSubclass")
	pDefSubclassProc   = windows.NewLazySystemDLL("comctl32.dll").NewProc("DefSubclassProc")
	classOnce          sync.Once
	classErr           error
	activeMu           sync.Mutex
	active             *panel
	wndCallback        = windows.NewCallback(wndProc)
	weekdayCallback    = windows.NewCallback(weekdayButtonProc)
)

func Show(state State, onSave OnSave, text TextFunc) error {
	activeMu.Lock()
	if active != nil && active.hwnd != 0 {
		activeMu.Unlock()
		Focus()
		return nil
	}
	activeMu.Unlock()
	if text == nil {
		text = func(key string) string { return key }
	}
	if err := ensureClass(); err != nil {
		return err
	}
	state = cloneState(state)
	p := &panel{state: state, onSave: onSave, text: text, controls: make(map[uint16]windows.Handle), rules: append([]automation.Rule(nil), state.Rules...), editing: -1}
	activeMu.Lock()
	active = p
	activeMu.Unlock()
	if err := p.create(); err != nil {
		activeMu.Lock()
		if active == p {
			active = nil
		}
		activeMu.Unlock()
		return err
	}
	return nil
}

// Focus brings the deepest automatic-task dialog to the foreground. It lets a
// tray click preserve modal ownership instead of closing the disabled parent.
func Focus() bool {
	if processpicker.Focus() {
		return true
	}
	activeMu.Lock()
	p := active
	activeMu.Unlock()
	if p == nil || p.hwnd == 0 {
		return false
	}
	pSetForeground.Call(uintptr(p.hwnd))
	return true
}

func Hide() {
	activeMu.Lock()
	p := active
	activeMu.Unlock()
	if p != nil && p.hwnd != 0 {
		pDestroyWindow.Call(uintptr(p.hwnd))
	}
}

// Update publishes the latest authoritative rule state to an open manager.
// The caller must run on the tray UI thread. Unsaved editor input is preserved
// and marked stale instead of being overwritten.
func Update(state State) {
	activeMu.Lock()
	p := active
	activeMu.Unlock()
	if p == nil || p.hwnd == 0 {
		return
	}
	p.updateState(cloneState(state))
}

func cloneState(state State) State {
	state.Rules = append([]automation.Rule(nil), state.Rules...)
	state.Issues = append([]automation.RuleIssue(nil), state.Issues...)
	return state
}

// Capture hosts the real manager or editor for deterministic devtools visual
// checks without starting the tray or reading user configuration.
func Capture(state State, text TextFunc, scale float64, dark, editor bool, capture func(windows.Handle) error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if text == nil {
		text = func(key string) string { return key }
	}
	if err := ensureClass(); err != nil {
		return err
	}
	state.Owner = 0
	p := &panel{
		state: state, text: text, controls: make(map[uint16]windows.Handle),
		rules: append([]automation.Rule(nil), state.Rules...), editing: -1,
		captureHost: true, captureScale: scale, themeOverride: &dark,
	}
	activeMu.Lock()
	if active != nil {
		activeMu.Unlock()
		return fmt.Errorf("automation panel already active")
	}
	active = p
	activeMu.Unlock()
	if err := p.create(); err != nil {
		activeMu.Lock()
		if active == p {
			active = nil
		}
		activeMu.Unlock()
		return err
	}
	defer func() {
		if p.hwnd != 0 {
			pDestroyWindow.Call(uintptr(p.hwnd))
		}
	}()
	if editor {
		index := -1
		if len(p.rules) > 0 {
			index = 0
		}
		p.showEditor(index)
		// A hidden screenshot host may report the window as not visible during
		// the immediate manager-to-editor transition. Restore the explicit
		// capture state after the transition so PrintWindow never receives an
		// otherwise valid but hidden editor frame.
		pShowWindow.Call(uintptr(p.hwnd), 5)
	}
	if !editor {
		for _, id := range []uint16{idNew, idEdit, idToggle, idDelete} {
			control := p.controls[id]
			if control == 0 {
				return fmt.Errorf("automation capture control %d was not created", id)
			}
			if visible, _, _ := pIsWindowVisible.Call(uintptr(control)); visible == 0 {
				return fmt.Errorf("automation capture control %d is not visible", id)
			}
		}
	}
	if capture == nil {
		return nil
	}
	pInvalidateRect.Call(uintptr(p.hwnd), 0, 0)
	pUpdateWindow.Call(uintptr(p.hwnd))
	for _, control := range p.controls {
		pInvalidateRect.Call(uintptr(control), 0, 0)
		pUpdateWindow.Call(uintptr(control))
	}
	for _, control := range p.anonymous {
		pInvalidateRect.Call(uintptr(control), 0, 0)
		pUpdateWindow.Call(uintptr(control))
	}
	pRedrawWindow.Call(uintptr(p.hwnd), 0, 0, 0x0001|0x0080|0x0100|0x0400)
	return capture(p.hwnd)
}

func ensureClass() error {
	classOnce.Do(func() {
		name, err := windows.UTF16PtrFromString(windowClass)
		if err != nil {
			classErr = err
			return
		}
		wc := wndClassEx{Size: uint32(unsafe.Sizeof(wndClassEx{})), WndProc: wndCallback, ClassName: name}
		result, _, callErr := pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
		if result == 0 && callErr != windows.ERROR_CLASS_ALREADY_EXISTS {
			classErr = fmt.Errorf("register automation panel: %w", callErr)
		}
	})
	return classErr
}

func (p *panel) create() error {
	p.initializeControlState()
	class, _ := windows.UTF16PtrFromString(windowClass)
	title, _ := windows.UTF16PtrFromString(p.t("automation_title"))
	p.style = wsPopup | wsCaption | wsSysMenu | wsClipChildren
	p.exStyle = wsExTopmost
	if p.captureHost {
		p.style = wsOverlapped | wsCaption | wsSysMenu | wsThickFrame | wsMinimizeBox | wsMaximizeBox | wsClipChildren
		p.exStyle = wsExAppWindow | wsExComposited
	}
	hwnd, _, callErr := pCreateWindowEx.Call(p.exStyle, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)), p.style, 0, 0, 1, 1, uintptr(p.state.Owner), 0, 0, 0)
	if hwnd == 0 {
		return fmt.Errorf("create automation panel: %w", callErr)
	}
	p.hwnd = windows.Handle(hwnd)
	p.moveToCursorMonitor()
	p.dpiScale = p.windowScale()
	scale := p.scale()
	p.font, _ = font.New(int32(14*scale+0.5), 400, p.state.Chinese)
	p.sectionFont, _ = font.New(int32(14*scale+0.5), 600, p.state.Chinese)
	p.applyTheme()
	contentScroll, scrollErr := nativeform.NewScrollbar(nativeform.ScrollbarOptions{
		Parent: p.hwnd, Palette: p.palette, Background: p.palette.WindowBackground, Scale: scale,
		OnChange: p.scrollContentTo,
	})
	if scrollErr != nil {
		pDestroyWindow.Call(hwnd)
		return scrollErr
	}
	p.contentScroll = contentScroll
	p.showManager()
	if p.layoutErr != nil {
		err := p.layoutErr
		pDestroyWindow.Call(hwnd)
		return err
	}
	if !p.captureHost && p.state.Owner != 0 {
		if enabled, _, _ := pIsWindowEnabled.Call(uintptr(p.state.Owner)); enabled != 0 {
			pEnableWindow.Call(uintptr(p.state.Owner), 0)
			p.ownerDisabled = true
		}
	}
	pShowWindow.Call(hwnd, 5)
	if p.captureHost {
		// A hidden devtools host can override the first ShowWindow call through
		// STARTUPINFO. The second call uses the explicit state and keeps native
		// child controls visible for PrintWindow.
		pShowWindow.Call(hwnd, 5)
	}
	if !p.captureHost {
		pSetForeground.Call(hwnd)
		trayicon.SetTabNavigationWindow(p.hwnd, nil)
		// Let the manager commit its first frame before constructing the hidden
		// editor controls. The queued prewarm removes the first-New delay without
		// making the manager itself wait for a second page of controls.
		pPostMessage.Call(hwnd, wmPrewarmEditor, 0, 0)
	}
	return nil
}

func (p *panel) clearControls() {
	p.closeChoice(false)
	if p.nameCue != nil {
		p.nameCue.Close()
		p.nameCue = nil
	}
	if p.managerScroll != nil {
		p.managerScroll.Close()
		p.managerScroll = nil
	}
	p.interaction.Release()
	for _, control := range p.controls {
		if control != 0 {
			pDestroyWindow.Call(uintptr(control))
		}
	}
	for _, control := range p.anonymous {
		if control != 0 {
			pDestroyWindow.Call(uintptr(control))
		}
	}
	if p.tooltip != 0 {
		pDestroyWindow.Call(uintptr(p.tooltip))
	}
	p.initializeControlState()
}

func (p *panel) initializeControlState() {
	p.controls = make(map[uint16]windows.Handle)
	p.anonymous = nil
	p.tooltip = 0
	p.tooltipText = nil
	p.labels = make(map[uint16]string)
	p.bounds = make(map[uint16]logicalBounds)
	p.fieldSurfaces = make(map[uint16]uint16)
	p.surfaceFields = make(map[uint16]uint16)
	p.choices = make(map[uint16]*choiceField)
	p.checks = make(map[uint16]bool)
	p.choiceOpen = 0
	p.managerReady = false
	p.editorReady = false
	p.nameCue = nil
}

func (p *panel) updateState(state State) {
	if state.Revision == p.state.Revision {
		p.state.NextText = state.NextText
		p.state.Issues = append([]automation.RuleIssue(nil), state.Issues...)
		if p.view == managerView {
			p.setText(idNext, p.managerStatusText())
			p.populateRules()
		}
		return
	}

	selectedID := p.selectedRuleID
	if p.view == editorView {
		p.syncDraft()
		selectedID = p.draft.ID
		if !reflect.DeepEqual(p.draft, p.originalDraft) {
			pending := cloneState(state)
			p.pendingState = &pending
			p.setEditorError(idSave, p.t("automation_changed_external"))
			return
		}
	}

	p.managerNotice = ""
	p.acceptState(state)
	if p.view == managerView {
		p.showManager()
		return
	}
	for index := range p.rules {
		if p.rules[index].ID == selectedID {
			p.showEditor(index)
			return
		}
	}
	p.showManager()
}

func (p *panel) acceptState(state State) {
	p.state = cloneState(state)
	p.rules = append([]automation.Rule(nil), state.Rules...)
	p.pendingState = nil
}

func (p *panel) issueForRule(index int) (automation.RuleIssue, bool) {
	for _, issue := range p.state.Issues {
		if issue.Index == index {
			return issue, true
		}
	}
	return automation.RuleIssue{}, false
}

func (p *panel) managerStatusText() string {
	if p.managerNotice != "" {
		return p.managerNotice
	}
	return p.state.NextText
}

func (p *panel) showManager() {
	p.beginRebuild()
	defer p.endRebuild()
	p.closeChoice(false)
	p.hideControls(editorControlIDs())
	if p.view != managerView {
		p.contentOffset = 0
	}
	p.view = managerView
	p.setCaption(p.t("automation_title"))
	p.resize(managerWidth, managerHeight)
	if !p.managerReady {
		p.child("STATIC", p.t("automation_rules_title"), wsChild|wsVisible|ssLeft, 18, 16, 564, 24, idTitle, p.sectionFont)
		surface := p.child("STATIC", "", wsChild|wsVisible|ssOwnerDraw, 18, 48, 564, 240, idListSurface, p.font)
		pSetWindowPos.Call(uintptr(surface), 1, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
		list := p.child("LISTBOX", "", wsChild|wsVisible|wsTabStop|wsVScroll|lbsNotify|lbsNoIntegralHeight, 20, 50, 560, 236, idList, p.font)
		if scrollbar, err := nativeform.NewListboxScrollbar(nativeform.ListboxScrollbarOptions{
			Parent: p.hwnd, Listbox: list, Palette: p.palette, Background: p.palette.Surface, Scale: p.scale(),
		}); err == nil {
			p.managerScroll = scrollbar
			p.syncManagerScrollbarBounds()
		}
		p.child("STATIC", p.t("automation_empty_title"), wsChild|ssLeft, 42, 136, 516, 24, idEmptyTitle, p.sectionFont)
		p.child("STATIC", p.t("automation_empty_body"), wsChild|ssLeft, 42, 168, 516, 44, idEmptyBody, p.font)
		p.child("STATIC", p.managerStatusText(), wsChild|wsVisible|ssLeft, 18, 296, 564, 24, idNext, p.font)
		p.child("BUTTON", p.t("automation_new"), wsChild|wsVisible|wsTabStop|bsOwnerDraw, 18, 328, 116, 36, idNew, p.font)
		p.child("BUTTON", p.t("automation_edit"), wsChild|wsVisible|wsTabStop|bsOwnerDraw, 142, 328, 116, 36, idEdit, p.font)
		p.child("BUTTON", p.t("automation_delete"), wsChild|wsVisible|wsTabStop|bsOwnerDraw, 266, 328, 116, 36, idDelete, p.font)
		p.child("BUTTON", p.t("automation_toggle"), wsChild|wsVisible|wsTabStop|bsOwnerDraw, 390, 328, 192, 36, idToggle, p.font)
		pSetWindowPos.Call(uintptr(p.controls[idList]), 0, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
		for id, key := range map[uint16]string{idList: "tip_automation_list", idNew: "tip_automation_new", idEdit: "tip_automation_edit", idToggle: "tip_automation_toggle", idDelete: "tip_automation_delete"} {
			p.addTooltip(id, key)
		}
		p.managerReady = true
	} else {
		p.showControls(managerControlIDs())
		p.setText(idNext, p.managerStatusText())
	}
	if p.managerScroll != nil {
		p.managerScroll.SetActive(true)
	}
	p.populateRules()
}

func (p *panel) syncManagerScrollbarBounds() {
	if p.managerScroll == nil {
		return
	}
	scale := p.scale()
	bounds := p.bounds[idList]
	width := int(float64(nativeform.ScrollbarWidth)*scale + 0.5)
	inset := max(1, int(2*scale+0.5))
	x := int(float64(bounds.X+bounds.Width)*scale+0.5) - width - inset
	y := int(float64(bounds.Y-p.contentOffset)*scale+0.5) + inset
	height := int(float64(bounds.Height)*scale+0.5) - 2*inset
	p.managerScroll.SetBounds(x, y, width, max(1, height))
}

func (p *panel) populateRules() {
	list := p.controls[idList]
	selectedID := p.selectedRuleID
	if index := p.selectedRule(); index >= 0 {
		selectedID = p.rules[index].ID
	}
	if top, _, _ := pSendMessage.Call(uintptr(list), lbGetTopIndex, 0, 0); top != ^uintptr(0) {
		p.listTopIndex = int(top)
	}
	pSendMessage.Call(uintptr(list), lbResetContent, 0, 0)
	selectedIndex := -1
	for index, rule := range p.rules {
		state := p.t("automation_rule_disabled")
		if rule.Enabled {
			state = p.t("automation_rule_enabled")
		}
		summary := p.ruleSummary(rule)
		if issue, invalid := p.issueForRule(index); invalid {
			state = p.t("automation_rule_invalid")
			summary = issue.Message
		}
		label := fmt.Sprintf("%s  %s — %s", state, rule.Name, summary)
		value, _ := windows.UTF16PtrFromString(label)
		pSendMessage.Call(uintptr(list), lbAddString, 0, uintptr(unsafe.Pointer(value)))
		if rule.ID == selectedID {
			selectedIndex = index
		}
	}
	if len(p.rules) == 0 {
		p.show(idList, false)
		p.show(idEmptyTitle, true)
		p.show(idEmptyBody, true)
	} else {
		p.show(idList, true)
		p.show(idEmptyTitle, false)
		p.show(idEmptyBody, false)
		if selectedIndex < 0 {
			selectedIndex = 0
		}
		pSendMessage.Call(uintptr(list), lbSetCurSel, uintptr(selectedIndex), 0)
		p.selectedRuleID = p.rules[selectedIndex].ID
		pSendMessage.Call(uintptr(list), lbSetTopIndex, uintptr(p.listTopIndex), 0)
	}
	p.updateManagerActions()
	if p.managerScroll != nil {
		p.managerScroll.Sync()
	}
}

func (p *panel) showEditor(index int) {
	p.rememberManagerView()
	draft := defaultRule()
	if index >= 0 && index < len(p.rules) {
		draft = p.rules[index]
	}
	p.showEditorDraft(index, draft)
}

func (p *panel) rememberManagerView() {
	if p.view != managerView || p.controls[idList] == 0 {
		return
	}
	if index := p.selectedRule(); index >= 0 {
		p.selectedRuleID = p.rules[index].ID
	}
	if top, _, _ := pSendMessage.Call(uintptr(p.controls[idList]), lbGetTopIndex, 0, 0); top != ^uintptr(0) {
		p.listTopIndex = int(top)
	}
}

func (p *panel) showEditorDraft(index int, draft automation.Rule) {
	p.beginRebuild()
	defer p.endRebuild()
	p.closeChoice(false)
	p.hideControls(managerControlIDs())
	if p.managerScroll != nil {
		p.managerScroll.SetActive(false)
	}
	if p.view != editorView {
		p.contentOffset = 0
	}
	p.view = editorView
	p.editing = index
	p.draft = draft
	p.originalDraft = draft
	p.processDescriptions = make(map[string]string)
	if index >= 0 {
		p.setCaption(p.t("automation_edit_title"))
	} else {
		p.setCaption(p.t("automation_new_title"))
	}
	if !p.editorReady {
		p.createEditorControls()
	}
	p.setText(idName, p.draft.Name)
	p.setText(idDate, p.draft.Date)
	p.setText(idTime, p.draft.Time)
	p.setText(idEndTime, p.draft.EndTime)
	p.setText(idIdleMinutes, strconv.Itoa(p.draft.IdleMinutes))
	p.setText(idWarningSeconds, strconv.Itoa(p.draft.WarningSeconds))
	p.setText(idMaxWait, strconv.Itoa(p.draft.MaxWaitMinutes))
	validation := ""
	if issue, invalid := p.issueForRule(index); invalid {
		validation = issue.Message
	}
	p.setText(idValidation, validation)
	for dayIndex, day := range editorWeekdays {
		p.setChecked(idWeekdayBase+uint16(dayIndex), containsDay(p.draft.Days, day))
	}
	p.setChecked(idKeepScreen, p.draft.KeepScreenOn)
	p.setChoiceOptions(idAction, actionLabels(p.text))
	p.setChoiceOptions(idProcessLogic, processLabels(p.text))
	p.setChoiceOptions(idBlockedPolicy, blockedLabels(p.text))
	p.selectCombo(idAction, actionIndex(p.draft.Action))
	p.setTriggerOptions(p.draft.Action, p.draft.Trigger)
	p.selectCombo(idProcessLogic, processIndex(p.draft.ProcessLogic))
	p.selectCombo(idBlockedPolicy, blockedIndex(p.draft.BlockedPolicy))
	p.layoutEditor()
	p.refreshProcessInfoTooltip()
	p.loadProcessDescriptions()
}

func (p *panel) createEditorControls() {
	p.namedLabel(idBasicsTitle, p.t("automation_basics"), p.sectionFont)
	p.namedLabel(idNameLabel, p.t("automation_name"), p.font)
	p.edit(idName, "")
	if cue, err := nativeform.NewCueBanner(p.controls[idName], p.t("automation_name_placeholder"), p.palette.MutedText, p.scale()); err == nil {
		p.nameCue = cue
	}
	p.namedLabel(idNameHint, p.t("automation_name_hint"), p.font)
	p.namedLabel(idActionLabel, p.t("automation_action"), p.font)
	p.combo(idAction, 0, 0, 314, actionLabels(p.text))
	p.namedLabel(idTriggerLabel, p.t("automation_trigger"), p.font)
	p.combo(idTrigger, 0, 0, 314, nil)
	p.namedLabel(idTriggerTitle, p.t("automation_trigger_conditions"), p.sectionFont)
	p.namedLabel(idDateLabel, p.t("automation_date"), p.font)
	p.edit(idDate, "")
	p.namedLabel(idTimeLabel, p.t("automation_time"), p.font)
	p.edit(idTime, "")
	p.namedLabel(idEndTimeLabel, p.t("automation_end_time"), p.font)
	p.edit(idEndTime, "")
	p.namedLabel(idDaysLabel, p.t("automation_days"), p.font)
	p.child("BUTTON", p.t("automation_days_workdays"), wsChild|wsTabStop|bsOwnerDraw, 0, 0, 1, 1, idDaysWorkdays, p.font)
	p.child("BUTTON", p.t("automation_days_everyday"), wsChild|wsTabStop|bsOwnerDraw, 0, 0, 1, 1, idDaysEveryday, p.font)
	for index, day := range editorWeekdays {
		style := uintptr(wsChild | bsOwnerDraw)
		if index == 0 {
			style |= wsTabStop
		}
		button := p.child("BUTTON", p.t("automation_day_"+day), style, 0, 0, 1, 1, idWeekdayBase+uint16(index), p.font)
		pSetWindowSubclass.Call(uintptr(button), weekdayCallback, weekdaySubclassID, 0)
	}
	p.namedLabel(idProcessLogicLabel, p.t("automation_process_logic"), p.font)
	p.combo(idProcessLogic, 0, 0, 314, processLabels(p.text))
	p.child("BUTTON", p.t("automation_choose_processes"), wsChild|wsTabStop|bsOwnerDraw, 0, 0, 1, 1, idChooseProcesses, p.font)
	p.child("STATIC", "", wsChild|ssLeft, 0, 0, 1, 1, idProcessSummary, p.font)
	p.child("BUTTON", "i", wsChild|wsTabStop|bsOwnerDraw, 0, 0, 1, 1, idProcessInfo, p.font)
	p.namedLabel(idOptionsTitle, p.t("automation_action_options"), p.sectionFont)
	p.child("BUTTON", p.t("automation_keep_screen"), wsChild|wsTabStop|bsOwnerDraw, 0, 0, 1, 1, idKeepScreen, p.font)
	p.namedLabel(idIdleMinutesLabel, p.t("automation_idle_minutes"), p.font)
	p.edit(idIdleMinutes, "")
	p.namedLabel(idWarningLabel, p.t("automation_warning_seconds"), p.font)
	p.edit(idWarningSeconds, "")
	p.namedLabel(idBlockedLabel, p.t("automation_blocked_policy"), p.font)
	p.combo(idBlockedPolicy, 0, 0, 314, blockedLabels(p.text))
	p.namedLabel(idMaxWaitLabel, p.t("automation_max_wait"), p.font)
	p.edit(idMaxWait, "")
	p.namedLabel(idNoOptions, p.t("automation_no_action_options"), p.font)
	p.namedLabel(idRuntimeNote, p.t("automation_runtime_note"), p.font)
	p.namedLabel(idValidation, "", p.font)
	p.child("BUTTON", p.t("common_cancel"), wsChild|wsTabStop|bsOwnerDraw, 0, 0, 1, 1, idCancel, p.font)
	p.child("BUTTON", p.t("common_save"), wsChild|wsTabStop|bsOwnerDraw, 0, 0, 1, 1, idSave, p.font)
	for id, key := range map[uint16]string{idName: "tip_automation_name", idAction: "tip_automation_action", idTrigger: "tip_automation_trigger", idDate: "tip_automation_date", idTime: "tip_automation_time", idEndTime: "tip_automation_end_time", idProcessLogic: "tip_process_logic", idChooseProcesses: "tip_choose_processes", idKeepScreen: "tip_keep_screen", idIdleMinutes: "tip_idle_minutes", idWarningSeconds: "tip_warning_seconds", idBlockedPolicy: "tip_blocked_policy", idMaxWait: "tip_max_wait", idSave: "tip_automation_save", idCancel: "tip_automation_cancel"} {
		p.addTooltip(id, key)
	}
	for index := range editorWeekdays {
		p.addTooltip(idWeekdayBase+uint16(index), "tip_automation_days")
	}
	p.addTooltip(idDaysWorkdays, "tip_automation_days_workdays")
	p.addTooltip(idDaysEveryday, "tip_automation_days_everyday")
	p.editorReady = true
}

func defaultRule() automation.Rule {
	now := time.Now()
	return automation.Rule{ID: fmt.Sprintf("rule-%x", now.UnixNano()), Name: "", Enabled: true, Action: automation.ActionStayAwake, Trigger: automation.TriggerProcessRunning, Date: now.Format("2006-01-02"), Time: now.Add(time.Hour).Format("15:04"), EndTime: now.Add(2 * time.Hour).Format("15:04"), Days: []string{"mon", "tue", "wed", "thu", "fri"}, ProcessLogic: automation.ProcessAny, IdleMinutes: automation.DefaultIdleMinutes, WarningSeconds: automation.DefaultWarningSeconds, BlockedPolicy: automation.BlockedSkip, MaxWaitMinutes: 60}
}

func (p *panel) saveEditor() {
	p.syncDraft()
	if id, message := p.validateDraft(); message != "" {
		p.setEditorError(id, message)
		return
	}
	if p.draft.Name == "" {
		p.draft.Name = p.ruleSummary(p.draft)
	}
	candidate := append([]automation.Rule(nil), p.rules...)
	if p.editing >= 0 && p.editing < len(candidate) {
		candidate[p.editing] = p.draft
	} else {
		candidate = append(candidate, p.draft)
	}
	normalized, issues := automation.PrepareRules(candidate)
	if len(issues) > 0 {
		p.setEditorError(idSave, issues[0].Message)
		return
	}
	p.selectedRuleID = p.draft.ID
	if ok, message := p.notifySave(normalized); !ok {
		p.setEditorError(idSave, message)
		return
	}
	p.showManager()
}

func (p *panel) syncDraft() {
	p.draft.Name = cleanSingleLine(p.controlText(idName))
	p.draft.Action = actionAt(p.comboIndex(idAction))
	p.draft.Trigger = p.triggerValue()
	p.draft.ProcessLogic = processAt(p.comboIndex(idProcessLogic))
	p.draft.BlockedPolicy = blockedAt(p.comboIndex(idBlockedPolicy))
	p.draft.Date = cleanSingleLine(p.controlText(idDate))
	p.draft.Time = cleanSingleLine(p.controlText(idTime))
	p.draft.EndTime = cleanSingleLine(p.controlText(idEndTime))
	p.draft.Days = nil
	for index, day := range editorWeekdays {
		if p.checked(idWeekdayBase + uint16(index)) {
			p.draft.Days = append(p.draft.Days, day)
		}
	}
	p.draft.KeepScreenOn = p.checked(idKeepScreen)
	p.draft.IdleMinutes, _ = strconv.Atoi(cleanSingleLine(p.controlText(idIdleMinutes)))
	p.draft.WarningSeconds, _ = strconv.Atoi(cleanSingleLine(p.controlText(idWarningSeconds)))
	p.draft.MaxWaitMinutes, _ = strconv.Atoi(cleanSingleLine(p.controlText(idMaxWait)))
}

func cleanSingleLine(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}

func (p *panel) setTriggerOptions(action automation.Action, desired automation.Trigger) {
	if automation.IsStateAction(action) {
		p.triggerOptions = []automation.Trigger{automation.TriggerProcessRunning, automation.TriggerTimeWindow}
	} else {
		p.triggerOptions = []automation.Trigger{automation.TriggerOnce, automation.TriggerDaily, automation.TriggerWeekly, automation.TriggerProcessStarted, automation.TriggerProcessExited}
	}
	selected := 0
	labels := make([]string, 0, len(p.triggerOptions))
	for index, trigger := range p.triggerOptions {
		labels = append(labels, p.t(triggerKey(trigger)))
		if trigger == desired {
			selected = index
		}
	}
	p.setChoiceOptions(idTrigger, labels)
	p.selectCombo(idTrigger, selected)
}

func (p *panel) triggerValue() automation.Trigger {
	index := p.comboIndex(idTrigger)
	if index < 0 || index >= len(p.triggerOptions) {
		return p.triggerOptions[0]
	}
	return p.triggerOptions[index]
}

func (p *panel) layoutEditor() {
	if p.layoutingEditor {
		return
	}
	p.layoutingEditor = true
	defer func() { p.layoutingEditor = false }()
	suggested := p.pendingSuggested
	p.pendingSuggested = nil
	layoutWidth := min(editorWidth, max(1, p.viewportWidth))
	for range 3 {
		height := p.layoutEditorContent(layoutWidth)
		p.resize(editorWidth, height)
		nextWidth := min(editorWidth, max(1, p.viewportWidth))
		if nextWidth == layoutWidth || p.layoutErr != nil {
			break
		}
		layoutWidth = nextWidth
	}
	if suggested != nil && p.layoutErr == nil {
		p.pendingSuggested = suggested
		p.resize(editorWidth, p.clientHeight)
	}
	p.setText(idProcessSummary, p.processSummary())
}

func (p *panel) layoutEditorContent(layoutWidth int) int {
	p.closeChoice(false)
	action := actionAt(p.comboIndex(idAction))
	trigger := p.triggerValue()
	const pad, gap, fieldH, labelH = nativeform.FormPadding, nativeform.ControlGap, nativeform.FieldHeight, 18
	reserve := 0
	if p.clientHeight > p.viewportHeight {
		reserve = nativeform.ScrollbarWidth + 4
	}
	contentW := max(1, layoutWidth-2*pad-reserve)
	columnW := (contentW - gap) / 2
	for _, id := range editorControlIDs() {
		p.show(id, false)
	}
	p.place(idBasicsTitle, pad, 16, contentW, 24, true)
	p.place(idNameLabel, pad, 48, contentW, labelH, true)
	p.place(idName, pad, 70, contentW, fieldH, true)
	p.place(idNameHint, pad, 108, contentW, 20, true)
	p.place(idActionLabel, pad, 144, columnW, labelH, true)
	p.place(idTriggerLabel, pad+columnW+gap, 144, columnW, labelH, true)
	p.placeCombo(idAction, pad, 166, columnW, true)
	p.placeCombo(idTrigger, pad+columnW+gap, 166, columnW, true)
	p.place(idTriggerTitle, pad, 216, contentW, 24, true)
	y := 248
	rowFields := func(leftLabel, leftControl, rightLabel, rightControl uint16) {
		p.place(leftLabel, pad, y, columnW, labelH, true)
		p.place(rightLabel, pad+columnW+gap, y, columnW, labelH, true)
		p.place(leftControl, pad, y+22, columnW, fieldH, true)
		p.place(rightControl, pad+columnW+gap, y+22, columnW, fieldH, true)
		y += 64
	}
	switch trigger {
	case automation.TriggerOnce:
		rowFields(idDateLabel, idDate, idTimeLabel, idTime)
	case automation.TriggerDaily:
		p.place(idTimeLabel, pad, y, columnW, labelH, true)
		p.place(idTime, pad, y+22, columnW, fieldH, true)
		y += 64
	case automation.TriggerWeekly:
		p.place(idTimeLabel, pad, y, columnW, labelH, true)
		p.place(idTime, pad, y+22, columnW, fieldH, true)
		y += 64
		y = p.layoutWeekdays(y, contentW)
	case automation.TriggerTimeWindow:
		rowFields(idTimeLabel, idTime, idEndTimeLabel, idEndTime)
		y = p.layoutWeekdays(y, contentW)
	}

	processRequired := trigger == automation.TriggerProcessRunning || trigger == automation.TriggerProcessStarted || trigger == automation.TriggerProcessExited
	p.setText(idProcessLogicLabel, p.t("automation_optional_process"))
	if processRequired {
		p.setText(idProcessLogicLabel, p.t("automation_process_condition"))
	}
	p.place(idProcessLogicLabel, pad, y, contentW, labelH, true)
	y += 22
	if trigger == automation.TriggerProcessExited {
		p.draft.ProcessLogic = automation.ProcessNone
		p.selectCombo(idProcessLogic, processIndex(automation.ProcessNone))
		p.place(idChooseProcesses, pad, y, contentW, fieldH, true)
	} else if trigger == automation.TriggerProcessStarted {
		p.draft.ProcessLogic = automation.ProcessAny
		p.selectCombo(idProcessLogic, processIndex(automation.ProcessAny))
		p.place(idChooseProcesses, pad, y, contentW, fieldH, true)
	} else {
		p.placeCombo(idProcessLogic, pad, y, columnW, true)
		p.place(idChooseProcesses, pad+columnW+gap, y, columnW, fieldH, true)
	}
	y += fieldH + gap
	p.place(idProcessSummary, pad, y, contentW, 22, true)
	showInfo := len(p.draft.Processes) > 0
	if showInfo {
		p.place(idProcessSummary, pad, y, contentW-30, 22, true)
	}
	p.place(idProcessInfo, pad+contentW-22, y, 22, 22, showInfo)
	y += 30

	p.place(idOptionsTitle, pad, y, contentW, 24, true)
	y += 32
	switch action {
	case automation.ActionStayAwake:
		p.place(idKeepScreen, pad, y, contentW, fieldH, true)
		y += fieldH + gap
	case automation.ActionEnableIdle:
		p.place(idIdleMinutesLabel, pad, y, columnW, labelH, true)
		p.place(idIdleMinutes, pad, y+22, columnW, fieldH, true)
		y += 64
	case automation.ActionPauseStayAwake, automation.ActionPauseIdle:
		p.place(idNoOptions, pad, y, contentW, 24, true)
		y += 32
	default:
		p.place(idWarningLabel, pad, y, columnW, labelH, true)
		p.place(idWarningSeconds, pad, y+22, columnW, fieldH, true)
		y += 64
		if len(p.draft.Processes) > 0 && trigger != automation.TriggerProcessStarted && trigger != automation.TriggerProcessExited {
			p.place(idBlockedLabel, pad, y, columnW, labelH, true)
			p.placeCombo(idBlockedPolicy, pad, y+22, columnW, true)
			if blockedAt(p.comboIndex(idBlockedPolicy)) == automation.BlockedWait {
				p.place(idMaxWaitLabel, pad+columnW+gap, y, columnW, labelH, true)
				p.place(idMaxWait, pad+columnW+gap, y+22, columnW, fieldH, true)
			}
			y += 64
		}
	}
	runtimeHeight := 22
	p.place(idRuntimeNote, pad, y, contentW, runtimeHeight, true)
	y += runtimeHeight + gap
	if strings.TrimSpace(p.controlText(idValidation)) != "" {
		p.place(idValidation, pad, y, contentW, 36, true)
		y += 42
	}
	p.place(idCancel, pad+contentW-220, y, 102, nativeform.ButtonHeight, true)
	p.place(idSave, pad+contentW-110, y, 110, nativeform.ButtonHeight, true)
	y += 52
	return y
}

func (p *panel) layoutWeekdays(y, contentW int) int {
	const pad, gap = nativeform.FormPadding, nativeform.ControlGap
	buttonW := (contentW - gap*(len(editorWeekdays)-1)) / len(editorWeekdays)
	const quickW, quickH = 88, 24
	p.place(idDaysLabel, pad, y+3, contentW-2*(quickW+gap), 18, true)
	p.place(idDaysWorkdays, pad+contentW-2*quickW-gap, y, quickW, quickH, true)
	p.place(idDaysEveryday, pad+contentW-quickW, y, quickW, quickH, true)
	y += 30
	for index := range editorWeekdays {
		p.place(idWeekdayBase+uint16(index), pad+index*(buttonW+gap), y, buttonW, nativeform.FieldHeight, true)
	}
	return y + nativeform.FieldHeight + gap
}

func (p *panel) handleCommand(id, notification uint16) {
	if notification == bnSetFocus || notification == enSetFocus || notification == lbnSetFocus {
		p.ensureControlVisible(id)
	}
	if p.view == managerView {
		p.handleManager(id, notification)
	} else {
		p.handleEditor(id, notification)
	}
}
func (p *panel) handleManager(id, notification uint16) {
	switch id {
	case idList:
		switch notification {
		case lbnDblClk:
			p.editSelected()
		case lbnSelChange:
			if index := p.selectedRule(); index >= 0 {
				p.selectedRuleID = p.rules[index].ID
			}
			p.updateManagerActions()
		}
	case idNew:
		p.showEditor(-1)
	case idEdit:
		p.editSelected()
	case idToggle:
		if index := p.selectedRule(); index >= 0 {
			candidate := append([]automation.Rule(nil), p.rules...)
			candidate[index].Enabled = !candidate[index].Enabled
			if ok, message := p.notifySave(candidate); !ok {
				p.setText(idNext, message)
			}
			p.populateRules()
		}
	case idDelete:
		if index := p.selectedRule(); index >= 0 {
			if !p.confirm(p.t("automation_delete_title"), fmt.Sprintf(p.t("automation_delete_confirm"), p.rules[index].Name)) {
				return
			}
			candidate := append([]automation.Rule(nil), p.rules...)
			candidate = append(candidate[:index], candidate[index+1:]...)
			p.selectedRuleID = ""
			if ok, message := p.notifySave(candidate); !ok {
				p.setText(idNext, message)
			}
			p.populateRules()
		}
	}
}
func (p *panel) handleEditor(id, notification uint16) {
	if _, ok := p.choices[id]; ok {
		if notification == bnClicked {
			// Open after the native BUTTON finishes its click processing. Opening
			// synchronously from BN_CLICKED lets the button restore focus and close
			// the popup immediately on some Windows versions.
			pPostMessage.Call(uintptr(p.hwnd), wmOpenChoice, uintptr(id), 0)
		}
		// Owner-drawn buttons also send focus notifications. When the popup
		// takes focus, treating BN_KILLFOCUS as a regular command would close
		// the popup that has just opened.
		return
	}
	if id >= idWeekdayBase && id < idWeekdayBase+uint16(len(editorWeekdays)) && notification == bnClicked {
		hadValidation := strings.TrimSpace(p.controlText(idValidation)) != ""
		p.setChecked(id, !p.checked(id))
		p.setText(idValidation, "")
		if hadValidation {
			p.relayoutEditor()
		}
		return
	}
	p.closeChoice(false)
	switch id {
	case idDaysWorkdays:
		if notification == bnClicked {
			p.selectWeekdays(false)
		}
	case idDaysEveryday:
		if notification == bnClicked {
			p.selectWeekdays(true)
		}
	case idKeepScreen:
		if notification == bnClicked {
			p.setChecked(id, !p.checked(id))
		}
	case idChooseProcesses:
		p.syncDraft()
		_ = processpicker.Show(processpicker.Options{Owner: p.hwnd, Selected: p.draft.Processes, Descriptions: p.processDescriptions, Chinese: p.state.Chinese, Text: p.text, OnConfirm: func(targets []automation.ProcessTarget, descriptions map[string]string) {
			p.draft.Processes = targets
			p.processDescriptions = descriptions
			p.setText(idProcessSummary, p.processSummary())
			p.refreshProcessInfoTooltip()
			p.setText(idValidation, "")
			p.relayoutEditor()
		}})
	case idProcessInfo:
		if notification == bnClicked && len(p.draft.Processes) > 0 {
			p.showProcessDetails()
		}
	case idSave:
		p.saveEditor()
	case idCancel:
		p.cancelEditor()
	}
}

func (p *panel) selectWeekdays(everyDay bool) {
	hadValidation := strings.TrimSpace(p.controlText(idValidation)) != ""
	for index := range editorWeekdays {
		p.setChecked(idWeekdayBase+uint16(index), everyDay || index < 5)
	}
	p.setText(idValidation, "")
	if hadValidation {
		p.relayoutEditor()
	}
}

func (p *panel) handleChoiceChanged(id uint16) {
	p.syncDraft()
	if id == idAction {
		p.setTriggerOptions(p.draft.Action, p.draft.Trigger)
	}
	p.setText(idValidation, "")
	p.relayoutEditor()
}

func (p *panel) relayoutEditor() {
	p.beginRebuild()
	p.layoutEditor()
	p.endRebuild()
}

func (p *panel) editSelected() {
	if index := p.selectedRule(); index >= 0 {
		p.selectedRuleID = p.rules[index].ID
		p.showEditor(index)
	}
}

func (p *panel) updateManagerActions() {
	index := p.selectedRule()
	enabled := index >= 0
	for _, id := range []uint16{idEdit, idDelete, idToggle} {
		p.enable(id, enabled)
	}
	label := p.t("automation_toggle")
	if enabled {
		if p.rules[index].Enabled {
			label = p.t("automation_disable")
		} else {
			label = p.t("automation_enable")
		}
	}
	p.setText(idToggle, label)
}

func (p *panel) cancelEditor() {
	p.syncDraft()
	if !reflect.DeepEqual(p.draft, p.originalDraft) && !p.confirm(p.t("automation_discard_title"), p.t("automation_discard_confirm")) {
		return
	}
	if p.pendingState != nil {
		p.acceptState(*p.pendingState)
	}
	p.showManager()
}
func (p *panel) selectedRule() int {
	value, _, _ := pSendMessage.Call(uintptr(p.controls[idList]), lbGetCurSel, 0, 0)
	if int(value) < 0 || int(value) >= len(p.rules) || value == ^uintptr(0) {
		return -1
	}
	return int(value)
}
func (p *panel) notifySave(rules []automation.Rule) (bool, string) {
	if p.onSave == nil {
		p.rules = append([]automation.Rule(nil), rules...)
		p.state.Rules = append([]automation.Rule(nil), rules...)
		p.state.Issues = nil
		return true, ""
	}
	result := p.onSave(SaveRequest{BaseRevision: p.state.Revision, Rules: append([]automation.Rule(nil), rules...)})
	if result.Error != "" {
		if p.view == managerView && (result.State.Revision != "" || result.State.Rules != nil) {
			p.acceptState(result.State)
			p.managerNotice = result.Error
		} else if result.State.Revision != "" && result.State.Revision != p.state.Revision {
			pending := cloneState(result.State)
			p.pendingState = &pending
		}
		return false, result.Error
	}
	p.managerNotice = ""
	p.acceptState(result.State)
	return true, ""
}
func (p *panel) processSummary() string {
	if len(p.draft.Processes) == 0 {
		return p.t("automation_no_processes")
	}
	return fmt.Sprintf(p.t("automation_process_count"), len(p.draft.Processes))
}

func (p *panel) processDetails() string {
	targets := automation.NormalizeTargets(p.draft.Processes)
	lines := make([]string, 0, len(targets))
	for _, target := range targets {
		name := target.Executable
		if description := p.processDescriptions[target.Key()]; description != "" {
			name = fmt.Sprintf(p.t("process_name_description"), name, description)
		}
		if target.Match == automation.MatchPath {
			lines = append(lines, fmt.Sprintf(p.t("automation_process_detail_path"), name, target.Path))
		} else {
			lines = append(lines, fmt.Sprintf(p.t("automation_process_detail_name"), name))
		}
	}
	return strings.Join(lines, "\n")
}

func (p *panel) refreshProcessInfoTooltip() {
	if p.controls[idProcessInfo] == 0 || len(p.draft.Processes) == 0 {
		return
	}
	p.addTooltipValue(idProcessInfo, p.processDetails())
}

func (p *panel) showProcessDetails() {
	title, _ := windows.UTF16PtrFromString(p.t("automation_process_details_title"))
	body, _ := windows.UTF16PtrFromString(p.processDetails())
	const mbOKInformation = 0x00000000 | 0x00000040
	user32.NewProc("MessageBoxW").Call(uintptr(p.hwnd), uintptr(unsafe.Pointer(body)), uintptr(unsafe.Pointer(title)), mbOKInformation)
}

func (p *panel) loadProcessDescriptions() {
	targets := automation.NormalizeTargets(p.draft.Processes)
	if len(targets) == 0 {
		return
	}
	ruleID := p.draft.ID
	go func() {
		descriptions := make(map[string]string, len(targets))
		names := make(map[string]struct{})
		for _, target := range targets {
			if target.Match == automation.MatchPath {
				if description := processcatalog.FileDescription(target.Path); description != "" {
					descriptions[target.Key()] = description
				}
			} else {
				names[strings.ToLower(target.Executable)] = struct{}{}
			}
		}
		if len(names) > 0 {
			if instances, err := processcatalog.SnapshotNames(); err == nil {
				filtered := instances[:0]
				for _, instance := range instances {
					if _, wanted := names[strings.ToLower(instance.Executable)]; wanted {
						filtered = append(filtered, instance)
					}
				}
				for _, group := range processcatalog.GroupInstances(processcatalog.EnrichDescriptions(filtered)) {
					if group.Description != "" {
						target := automation.ProcessTarget{Match: automation.MatchName, Executable: group.Executable}
						descriptions[target.Key()] = group.Description
					}
				}
			}
		}
		if len(descriptions) == 0 {
			return
		}
		trayicon.Post(func() {
			if p.hwnd == 0 || p.view != editorView || p.draft.ID != ruleID {
				return
			}
			for key, value := range descriptions {
				p.processDescriptions[key] = value
			}
			p.refreshProcessInfoTooltip()
		})
	}()
}
func (p *panel) ruleSummary(rule automation.Rule) string {
	action := p.t(actionKey(rule.Action))
	switch rule.Trigger {
	case automation.TriggerProcessRunning:
		return fmt.Sprintf(p.t("automation_summary_process_running"), action, len(rule.Processes))
	case automation.TriggerProcessStarted:
		return fmt.Sprintf(p.t("automation_summary_process_started"), action, len(rule.Processes))
	case automation.TriggerProcessExited:
		return fmt.Sprintf(p.t("automation_summary_process_exited"), action, len(rule.Processes))
	case automation.TriggerTimeWindow:
		return fmt.Sprintf(p.t("automation_summary_time_window"), action, rule.Time, rule.EndTime)
	case automation.TriggerOnce:
		return fmt.Sprintf(p.t("automation_summary_once"), action, rule.Date, rule.Time)
	case automation.TriggerDaily:
		return fmt.Sprintf(p.t("automation_summary_daily"), action, rule.Time)
	case automation.TriggerWeekly:
		return fmt.Sprintf(p.t("automation_summary_weekly"), action, rule.Time)
	}
	return action
}

func (p *panel) namedLabel(id uint16, value string, useFont windows.Handle) {
	p.child("STATIC", value, wsChild|ssLeft, 0, 0, 1, 1, id, useFont)
}
func (p *panel) edit(id uint16, value string) {
	surfaceID := idFieldSurfaceBase + id
	p.child("STATIC", "", wsChild|ssOwnerDraw, 0, 0, 1, 1, surfaceID, p.font)
	hwnd := p.child("EDIT", value, wsChild|wsTabStop|esAutoHScroll, 0, 0, 1, 1, id, p.font)
	p.fieldSurfaces[id] = surfaceID
	p.surfaceFields[surfaceID] = id
	p.interaction.Track(hwnd, p.controls[surfaceID])
	margin := int(6*p.scale() + 0.5)
	pSendMessage.Call(uintptr(hwnd), emSetMargins, 3, uintptr(margin|(margin<<16)))
}
func (p *panel) combo(id uint16, x, y, width int, labels []string) {
	p.child("BUTTON", "", wsChild|wsTabStop|bsOwnerDraw, x, y, width, nativeform.FieldHeight, id, p.font)
	p.setChoiceOptions(id, labels)
}
func (p *panel) place(id uint16, x, y, width, height int, visible bool) {
	hwnd := p.controls[id]
	if hwnd == 0 {
		return
	}
	p.bounds[id] = logicalBounds{X: x, Y: y, Width: width, Height: height}
	if surfaceID, ok := p.fieldSurfaces[id]; ok {
		p.positionControl(p.controls[surfaceID], x, y, width, height)
		innerHeight := min(20, height-4)
		p.positionControl(hwnd, x+2, y+(height-innerHeight)/2, width-4, innerHeight)
		p.show(surfaceID, visible)
		p.show(id, visible)
		if visible {
			pSetWindowPos.Call(uintptr(hwnd), 0, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
		}
		return
	}
	p.positionControl(hwnd, x, y, width, height)
	p.show(id, visible)
}
func (p *panel) positionControl(hwnd windows.Handle, x, y, width, height int) {
	if hwnd == 0 {
		return
	}
	scale := p.scale()
	pSetWindowPos.Call(uintptr(hwnd), 0,
		uintptr(int(float64(x)*scale)), uintptr(int(float64(y-p.contentOffset)*scale)),
		uintptr(int(float64(width)*scale)), uintptr(int(float64(height)*scale)),
		swpNoZOrder|swpNoActivate)
}
func (p *panel) placeCombo(id uint16, x, y, width int, visible bool) {
	p.place(id, x, y, width, nativeform.FieldHeight, visible)
}
func (p *panel) child(className, value string, style uintptr, x, y, width, height int, id uint16, useFont windows.Handle) windows.Handle {
	class, _ := windows.UTF16PtrFromString(className)
	caption, _ := windows.UTF16PtrFromString(value)
	scale := p.scale()
	hwnd, _, _ := pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(caption)), style, uintptr(int(float64(x)*scale)), uintptr(int(float64(y)*scale)), uintptr(int(float64(width)*scale)), uintptr(int(float64(height)*scale)), uintptr(p.hwnd), uintptr(id), 0, 0)
	if hwnd != 0 && useFont != 0 {
		pSendMessage.Call(hwnd, wmSetFont, uintptr(useFont), 1)
	}
	if hwnd != 0 {
		nativeform.ApplyControl(windows.Handle(hwnd), p.themeDark)
	}
	if id != 0 {
		p.controls[id] = windows.Handle(hwnd)
		p.labels[id] = value
		p.bounds[id] = logicalBounds{X: x, Y: y, Width: width, Height: height}
		if strings.EqualFold(className, "BUTTON") && style&bsOwnerDraw != 0 {
			p.interaction.Track(windows.Handle(hwnd), windows.Handle(hwnd))
		}
	} else if hwnd != 0 {
		p.anonymous = append(p.anonymous, windows.Handle(hwnd))
	}
	return windows.Handle(hwnd)
}
func (p *panel) addTooltip(id uint16, key string) {
	p.addTooltipValue(id, p.t(key))
}

func (p *panel) addTooltipValue(id uint16, value string) {
	control := p.controls[id]
	if control == 0 {
		return
	}
	if p.tooltip == 0 {
		class, _ := windows.UTF16PtrFromString("tooltips_class32")
		tip, _, _ := pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(class)), 0, wsPopup|ttsAlwaysTip|ttsNoPrefix, 0, 0, 0, 0, uintptr(p.hwnd), 0, 0, 0)
		if tip == 0 {
			return
		}
		p.tooltip = windows.Handle(tip)
		pSendMessage.Call(tip, ttmSetMaxTipWidth, 0, uintptr(int(360*p.scale())))
		nativeform.ApplyTooltip(p.tooltip, p.themeDark, p.palette)
	}
	text, _ := windows.UTF16FromString(value)
	p.tooltipText = append(p.tooltipText, text)
	info := toolInfo{Size: uint32(unsafe.Sizeof(toolInfo{})), Flags: ttfIDIsHwnd | ttfSubclass, Hwnd: p.hwnd, ID: uintptr(control), Text: &p.tooltipText[len(p.tooltipText)-1][0]}
	pSendMessage.Call(uintptr(p.tooltip), ttmDelTool, 0, uintptr(unsafe.Pointer(&info)))
	pSendMessage.Call(uintptr(p.tooltip), ttmAddTool, 0, uintptr(unsafe.Pointer(&info)))
}
func (p *panel) setText(id uint16, value string) {
	if p.controls[id] == 0 {
		return
	}
	text, _ := windows.UTF16PtrFromString(value)
	pSetWindowText.Call(uintptr(p.controls[id]), uintptr(unsafe.Pointer(text)))
	p.labels[id] = value
	pInvalidateRect.Call(uintptr(p.controls[id]), 0, 0)
}
func (p *panel) setCaption(value string) {
	text, _ := windows.UTF16PtrFromString(value)
	pSetWindowText.Call(uintptr(p.hwnd), uintptr(unsafe.Pointer(text)))
}
func (p *panel) show(id uint16, visible bool) {
	command := uintptr(0)
	if visible {
		command = 5
	}
	pShowWindow.Call(uintptr(p.controls[id]), command)
	if surfaceID, ok := p.fieldSurfaces[id]; ok {
		pShowWindow.Call(uintptr(p.controls[surfaceID]), command)
	}
}

func (p *panel) hideControls(ids []uint16) {
	for _, id := range ids {
		p.show(id, false)
	}
}

func (p *panel) showControls(ids []uint16) {
	for _, id := range ids {
		p.show(id, true)
	}
}

func (p *panel) controlText(id uint16) string {
	hwnd := p.controls[id]
	length, _, _ := pSendMessage.Call(uintptr(hwnd), wmGetTextLength, 0, 0)
	buf := make([]uint16, int(length)+1)
	if len(buf) > 0 {
		pSendMessage.Call(uintptr(hwnd), wmGetText, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])))
	}
	return windows.UTF16ToString(buf)
}
func (p *panel) selectCombo(id uint16, index int) {
	choice := p.choices[id]
	if choice == nil || len(choice.labels) == 0 {
		return
	}
	if index < 0 || index >= len(choice.labels) {
		index = 0
	}
	choice.selected = index
	p.setText(id, choice.labels[index])
}
func (p *panel) comboIndex(id uint16) int {
	choice := p.choices[id]
	if choice == nil || choice.selected < 0 || choice.selected >= len(choice.labels) {
		return 0
	}
	return choice.selected
}
func (p *panel) setChecked(id uint16, value bool) {
	p.checks[id] = value
	if p.controls[id] != 0 {
		pInvalidateRect.Call(uintptr(p.controls[id]), 0, 0)
	}
}
func (p *panel) checked(id uint16) bool {
	return p.checks[id]
}
func (p *panel) enable(id uint16, value bool) {
	flag := uintptr(0)
	if value {
		flag = 1
	}
	pEnableWindow.Call(uintptr(p.controls[id]), flag)
	p.invalidateControl(id)
	if surfaceID, ok := p.fieldSurfaces[id]; ok {
		pInvalidateRect.Call(uintptr(p.controls[surfaceID]), 0, 0)
	}
}
func (p *panel) t(key string) string { return p.text(key) }
func (p *panel) scale() float64 {
	if p.captureScale > 0 {
		return p.captureScale
	}
	if p.dpiScale > 0 {
		return p.dpiScale
	}
	return p.windowScale()
}
func (p *panel) windowScale() float64 {
	if p.hwnd == 0 {
		return 1
	}
	dpi, _, _ := pGetDpiForWindow.Call(uintptr(p.hwnd))
	if dpi == 0 {
		return 1
	}
	return float64(dpi) / 96
}
func (p *panel) resize(width, height int) {
	p.resizeInWorkArea(width, height, p.layoutWorkArea)
}

func (p *panel) resizeInWorkArea(width, height int, workArea *nativeform.Rect) {
	previousViewportWidth := p.viewportWidth
	previousWorkArea := p.layoutWorkArea
	p.layoutWorkArea = workArea
	defer func() { p.layoutWorkArea = previousWorkArea }()
	p.clientWidth, p.clientHeight = width, height
	scale := p.scale()
	anchor := p.hwnd
	if p.state.Owner != 0 {
		anchor = p.state.Owner
	}
	suggested := p.pendingSuggested
	p.pendingSuggested = nil
	_, err := nativeform.PlaceWindow(nativeform.WindowPlacement{
		Window: p.hwnd, Anchor: anchor, Owner: p.state.Owner,
		Style: p.style, ExStyle: p.exStyle,
		ClientWidth: int(float64(width)*scale + 0.5), ClientHeight: int(float64(height)*scale + 0.5),
		DPI: uint32(scale*96 + 0.5), Suggested: suggested, WorkArea: workArea,
	})
	if err != nil {
		p.layoutErr = err
		return
	}
	p.layoutErr = nil
	p.syncContentViewport()
	if p.view == editorView && p.editorReady && !p.layoutingEditor && p.viewportWidth != previousViewportWidth {
		p.layoutEditor()
	}
}

func (p *panel) moveToCursorMonitor() {
	var cursor point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&cursor)))
	pSetWindowPos.Call(uintptr(p.hwnd), 0, uintptr(cursor.X), uintptr(cursor.Y), 1, 1, 0x0010)
}

func (p *panel) syncContentViewport() {
	if p.hwnd == 0 {
		return
	}
	physicalWidth, physicalHeight, err := nativeform.ClientSize(p.hwnd)
	if err != nil {
		p.layoutErr = err
		return
	}
	scale := p.scale()
	p.viewportWidth = max(1, int(float64(physicalWidth)/scale))
	p.viewportHeight = max(1, int(float64(physicalHeight)/scale))
	maximum := max(0, p.clientHeight-p.viewportHeight)
	p.contentOffset = max(0, min(p.contentOffset, maximum))
	if p.contentScroll != nil {
		p.contentScroll.SetScale(scale)
		barWidth := max(1, int(float64(nativeform.ScrollbarWidth)*scale+0.5))
		inset := max(1, int(2*scale+0.5))
		p.contentScroll.SetBounds(physicalWidth-barWidth-inset, inset, barWidth, max(1, physicalHeight-2*inset))
		p.contentScroll.SetMetrics(max(1, p.clientHeight), max(1, p.viewportHeight), p.contentOffset)
	}
	p.repositionContent()
}

func (p *panel) scrollContentTo(position int) {
	maximum := max(0, p.clientHeight-p.viewportHeight)
	position = max(0, min(position, maximum))
	if position == p.contentOffset {
		return
	}
	p.closeChoice(false)
	p.contentOffset = position
	p.repositionContent()
	if p.contentScroll != nil {
		p.contentScroll.SetMetrics(max(1, p.clientHeight), max(1, p.viewportHeight), p.contentOffset)
	}
}

func (p *panel) repositionContent() {
	for id, bounds := range p.bounds {
		if _, fieldSurface := p.surfaceFields[id]; fieldSurface {
			continue
		}
		hwnd := p.controls[id]
		if hwnd == 0 {
			continue
		}
		if surfaceID, field := p.fieldSurfaces[id]; field {
			p.positionControl(p.controls[surfaceID], bounds.X, bounds.Y, bounds.Width, bounds.Height)
			innerHeight := min(20, bounds.Height-4)
			p.positionControl(hwnd, bounds.X+2, bounds.Y+(bounds.Height-innerHeight)/2, bounds.Width-4, innerHeight)
			continue
		}
		p.positionControl(hwnd, bounds.X, bounds.Y, bounds.Width, bounds.Height)
	}
	p.syncManagerScrollbarBounds()
}

func (p *panel) ensureControlVisible(id uint16) {
	bounds, ok := p.bounds[id]
	if !ok || p.clientHeight <= p.viewportHeight {
		return
	}
	top, bottom := bounds.Y, bounds.Y+bounds.Height
	position := p.contentOffset
	if top < position {
		position = top
	} else if bottom > position+p.viewportHeight {
		position = bottom - p.viewportHeight
	}
	p.scrollContentTo(position)
}

func (p *panel) scrollWheel(wParam uintptr) bool {
	if p.clientHeight <= p.viewportHeight {
		return false
	}
	delta := int16(wParam >> 16)
	if delta > 0 {
		p.scrollContentTo(p.contentOffset - 48)
	} else if delta < 0 {
		p.scrollContentTo(p.contentOffset + 48)
	}
	return delta != 0
}

func (p *panel) rebuildForDPI() {
	view, editing, draft, contentOffset := p.view, p.editing, p.draft, p.contentOffset
	if view == editorView {
		p.syncDraft()
		draft = p.draft
	}
	p.clearControls()
	if p.font != 0 {
		pDeleteObject.Call(uintptr(p.font))
	}
	if p.sectionFont != 0 {
		pDeleteObject.Call(uintptr(p.sectionFont))
	}
	scale := p.scale()
	p.font, _ = font.New(int32(14*scale+0.5), 400, p.state.Chinese)
	p.sectionFont, _ = font.New(int32(14*scale+0.5), 600, p.state.Chinese)
	if view == editorView {
		p.showEditorDraft(editing, draft)
	} else {
		p.showManager()
	}
	p.scrollContentTo(contentOffset)
}
func (p *panel) applyTheme() {
	p.themeDark = theme.Current() == theme.ModeDark
	if p.themeOverride != nil {
		p.themeDark = *p.themeOverride
	}
	p.palette = colors.ForTheme(p.themeDark)
	p.releaseBrushes()
	p.windowBrush = makeBrush(p.palette.WindowBackground)
	p.surfaceBrush = makeBrush(p.palette.Surface)
	p.disabledBrush = makeBrush(p.palette.DisabledSurface)
	nativeform.ApplyFrame(p.hwnd, p.themeDark)
	scale := p.scale()
	p.icons.Apply(p.hwnd, p.themeDark, int(32*scale+0.5), int(16*scale+0.5), false)
	for _, control := range p.controls {
		nativeform.ApplyControl(control, p.themeDark)
		pInvalidateRect.Call(uintptr(control), 0, 0)
	}
	for _, control := range p.anonymous {
		nativeform.ApplyControl(control, p.themeDark)
		pInvalidateRect.Call(uintptr(control), 0, 0)
	}
	nativeform.ApplyTooltip(p.tooltip, p.themeDark, p.palette)
	if p.managerScroll != nil {
		p.managerScroll.SetTheme(p.palette, p.palette.Surface)
		p.managerScroll.Sync()
	}
	if p.contentScroll != nil {
		p.contentScroll.SetTheme(p.palette, p.palette.WindowBackground)
	}
	if p.nameCue != nil {
		p.nameCue.SetTheme(p.palette.MutedText)
	}
	if p.hwnd != 0 {
		pInvalidateRect.Call(uintptr(p.hwnd), 0, 0)
	}
}

func (p *panel) drawOwnerItem(value *drawItem) bool {
	return p.drawStyledOwnerItem(value)
}

func makeBrush(color uint32) windows.Handle {
	brush, _, _ := pCreateBrush.Call(uintptr(color))
	return windows.Handle(brush)
}

func (p *panel) releaseBrushes() {
	for _, brush := range []windows.Handle{p.windowBrush, p.surfaceBrush, p.disabledBrush} {
		if brush != 0 {
			pDeleteObject.Call(uintptr(brush))
		}
	}
	p.windowBrush, p.surfaceBrush, p.disabledBrush = 0, 0, 0
}

func weekdayButtonProc(hwnd windows.Handle, message uint32, wParam, lParam uintptr, subclassID, refData uintptr) uintptr {
	if message == wmKeyDown && (wParam == vkLeft || wParam == vkRight || wParam == vkHome || wParam == vkEnd) {
		activeMu.Lock()
		p := active
		activeMu.Unlock()
		if p != nil && p.view == editorView {
			id := p.controlID(hwnd)
			if id >= idWeekdayBase && id < idWeekdayBase+uint16(len(editorWeekdays)) {
				index := int(id - idWeekdayBase)
				switch wParam {
				case vkLeft:
					index = (index + len(editorWeekdays) - 1) % len(editorWeekdays)
				case vkRight:
					index = (index + 1) % len(editorWeekdays)
				case vkHome:
					index = 0
				case vkEnd:
					index = len(editorWeekdays) - 1
				}
				pSetFocus.Call(uintptr(p.controls[idWeekdayBase+uint16(index)]))
				return 0
			}
		}
	}
	result, _, _ := pDefSubclassProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return result
}

func (p *panel) confirm(title, body string) bool {
	t, _ := windows.UTF16PtrFromString(title)
	b, _ := windows.UTF16PtrFromString(body)
	const mbYesNoWarningDefaultNo = 0x00000004 | 0x00000030 | 0x00000100
	result, _, _ := user32.NewProc("MessageBoxW").Call(uintptr(p.hwnd), uintptr(unsafe.Pointer(b)), uintptr(unsafe.Pointer(t)), mbYesNoWarningDefaultNo)
	return result == 6
}

func wndProc(hwnd windows.Handle, message uint32, wParam, lParam uintptr) uintptr {
	activeMu.Lock()
	p := active
	activeMu.Unlock()
	if p == nil || p.hwnd != hwnd {
		result, _, _ := pDefWindowProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
		return result
	}
	switch message {
	case wmClose:
		p.closeChoice(false)
		if p.view == editorView {
			p.cancelEditor()
		} else {
			pDestroyWindow.Call(uintptr(hwnd))
		}
		return 0
	case wmLButtonDown:
		p.closeChoice(false)
	case wmMouseWheel:
		if p.scrollWheel(wParam) {
			return 0
		}
	case wmOpenChoice:
		if p.view == editorView {
			p.toggleChoice(uint16(wParam))
		}
		return 0
	case wmPrewarmEditor:
		if p.view == managerView && !p.editorReady {
			p.createEditorControls()
		}
		return 0
	case wmCommand:
		p.handleCommand(uint16(wParam), uint16(wParam>>16))
		return 0
	case wmDrawItem:
		if p.drawOwnerItem((*drawItem)(nativeform.MessagePointer(lParam))) {
			return 1
		}
	case wmPaint:
		nativeform.PaintWindowBackground(hwnd, p.windowBrush)
		return 0
	case wmEraseBkgnd:
		var bounds rect
		pGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&bounds)))
		pFillRect.Call(wParam, uintptr(unsafe.Pointer(&bounds)), uintptr(p.windowBrush))
		return 1
	case wmCtlColorStatic:
		textColor := p.palette.PrimaryText
		controlID := p.controlID(windows.Handle(lParam))
		if isSecondaryLabel(controlID) {
			textColor = p.palette.SecondaryText
		} else if isMutedLabel(controlID) {
			textColor = p.palette.MutedText
		}
		if controlID == idValidation {
			if p.themeDark {
				textColor = p.palette.DangerBorder
			} else {
				textColor = p.palette.DangerBackground
			}
		}
		brush := p.windowBrush
		backgroundColor := p.palette.WindowBackground
		if controlID == idEmptyTitle || controlID == idEmptyBody {
			brush = p.surfaceBrush
			backgroundColor = p.palette.Surface
		}
		pSetTextColor.Call(wParam, uintptr(textColor))
		pSetBkColor.Call(wParam, uintptr(backgroundColor))
		pSetBkMode.Call(wParam, opaque)
		return uintptr(brush)
	case wmCtlColorButton:
		pSetTextColor.Call(wParam, uintptr(p.palette.PrimaryText))
		pSetBkMode.Call(wParam, transparent)
		return uintptr(p.windowBrush)
	case wmCtlColorEdit, wmCtlColorList:
		brush := p.surfaceBrush
		textColor := p.palette.PrimaryText
		backgroundColor := p.palette.Surface
		if enabled, _, _ := pIsWindowEnabled.Call(lParam); enabled == 0 {
			brush = p.disabledBrush
			textColor = p.palette.DisabledText
			backgroundColor = p.palette.DisabledSurface
		}
		pSetTextColor.Call(wParam, uintptr(textColor))
		pSetBkColor.Call(wParam, uintptr(backgroundColor))
		return uintptr(brush)
	case wmSettingChange, wmSysColorChange, wmThemeChanged:
		p.applyTheme()
		return 0
	case wmDpiChanged:
		if p.font != 0 {
			dpi := uint32(wParam & 0xffff)
			if dpi == 0 {
				dpi = 96
			}
			p.dpiScale = float64(dpi) / 96
			if lParam != 0 {
				suggested := nativeform.Rect(*(*rect)(nativeform.MessagePointer(lParam)))
				p.pendingSuggested = &suggested
			}
			p.rebuildForDPI()
			scale := p.scale()
			p.icons.Apply(p.hwnd, p.themeDark, int(32*scale+0.5), int(16*scale+0.5), true)
		}
		return 0
	case wmDestroy:
		processpicker.Hide()
		if p.contentScroll != nil {
			p.contentScroll.Close()
			p.contentScroll = nil
		}
		if p.nameCue != nil {
			p.nameCue.Close()
			p.nameCue = nil
		}
		if p.managerScroll != nil {
			p.managerScroll.Close()
			p.managerScroll = nil
		}
		trayicon.ClearTabNavigationWindow(hwnd)
		if p.font != 0 {
			pDeleteObject.Call(uintptr(p.font))
		}
		if p.sectionFont != 0 {
			pDeleteObject.Call(uintptr(p.sectionFont))
		}
		p.releaseBrushes()
		p.icons.Release()
		if p.ownerDisabled && p.state.Owner != 0 {
			if valid, _, _ := pIsWindow.Call(uintptr(p.state.Owner)); valid != 0 {
				pEnableWindow.Call(uintptr(p.state.Owner), 1)
				pSetForeground.Call(uintptr(p.state.Owner))
			}
			p.ownerDisabled = false
		}
		activeMu.Lock()
		if active == p {
			active = nil
		}
		activeMu.Unlock()
		p.hwnd = 0
		return 0
	}
	result, _, _ := pDefWindowProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return result
}

func (p *panel) controlID(hwnd windows.Handle) uint16 {
	for id, control := range p.controls {
		if control == hwnd {
			return id
		}
	}
	return 0
}

func isSecondaryLabel(id uint16) bool {
	switch id {
	case idNameLabel, idActionLabel, idTriggerLabel, idDateLabel, idTimeLabel, idEndTimeLabel, idDaysLabel,
		idProcessLogicLabel, idIdleMinutesLabel, idWarningLabel, idBlockedLabel, idMaxWaitLabel,
		idProcessSummary, idNext, idEmptyBody:
		return true
	default:
		return false
	}
}

func isMutedLabel(id uint16) bool {
	switch id {
	case idNameHint, idRuntimeNote, idNoOptions:
		return true
	default:
		return false
	}
}

var actions = []automation.Action{automation.ActionStayAwake, automation.ActionPauseStayAwake, automation.ActionEnableIdle, automation.ActionPauseIdle, automation.ActionLock, automation.ActionSleep, automation.ActionHibernate, automation.ActionShutdown, automation.ActionRestart}
var processModes = []automation.ProcessLogic{automation.ProcessAny, automation.ProcessAll, automation.ProcessNone}
var blockedModes = []automation.BlockedPolicy{automation.BlockedSkip, automation.BlockedWait}
var editorWeekdays = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

func managerControlIDs() []uint16 {
	return []uint16{idTitle, idListSurface, idList, idEmptyTitle, idEmptyBody, idNext, idNew, idEdit, idDelete, idToggle}
}

func editorControlIDs() []uint16 {
	ids := []uint16{
		idBasicsTitle, idNameLabel, idName, idNameHint, idActionLabel, idAction, idTriggerLabel, idTrigger,
		idTriggerTitle, idDateLabel, idDate, idTimeLabel, idTime, idEndTimeLabel, idEndTime, idDaysLabel, idDaysWorkdays, idDaysEveryday,
		idProcessLogicLabel, idProcessLogic, idChooseProcesses, idProcessSummary, idProcessInfo, idOptionsTitle, idKeepScreen,
		idIdleMinutesLabel, idIdleMinutes, idWarningLabel, idWarningSeconds, idBlockedLabel, idBlockedPolicy,
		idMaxWaitLabel, idMaxWait, idNoOptions, idRuntimeNote, idValidation, idCancel, idSave,
	}
	for index := range editorWeekdays {
		ids = append(ids, idWeekdayBase+uint16(index))
	}
	return ids
}

func containsDay(days []string, wanted string) bool {
	for _, day := range days {
		if strings.EqualFold(strings.TrimSpace(day), wanted) {
			return true
		}
	}
	return false
}

func triggerKey(trigger automation.Trigger) string {
	switch trigger {
	case automation.TriggerProcessRunning:
		return "automation_trigger_process_running"
	case automation.TriggerProcessStarted:
		return "automation_trigger_process_started"
	case automation.TriggerProcessExited:
		return "automation_trigger_process_exited"
	case automation.TriggerTimeWindow:
		return "automation_trigger_time_window"
	case automation.TriggerOnce:
		return "automation_trigger_once"
	case automation.TriggerDaily:
		return "automation_trigger_daily"
	case automation.TriggerWeekly:
		return "automation_trigger_weekly"
	default:
		return "automation_trigger"
	}
}

func (p *panel) validateDraft() (uint16, string) {
	trigger := p.draft.Trigger
	if (trigger == automation.TriggerProcessRunning || trigger == automation.TriggerProcessStarted || trigger == automation.TriggerProcessExited) && len(p.draft.Processes) == 0 {
		return idChooseProcesses, p.t("automation_error_process_required")
	}
	if trigger == automation.TriggerOnce {
		if _, err := time.Parse("2006-01-02", p.draft.Date); err != nil {
			return idDate, p.t("automation_error_date")
		}
	}
	if trigger == automation.TriggerOnce || trigger == automation.TriggerDaily || trigger == automation.TriggerWeekly || trigger == automation.TriggerTimeWindow {
		if _, err := time.Parse("15:04", p.draft.Time); err != nil {
			return idTime, p.t("automation_error_time")
		}
	}
	if trigger == automation.TriggerTimeWindow {
		if _, err := time.Parse("15:04", p.draft.EndTime); err != nil {
			return idEndTime, p.t("automation_error_end_time")
		}
	}
	if (trigger == automation.TriggerWeekly || trigger == automation.TriggerTimeWindow) && len(p.draft.Days) == 0 {
		return idWeekdayBase, p.t("automation_error_days")
	}
	if p.draft.Action == automation.ActionEnableIdle && (p.draft.IdleMinutes <= 0 || p.draft.IdleMinutes > 7*24*60) {
		return idIdleMinutes, p.t("automation_error_idle_minutes")
	}
	if automation.IsEventAction(p.draft.Action) && (p.draft.WarningSeconds < automation.MinWarningSeconds || p.draft.WarningSeconds > 3600) {
		return idWarningSeconds, p.t("automation_error_warning")
	}
	if automation.IsEventAction(p.draft.Action) && len(p.draft.Processes) > 0 && trigger != automation.TriggerProcessStarted && trigger != automation.TriggerProcessExited && p.draft.BlockedPolicy == automation.BlockedWait && (p.draft.MaxWaitMinutes <= 0 || p.draft.MaxWaitMinutes > 7*24*60) {
		return idMaxWait, p.t("automation_error_max_wait")
	}
	return 0, ""
}

func (p *panel) setEditorError(id uint16, message string) {
	p.setText(idValidation, message)
	p.relayoutEditor()
	if control := p.controls[id]; control != 0 {
		pSetFocus.Call(uintptr(control))
	}
}

func actionAt(i int) automation.Action {
	if i < 0 || i >= len(actions) {
		return actions[0]
	}
	return actions[i]
}
func actionIndex(v automation.Action) int {
	for i, x := range actions {
		if x == v {
			return i
		}
	}
	return 0
}
func processAt(i int) automation.ProcessLogic {
	if i < 0 || i >= len(processModes) {
		return processModes[0]
	}
	return processModes[i]
}
func processIndex(v automation.ProcessLogic) int {
	for i, x := range processModes {
		if x == v {
			return i
		}
	}
	return 0
}
func blockedAt(i int) automation.BlockedPolicy {
	if i < 0 || i >= len(blockedModes) {
		return blockedModes[0]
	}
	return blockedModes[i]
}
func blockedIndex(v automation.BlockedPolicy) int {
	for i, x := range blockedModes {
		if x == v {
			return i
		}
	}
	return 0
}
func actionLabels(t TextFunc) []string {
	out := make([]string, len(actions))
	for i, v := range actions {
		out[i] = t(actionKey(v))
	}
	return out
}
func processLabels(t TextFunc) []string {
	return []string{t("automation_process_any"), t("automation_process_all"), t("automation_process_none")}
}
func blockedLabels(t TextFunc) []string {
	return []string{t("automation_blocked_skip"), t("automation_blocked_wait")}
}
func actionKey(v automation.Action) string {
	switch v {
	case automation.ActionStayAwake:
		return "automation_action_stay_awake"
	case automation.ActionPauseStayAwake:
		return "automation_action_pause_stay_awake"
	case automation.ActionEnableIdle:
		return "automation_action_enable_idle"
	case automation.ActionPauseIdle:
		return "automation_action_pause_idle"
	case automation.ActionLock:
		return "menu_lock"
	case automation.ActionSleep:
		return "menu_sleep"
	case automation.ActionHibernate:
		return "menu_hibernate"
	case automation.ActionShutdown:
		return "menu_shutdown"
	case automation.ActionRestart:
		return "menu_restart"
	}
	return "menu_more"
}
