// Package processpicker implements the bounded native process-selection
// window used by automatic tasks. PIDs are display-time metadata only and are
// never returned to callers.
package processpicker

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/JeffioZ/idletrigger/internal/automation"
	"github.com/JeffioZ/idletrigger/internal/feature/theme"
	"github.com/JeffioZ/idletrigger/internal/platform/windows/processcatalog"
	"github.com/JeffioZ/idletrigger/internal/ui/colors"
	"github.com/JeffioZ/idletrigger/internal/ui/font"
	"github.com/JeffioZ/idletrigger/internal/ui/nativeform"
	"github.com/JeffioZ/idletrigger/internal/ui/trayicon"
)

type Options struct {
	Owner        windows.Handle
	Selected     []automation.ProcessTarget
	Descriptions map[string]string
	Chinese      bool
	Text         func(string) string
	OnConfirm    func([]automation.ProcessTarget, map[string]string)
}

type rect struct{ Left, Top, Right, Bottom int32 }
type logicalBounds struct{ X, Y, Width, Height int }
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

type item struct {
	target                   automation.ProcessTarget
	name, description, count string
	search                   string
}

type initCommonControlsEx struct {
	Size uint32
	ICC  uint32
}

type lvColumn struct {
	Mask    uint32
	Fmt     int32
	Cx      int32
	Text    *uint16
	TextMax int32
	SubItem int32
	Image   int32
	Order   int32
}

type lvItem struct {
	Mask      uint32
	Item      int32
	SubItem   int32
	State     uint32
	StateMask uint32
	Text      *uint16
	TextMax   int32
	Image     int32
	LParam    uintptr
}

type nmHeader struct {
	HwndFrom windows.Handle
	IDFrom   uintptr
	Code     int32
}

type nmListView struct {
	Header             nmHeader
	Item, SubItem      int32
	NewState, OldState uint32
	Changed            uint32
	Action             struct{ X, Y int32 }
	LParam             uintptr
}

type nmLVGetInfoTip struct {
	Header  nmHeader
	Flags   uint32
	Text    *uint16
	TextMax int32
	Item    int32
	SubItem int32
	LParam  uintptr
}

type lvHitTestInfo struct {
	Point         struct{ X, Y int32 }
	Flags         uint32
	Item, SubItem int32
	Group         int32
}

type openFileName struct {
	Size             uint32
	Owner            windows.Handle
	Instance         windows.Handle
	Filter           *uint16
	CustomFilter     *uint16
	MaxCustomFilter  uint32
	FilterIndex      uint32
	File             *uint16
	MaxFile          uint32
	FileTitle        *uint16
	MaxFileTitle     uint32
	InitialDirectory *uint16
	Title            *uint16
	Flags            uint32
	FileOffset       uint16
	FileExtension    uint16
	DefaultExtension *uint16
	CustomData       uintptr
	Hook             uintptr
	TemplateName     *uint16
	Reserved         unsafe.Pointer
	ReservedSize     uint32
	FlagsEx          uint32
}

type picker struct {
	hwnd             windows.Handle
	options          Options
	controls         map[uint16]windows.Handle
	bounds           map[uint16]logicalBounds
	labels           map[uint16]string
	surfaceFields    map[uint16]uint16
	font             windows.Handle
	windowBrush      windows.Handle
	surfaceBrush     windows.Handle
	palette          colors.Palette
	themeDark        bool
	icons            nativeform.WindowIcons
	style            uintptr
	exStyle          uintptr
	tooltip          windows.Handle
	tooltipText      [][]uint16
	items            []item
	visible          []item
	selected         map[string]automation.ProcessTarget
	descriptions     map[string]string
	descriptionTried map[string]struct{}
	sortColumn       int
	sortAscending    bool
	populating       bool
	captureHost      bool
	captureScale     float64
	themeOverride    *bool
	generation       uint64
	loading          bool
	enriching        bool
	closed           atomic.Bool
	cleanupOnce      sync.Once
	workers          sync.WaitGroup
	interaction      nativeform.InteractionTracker
	stateImages      windows.Handle
	header           windows.Handle
	headerHover      int
	headerPressed    int
	scrollbar        *nativeform.Scrollbar
	previewScroll    *nativeform.ListboxScrollbar
	contentScroll    *nativeform.Scrollbar
	searchCue        *nativeform.CueBanner
	lastSnapshot     time.Time
	lastScanDuration time.Duration
	anchorTopKey     string
	anchorFocusKey   string
	ownerDisabled    bool
	createErr        error
	layoutErr        error
	dpiScale         float64
	contentOffset    int
	viewportWidth    int
	viewportHeight   int
	pendingSuggested *nativeform.Rect
}

const (
	windowClass           = "IdleTriggerProcessPicker"
	windowWidth           = 700
	windowHeight          = 510
	idSearch              = 101
	idRefresh             = 102
	idList                = 103
	idStatus              = 104
	idPrivacy             = 105
	idCancel              = 106
	idConfirm             = 107
	idPreviewTitle        = 108
	idPreview             = 109
	idEmpty               = 110
	idBrowse              = 111
	idHeading             = 112
	idHelper              = 113
	idSearchSurface       = 114
	idListSurface         = 115
	idPreviewSurface      = 116
	wmDestroy             = 0x0002
	wmActivate            = 0x0006
	wmPaint               = 0x000F
	wmClose               = 0x0010
	wmEraseBkgnd          = 0x0014
	wmDrawItem            = 0x002B
	wmCommand             = 0x0111
	wmNotify              = 0x004E
	wmTimer               = 0x0113
	wmMouseWheel          = 0x020A
	wmSetRedraw           = 0x000B
	wmCtlColorStatic      = 0x0138
	wmCtlColorEdit        = 0x0133
	wmCtlColorList        = 0x0134
	wmSettingChange       = 0x001A
	wmSysColorChange      = 0x0015
	wmThemeChanged        = 0x031A
	wmDpiChanged          = 0x02E0
	wmSetFont             = 0x0030
	wsOverlapped          = 0x00000000
	wsPopup               = 0x80000000
	wsCaption             = 0x00C00000
	wsSysMenu             = 0x00080000
	wsThickFrame          = 0x00040000
	wsMinimizeBox         = 0x00020000
	wsMaximizeBox         = 0x00010000
	wsClipChildren        = 0x02000000
	wsClipSiblings        = 0x04000000
	wsChild               = 0x40000000
	wsVisible             = 0x10000000
	wsTabStop             = 0x00010000
	wsVScroll             = 0x00200000
	wsExTopmost           = 0x00000008
	wsExAppWindow         = 0x00040000
	wsExComposited        = 0x02000000
	esAutoHScroll         = 0x0080
	lbsNoSelection        = 0x4000
	lbsNoIntegralHeight   = 0x0100
	lvsReport             = 0x0001
	lvsSingleSel          = 0x0004
	lvsShowSelAlways      = 0x0008
	bsOwnerDraw           = 0x0000000B
	ssLeft                = 0
	ssOwnerDraw           = 0x0000000D
	lbAddString           = 0x0180
	lbResetContent        = 0x0184
	lvmFirst              = 0x1000
	lvmSetBackgroundColor = lvmFirst + 1
	lvmSetImageList       = lvmFirst + 3
	lvmDeleteItem         = lvmFirst + 8
	lvmDeleteAllItems     = lvmFirst + 9
	lvmGetHeader          = lvmFirst + 31
	lvmGetColumnWidth     = lvmFirst + 29
	lvmGetTopIndex        = lvmFirst + 39
	lvmGetCountPerPage    = lvmFirst + 40
	lvmGetItemRect        = lvmFirst + 14
	lvmScroll             = lvmFirst + 20
	lvmGetItemState       = lvmFirst + 44
	lvmSetItemState       = lvmFirst + 43
	lvmEnsureVisible      = lvmFirst + 19
	lvmSubItemHitTest     = lvmFirst + 57
	lvmInsertItemW        = lvmFirst + 77
	lvmGetStringWidthW    = lvmFirst + 87
	lvmSetColumnW         = lvmFirst + 96
	lvmSetItemTextW       = lvmFirst + 116
	lvmInsertColumnW      = lvmFirst + 97
	lvmSetColumnWidth     = lvmFirst + 30
	lvmSetExtendedStyle   = lvmFirst + 54
	lvmSetTextColor       = lvmFirst + 36
	lvmSetTextBackground  = lvmFirst + 38
	lvsExCheckboxes       = 0x00000004
	lvsExFullRowSelect    = 0x00000020
	lvsExInfoTip          = 0x00000400
	lvsExDoubleBuffer     = 0x00010000
	lvifText              = 0x00000001
	lvifState             = 0x00000008
	lvisStateImageMask    = 0x0000F000
	lvisFocused           = 0x00000001
	lvisSelected          = 0x00000002
	lvnItemChanged        = -101
	lvnColumnClick        = -108
	lvnGetInfoTipW        = -158
	lvnSetFocus           = -7
	nmClick               = -2
	lvhtOnItemLabel       = 0x0004
	lvhtOnItemStateIcon   = 0x0008
	wmGetTextLength       = 0x000E
	wmGetText             = 0x000D
	enChange              = 0x0300
	enSetFocus            = 0x0100
	bnSetFocus            = 6
	ttfIDIsHwnd           = 0x0001
	ttfSubclass           = 0x0010
	ttmAddTool            = 0x0432
	ttmSetMaxTipWidth     = 0x0418
	ttsAlwaysTip          = 0x01
	ttsNoPrefix           = 0x02
	opaque                = 2
	sbHorz                = 0
	sbVert                = 1
	rdwInvalidate         = 0x0001
	rdwErase              = 0x0004
	rdwAllChildren        = 0x0080
	rdwUpdateNow          = 0x0100
	rdwFrame              = 0x0400
	odsSelected           = 0x0001
	odsFocus              = 0x0010
	odsDisabled           = 0x0004
	lvsilState            = 2
	ilcColor32            = 0x00000020
	swpNoZOrder           = 0x0004
	swpNoMove             = 0x0002
	swpNoSize             = 0x0001
	swpNoActivate         = 0x0010
	swpShowWindow         = 0x0040
	searchTimerID         = 1
	ofnHideReadOnly       = 0x00000004
	ofnNoChangeDir        = 0x00000008
	ofnPathMustExist      = 0x00000800
	ofnFileMustExist      = 0x00001000
	ofnExplorer           = 0x00080000
	ofnDontAddToRecent    = 0x02000000
)

const (
	processPickerAutoRefreshMinAge  = 15 * time.Second
	processPickerAutoRefreshMaxAge  = 60 * time.Second
	processPickerScanCostMultiplier = 250
)

type processLoadMode uint8

const (
	processLoadInitial processLoadMode = iota
	processLoadAutomatic
	processLoadManual
)

var (
	user32                = windows.NewLazySystemDLL("user32.dll")
	gdi32                 = windows.NewLazySystemDLL("gdi32.dll")
	pCreateWindowEx       = user32.NewProc("CreateWindowExW")
	pDestroyWindow        = user32.NewProc("DestroyWindow")
	pDefWindowProc        = user32.NewProc("DefWindowProcW")
	pRegisterClassEx      = user32.NewProc("RegisterClassExW")
	pSendMessage          = user32.NewProc("SendMessageW")
	pSetWindowText        = user32.NewProc("SetWindowTextW")
	pSetWindowPos         = user32.NewProc("SetWindowPos")
	pShowWindow           = user32.NewProc("ShowWindow")
	pUpdateWindow         = user32.NewProc("UpdateWindow")
	pRedrawWindow         = user32.NewProc("RedrawWindow")
	pSetForeground        = user32.NewProc("SetForegroundWindow")
	pEnableWindow         = user32.NewProc("EnableWindow")
	pIsWindow             = user32.NewProc("IsWindow")
	pIsWindowEnabled      = user32.NewProc("IsWindowEnabled")
	pGetDpiForWindow      = user32.NewProc("GetDpiForWindow")
	pGetWindowRect        = user32.NewProc("GetWindowRect")
	pShowScrollBar        = user32.NewProc("ShowScrollBar")
	pGetClientRect        = user32.NewProc("GetClientRect")
	pFillRect             = user32.NewProc("FillRect")
	pInvalidateRect       = user32.NewProc("InvalidateRect")
	pSetTimer             = user32.NewProc("SetTimer")
	pKillTimer            = user32.NewProc("KillTimer")
	pGetDC                = user32.NewProc("GetDC")
	pReleaseDC            = user32.NewProc("ReleaseDC")
	pSetTextColor         = gdi32.NewProc("SetTextColor")
	pSetBkColor           = gdi32.NewProc("SetBkColor")
	pSetBkMode            = gdi32.NewProc("SetBkMode")
	pCreateBrush          = gdi32.NewProc("CreateSolidBrush")
	pDeleteObject         = gdi32.NewProc("DeleteObject")
	pCreateCompatibleDC   = gdi32.NewProc("CreateCompatibleDC")
	pDeleteDC             = gdi32.NewProc("DeleteDC")
	pCreateBitmap         = gdi32.NewProc("CreateCompatibleBitmap")
	pSelectObject         = gdi32.NewProc("SelectObject")
	comdlg32              = windows.NewLazySystemDLL("comdlg32.dll")
	pGetOpenFileName      = comdlg32.NewProc("GetOpenFileNameW")
	kernel32              = windows.NewLazySystemDLL("kernel32.dll")
	pGetBinaryType        = kernel32.NewProc("GetBinaryTypeW")
	comctl                = windows.NewLazySystemDLL("comctl32.dll")
	pInitCommonControls   = comctl.NewProc("InitCommonControlsEx")
	pImageListCreate      = comctl.NewProc("ImageList_Create")
	pImageListAdd         = comctl.NewProc("ImageList_Add")
	pImageListDestroy     = comctl.NewProc("ImageList_Destroy")
	classOnce             sync.Once
	classErr              error
	activeMu              sync.Mutex
	active                *picker
	nextGeneration        atomic.Uint64
	wndCallback           = windows.NewCallback(wndProc)
	createWindowForPicker = func(exStyle uintptr, class, caption *uint16, style uintptr, x, y, width, height int, parent windows.Handle, id uintptr) (windows.Handle, error) {
		hwnd, _, callErr := pCreateWindowEx.Call(
			exStyle, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(caption)), style,
			uintptr(x), uintptr(y), uintptr(width), uintptr(height), uintptr(parent), id, 0, 0,
		)
		if hwnd == 0 {
			return 0, callErr
		}
		return windows.Handle(hwnd), nil
	}
	newFontForPicker            = font.New
	newCueBannerForPicker       = nativeform.NewCueBanner
	newScrollbarForPicker       = nativeform.NewScrollbar
	newListboxScrollForPicker   = nativeform.NewListboxScrollbar
	postPickerUI                = trayicon.Post
	snapshotNamesForPicker      = processcatalog.SnapshotNames
	enrichDescriptionsForPicker = processcatalog.EnrichDescriptions
	fileDescriptionForPicker    = processcatalog.FileDescription
	createBrushForPicker        = func(color uint32) windows.Handle {
		brush, _, _ := pCreateBrush.Call(uintptr(color))
		return windows.Handle(brush)
	}
)

func Show(options Options) error {
	activeMu.Lock()
	if active != nil && active.hwnd != 0 {
		activeMu.Unlock()
		Focus()
		return nil
	}
	activeMu.Unlock()
	if options.Text == nil {
		options.Text = func(key string) string { return key }
	}
	if err := ensureClass(); err != nil {
		return err
	}
	p := &picker{
		options: options, controls: make(map[uint16]windows.Handle), bounds: make(map[uint16]logicalBounds),
		labels: make(map[uint16]string), surfaceFields: make(map[uint16]uint16),
		selected: make(map[string]automation.ProcessTarget), descriptions: make(map[string]string), descriptionTried: make(map[string]struct{}), sortAscending: true,
		headerHover: -1, headerPressed: -1,
	}
	for key, value := range options.Descriptions {
		p.descriptions[key] = value
	}
	for _, target := range automation.NormalizeTargets(options.Selected) {
		p.selected[target.Key()] = target
	}
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

// Focus brings the active picker to the foreground and reports whether one is
// open. The automation manager uses it to preserve nested modal behavior.
func Focus() bool {
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
		p.destroy()
	}
}

// Capture creates the real picker with deterministic process groups for
// devtools visual checks. It never scans the host process list.
func Capture(options Options, groups []processcatalog.Group, scale float64, dark bool, capture func(windows.Handle) error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if options.Text == nil {
		options.Text = func(key string) string { return key }
	}
	if err := ensureClass(); err != nil {
		return err
	}
	options.Owner = 0
	p := &picker{
		options: options, controls: make(map[uint16]windows.Handle), bounds: make(map[uint16]logicalBounds),
		labels: make(map[uint16]string), surfaceFields: make(map[uint16]uint16),
		selected: make(map[string]automation.ProcessTarget), descriptions: make(map[string]string), descriptionTried: make(map[string]struct{}), sortAscending: true,
		headerHover: -1, headerPressed: -1,
		captureHost: true, captureScale: scale, themeOverride: &dark,
	}
	for key, value := range options.Descriptions {
		p.descriptions[key] = value
	}
	for _, target := range automation.NormalizeTargets(options.Selected) {
		p.selected[target.Key()] = target
	}
	activeMu.Lock()
	if active != nil {
		activeMu.Unlock()
		return fmt.Errorf("process picker already active")
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
			p.destroy()
		}
	}()
	p.items = buildItems(groups, p.selected, options.Text)
	p.loading = false
	p.applyFilter()
	if capture == nil {
		return nil
	}
	p.repaint()
	return capture(p.hwnd)
}

func (p *picker) repaint() {
	if p.hwnd == 0 {
		return
	}
	pInvalidateRect.Call(uintptr(p.hwnd), 0, 0)
	pUpdateWindow.Call(uintptr(p.hwnd))
	for _, control := range p.controls {
		pInvalidateRect.Call(uintptr(control), 0, 0)
		pUpdateWindow.Call(uintptr(control))
	}
	pRedrawWindow.Call(uintptr(p.hwnd), 0, 0, 0x0001|0x0080|0x0100|0x0400)
}

func (p *picker) abortCreate() {
	p.closed.Store(true)
	p.generation = nextGeneration.Add(1)
	if p.hwnd != 0 {
		if destroyed, _, _ := pDestroyWindow.Call(uintptr(p.hwnd)); destroyed != 0 {
			return
		}
	}
	p.releaseResources()
}

func (p *picker) destroy() {
	if p == nil {
		return
	}
	p.closed.Store(true)
	p.generation = nextGeneration.Add(1)
	if p.hwnd != 0 {
		if destroyed, _, _ := pDestroyWindow.Call(uintptr(p.hwnd)); destroyed != 0 {
			return
		}
	}
	p.releaseResources()
}

func (p *picker) releaseResources() {
	p.cleanupOnce.Do(func() {
		p.closed.Store(true)
		p.generation = nextGeneration.Add(1)
		hwnd := p.hwnd
		if hwnd != 0 {
			pKillTimer.Call(uintptr(hwnd), searchTimerID)
		}
		p.releaseHeaderRenderer()
		p.interaction.Release()
		if p.searchCue != nil {
			p.searchCue.Close()
			p.searchCue = nil
		}
		if p.scrollbar != nil {
			p.scrollbar.Close()
			p.scrollbar = nil
		}
		if p.previewScroll != nil {
			p.previewScroll.Close()
			p.previewScroll = nil
		}
		if p.contentScroll != nil {
			p.contentScroll.Close()
			p.contentScroll = nil
		}
		p.releaseStateImages()
		if p.tooltip != 0 {
			pDestroyWindow.Call(uintptr(p.tooltip))
			p.tooltip = 0
		}
		if hwnd != 0 {
			trayicon.ClearTabNavigationWindow(hwnd)
		}
		if p.font != 0 {
			pDeleteObject.Call(uintptr(p.font))
			p.font = 0
		}
		p.releaseBrushes()
		p.icons.Release()
		if p.ownerDisabled && p.options.Owner != 0 {
			if valid, _, _ := pIsWindow.Call(uintptr(p.options.Owner)); valid != 0 {
				pEnableWindow.Call(uintptr(p.options.Owner), 1)
				pSetForeground.Call(uintptr(p.options.Owner))
				trayicon.SetTabNavigationWindow(p.options.Owner, nil)
			}
			p.ownerDisabled = false
		}
		activeMu.Lock()
		if active == p {
			active = nil
		}
		activeMu.Unlock()
		p.hwnd = 0
		p.controls = nil
		p.bounds = nil
		p.tooltipText = nil
	})
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
			classErr = fmt.Errorf("register process picker: %w", callErr)
		}
	})
	return classErr
}

func (p *picker) create() (err error) {
	defer func() {
		if err != nil {
			p.abortCreate()
		}
	}()
	class, err := windows.UTF16PtrFromString(windowClass)
	if err != nil {
		return err
	}
	title, err := windows.UTF16PtrFromString(p.text("process_picker_title"))
	if err != nil {
		return err
	}
	p.style = wsPopup | wsCaption | wsSysMenu | wsClipChildren
	p.exStyle = wsExTopmost
	if p.captureHost {
		p.style = wsOverlapped | wsCaption | wsSysMenu | wsThickFrame | wsMinimizeBox | wsMaximizeBox | wsClipChildren
		p.exStyle = wsExAppWindow | wsExComposited
	}
	hwnd, callErr := createWindowForPicker(p.exStyle, class, title, p.style, 0, 0, 1, 1, p.options.Owner, 0)
	if callErr != nil {
		return fmt.Errorf("create process picker: %w", callErr)
	}
	p.hwnd = hwnd
	p.dpiScale = p.windowScale()
	scale := p.scale()
	p.font, _ = newFontForPicker(int32(14*scale+0.5), 400, p.options.Chinese)
	if p.font == 0 {
		return fmt.Errorf("create process picker font")
	}
	p.applyTheme()
	if p.windowBrush == 0 || p.surfaceBrush == 0 {
		return fmt.Errorf("create process picker brushes")
	}
	init := initCommonControlsEx{Size: uint32(unsafe.Sizeof(initCommonControlsEx{})), ICC: 0x00000001}
	if ok, _, callErr := pInitCommonControls.Call(uintptr(unsafe.Pointer(&init))); ok == 0 {
		return fmt.Errorf("initialize process picker controls: %w", callErr)
	}
	p.child("STATIC", p.text("process_picker_heading"), wsChild|wsVisible|ssLeft, 18, 16, 664, 20, idHeading)
	p.child("STATIC", p.text("process_picker_helper"), wsChild|wsVisible|ssLeft, 18, 40, 664, 30, idHelper)
	searchSurface := p.child("STATIC", "", wsChild|wsVisible|ssOwnerDraw, 18, 78, 370, 34, idSearchSurface)
	pSetWindowPos.Call(uintptr(searchSurface), 1, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
	search := p.child("EDIT", "", wsChild|wsVisible|wsTabStop|esAutoHScroll, 20, 85, 366, 20, idSearch)
	if p.createErr != nil {
		return p.createErr
	}
	cue, cueErr := newCueBannerForPicker(search, p.text("process_picker_search_hint"), p.palette.MutedText, p.scale())
	if cueErr != nil {
		return cueErr
	}
	p.searchCue = cue
	p.surfaceFields[idSearchSurface] = idSearch
	p.interaction.Track(search, p.controls[idSearchSurface])
	p.child("BUTTON", p.text("process_picker_refresh"), wsChild|wsVisible|wsTabStop|bsOwnerDraw, 396, 78, 132, 34, idRefresh)
	p.child("BUTTON", p.text("process_picker_browse"), wsChild|wsVisible|wsTabStop|bsOwnerDraw, 536, 78, 146, 34, idBrowse)
	listSurface := p.child("STATIC", "", wsChild|wsVisible|ssOwnerDraw, 18, 120, 664, 180, idListSurface)
	pSetWindowPos.Call(uintptr(listSurface), 1, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
	list := p.child("SysListView32", "", wsChild|wsVisible|wsTabStop|wsClipSiblings|lvsReport|lvsSingleSel|lvsShowSelAlways, 20, 122, 660, 176, idList)
	if p.createErr != nil {
		return p.createErr
	}
	p.surfaceFields[idListSurface] = idList
	pSendMessage.Call(uintptr(list), lvmSetExtendedStyle, 0, lvsExCheckboxes|lvsExFullRowSelect|lvsExInfoTip|lvsExDoubleBuffer)
	p.createColumns()
	p.resizeColumns()
	p.updateHeaderLabels()
	if err := p.installHeaderRenderer(); err != nil {
		return err
	}
	p.applyListTheme()
	if err := p.applyStateImages(); err != nil {
		return err
	}
	scrollbar, scrollErr := newScrollbarForPicker(nativeform.ScrollbarOptions{
		Parent: p.hwnd, Palette: p.palette, Background: p.palette.Surface, Scale: p.scale(),
		OnChange: p.scrollListTo,
	})
	if scrollErr != nil {
		return scrollErr
	}
	p.scrollbar = scrollbar
	p.syncListScrollbarBounds()
	p.syncListScrollbar()
	p.resizeColumns()
	pShowScrollBar.Call(uintptr(list), sbHorz, 0)
	p.child("STATIC", p.text("process_picker_loading"), wsChild|ssLeft, 42, 180, 616, 40, idEmpty)
	p.child("STATIC", p.text("process_picker_loading"), wsChild|wsVisible|ssLeft, 18, 308, 664, 20, idStatus)
	p.child("STATIC", p.text("process_picker_selection_title"), wsChild|wsVisible|ssLeft, 18, 336, 664, 20, idPreviewTitle)
	previewSurface := p.child("STATIC", "", wsChild|wsVisible|ssOwnerDraw, 18, 360, 664, 58, idPreviewSurface)
	pSetWindowPos.Call(uintptr(previewSurface), 1, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
	preview := p.child("LISTBOX", "", wsChild|wsVisible|wsVScroll|lbsNoSelection|lbsNoIntegralHeight, 20, 362, 660, 54, idPreview)
	if p.createErr != nil {
		return p.createErr
	}
	previewScroll, previewErr := newListboxScrollForPicker(nativeform.ListboxScrollbarOptions{
		Parent: p.hwnd, Listbox: preview, Palette: p.palette, Background: p.palette.Surface, Scale: p.scale(),
	})
	if previewErr != nil {
		return previewErr
	}
	p.previewScroll = previewScroll
	p.syncPreviewScrollbarBounds()
	p.child("STATIC", p.text("process_picker_privacy"), wsChild|wsVisible|ssLeft, 18, 426, 664, 22, idPrivacy)
	p.child("BUTTON", p.text("common_cancel"), wsChild|wsVisible|wsTabStop|bsOwnerDraw, 462, 456, 106, 36, idCancel)
	p.child("BUTTON", p.text("process_picker_confirm"), wsChild|wsVisible|wsTabStop|bsOwnerDraw, 576, 456, 106, 36, idConfirm)
	if p.createErr != nil {
		return p.createErr
	}
	contentScroll, contentScrollErr := newScrollbarForPicker(nativeform.ScrollbarOptions{
		Parent: p.hwnd, Palette: p.palette, Background: p.palette.WindowBackground, Scale: p.scale(),
		OnChange: p.scrollContentTo,
	})
	if contentScrollErr != nil {
		return contentScrollErr
	}
	p.contentScroll = contentScroll
	for _, id := range []uint16{idSearch, idList, idPreview} {
		pSetWindowPos.Call(uintptr(p.controls[id]), 0, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
	}
	p.updatePreview()
	if err := p.createTooltips(); err != nil {
		return err
	}
	p.positionInWorkArea(nil)
	if p.layoutErr != nil {
		return p.layoutErr
	}
	if p.options.Owner != 0 {
		if enabled, _, _ := pIsWindowEnabled.Call(uintptr(p.options.Owner)); enabled != 0 {
			pEnableWindow.Call(uintptr(p.options.Owner), 0)
			p.ownerDisabled = true
		}
	}
	if !p.captureHost {
		// Establish the loading state before the first visible frame so the
		// picker never flashes an uninitialized empty list.
		p.startLoad(processLoadInitial)
	}
	pShowWindow.Call(uintptr(hwnd), 5)
	if p.captureHost {
		// STARTUPINFO from a hidden screenshot host may override the first
		// ShowWindow call; repeat it so PrintWindow sees the child controls.
		pShowWindow.Call(uintptr(hwnd), 5)
	} else {
		pSetForeground.Call(uintptr(hwnd))
		trayicon.SetTabNavigationWindow(p.hwnd, nil)
	}
	return nil
}

func (p *picker) createColumns() {
	columns := []struct {
		key   string
		width int
	}{
		{"process_picker_column_process", 250},
		{"process_picker_column_description", 310},
		{"process_picker_column_instances", 82},
	}
	for index, column := range columns {
		text, _ := windows.UTF16PtrFromString(p.text(column.key))
		value := lvColumn{Mask: 0x0001 | 0x0002 | 0x0004, Cx: int32(float64(column.width) * p.scale()), Text: text, SubItem: int32(index)}
		pSendMessage.Call(uintptr(p.controls[idList]), lvmInsertColumnW, uintptr(index), uintptr(unsafe.Pointer(&value)))
	}
}

func (p *picker) resizeColumns() {
	list := p.controls[idList]
	if list == 0 {
		return
	}
	var client rect
	pGetClientRect.Call(uintptr(list), uintptr(unsafe.Pointer(&client)))
	widths := processColumnWidths(int(client.Right-client.Left), p.scale())
	for index, width := range widths {
		pSendMessage.Call(uintptr(list), lvmSetColumnWidth, uintptr(index), uintptr(width))
	}
	// Common controls may recreate non-client scrollbars after a column resize.
	// Reassert the themed vertical overlay and the no-horizontal-scroll contract
	// after the final widths are installed.
	pShowScrollBar.Call(uintptr(list), sbVert, 0)
	pShowScrollBar.Call(uintptr(list), sbHorz, 0)
}

func processColumnWidths(clientWidth int, scale float64) [3]int {
	// The themed scrollbar begins below the native header, so the header and
	// its columns use the full client width. The body scrollbar overlays the
	// roomy instance-count column without creating a fourth header remainder.
	available := max(0, clientWidth)
	last := min(available, int(82*scale+0.5))
	remaining := available - last
	minimumFirst := int(190*scale + 0.5)
	minimumSecond := int(180*scale + 0.5)
	desiredSecond := int(310*scale + 0.5)
	first, second := remaining, 0
	if remaining >= minimumFirst+minimumSecond {
		second = min(desiredSecond, remaining-minimumFirst)
		first = remaining - second
	}
	return [3]int{first, second, last}
}

func (p *picker) syncListScrollbarBounds() {
	if p.scrollbar == nil {
		return
	}
	scale := p.scale()
	bounds := p.bounds[idList]
	headerHeight := int(24*scale + 0.5)
	if p.header != 0 {
		var headerBounds rect
		if ok, _, _ := pGetWindowRect.Call(uintptr(p.header), uintptr(unsafe.Pointer(&headerBounds))); ok != 0 && headerBounds.Bottom > headerBounds.Top {
			headerHeight = int(headerBounds.Bottom - headerBounds.Top)
		}
	}
	width := int(float64(nativeform.ScrollbarWidth)*scale + 0.5)
	rightInset := int(2*scale + 0.5)
	x := int(float64(bounds.X+bounds.Width)*scale+0.5) - width - rightInset
	y := int(float64(bounds.Y-p.contentOffset)*scale+0.5) + headerHeight
	height := int(float64(bounds.Height)*scale+0.5) - headerHeight
	p.scrollbar.SetBounds(x, y, width, max(1, height))
}

func (p *picker) syncListScrollbar() {
	list := p.controls[idList]
	if list == 0 || p.scrollbar == nil {
		return
	}
	// Native non-client scrollbars cannot be colored reliably on every
	// supported Windows build. The list keeps its native scroll mechanics while
	// the shared themed scrollbar provides the visible mouse affordance.
	pShowScrollBar.Call(uintptr(list), sbVert, 0)
	top, _, _ := pSendMessage.Call(uintptr(list), lvmGetTopIndex, 0, 0)
	page, _, _ := pSendMessage.Call(uintptr(list), lvmGetCountPerPage, 0, 0)
	p.scrollbar.SetMetrics(len(p.visible), int(page), int(top))
}

func (p *picker) syncPreviewScrollbarBounds() {
	if p.previewScroll == nil {
		return
	}
	scale := p.scale()
	bounds := p.bounds[idPreview]
	width := int(float64(nativeform.ScrollbarWidth)*scale + 0.5)
	inset := max(1, int(2*scale+0.5))
	x := int(float64(bounds.X+bounds.Width)*scale+0.5) - width - inset
	y := int(float64(bounds.Y-p.contentOffset)*scale+0.5) + inset
	height := int(float64(bounds.Height)*scale+0.5) - 2*inset
	p.previewScroll.SetBounds(x, y, width, max(1, height))
}

func (p *picker) scrollListTo(position int) {
	list := p.controls[idList]
	if list == 0 || len(p.visible) == 0 {
		return
	}
	current, _, _ := pSendMessage.Call(uintptr(list), lvmGetTopIndex, 0, 0)
	if int(current) == position {
		return
	}
	itemBounds := rect{}
	if ok, _, _ := pSendMessage.Call(uintptr(list), lvmGetItemRect, 0, uintptr(unsafe.Pointer(&itemBounds))); ok == 0 {
		return
	}
	rowHeight := max(1, int(itemBounds.Bottom-itemBounds.Top))
	delta := (position - int(current)) * rowHeight
	pSendMessage.Call(uintptr(list), lvmScroll, 0, uintptr(delta))
	p.syncListScrollbar()
}

func (p *picker) updateHeaderLabels() {
	for index := range 3 {
		caption := p.headerCaption(index)
		text, _ := windows.UTF16PtrFromString(caption)
		column := lvColumn{Mask: 0x0004, Text: text}
		pSendMessage.Call(uintptr(p.controls[idList]), lvmSetColumnW, uintptr(index), uintptr(unsafe.Pointer(&column)))
	}
}

func (p *picker) headerCaption(index int) string {
	keys := []string{"process_picker_column_process", "process_picker_column_description", "process_picker_column_instances"}
	if index < 0 || index >= len(keys) {
		return ""
	}
	caption := p.text(keys[index])
	if p.sortColumn == index {
		if p.sortAscending {
			caption += "  ↑"
		} else {
			caption += "  ↓"
		}
	}
	return caption
}

func (p *picker) child(className, value string, style uintptr, x, y, width, height int, id uint16) windows.Handle {
	if p.createErr != nil {
		return 0
	}
	class, err := windows.UTF16PtrFromString(className)
	if err != nil {
		p.createErr = err
		return 0
	}
	caption, err := windows.UTF16PtrFromString(value)
	if err != nil {
		p.createErr = err
		return 0
	}
	scale := p.scale()
	hwnd, err := createWindowForPicker(0, class, caption, style,
		int(float64(x)*scale), int(float64(y)*scale), int(float64(width)*scale), int(float64(height)*scale),
		p.hwnd, uintptr(id))
	if err != nil {
		p.createErr = fmt.Errorf("create process picker control %d: %w", id, err)
		return 0
	}
	if p.font != 0 {
		pSendMessage.Call(uintptr(hwnd), wmSetFont, uintptr(p.font), 1)
	}
	nativeform.ApplyControl(hwnd, p.themeDark)
	p.controls[id] = hwnd
	p.bounds[id] = logicalBounds{X: x, Y: y, Width: width, Height: height}
	p.labels[id] = value
	if strings.EqualFold(className, "BUTTON") && style&bsOwnerDraw != 0 {
		p.interaction.Track(hwnd, 0)
	}
	return hwnd
}

func (p *picker) createTooltips() error {
	class, err := windows.UTF16PtrFromString("tooltips_class32")
	if err != nil {
		return err
	}
	hwnd, err := createWindowForPicker(0, class, nil, wsPopup|ttsAlwaysTip|ttsNoPrefix, 0, 0, 0, 0, p.hwnd, 0)
	if err != nil {
		return fmt.Errorf("create process picker tooltip: %w", err)
	}
	p.tooltip = hwnd
	nativeform.ApplyTooltip(p.tooltip, p.themeDark, p.palette)
	pSendMessage.Call(uintptr(hwnd), ttmSetMaxTipWidth, 0, uintptr(int(360*p.scale())))
	for id, key := range map[uint16]string{
		idSearch: "tip_process_search", idRefresh: "tip_process_refresh", idBrowse: "tip_process_browse",
		idCancel: "tip_process_cancel", idConfirm: "tip_process_confirm",
	} {
		if err := p.addTooltip(p.controls[id], p.text(key)); err != nil {
			return err
		}
	}
	return nil
}

func (p *picker) addTooltip(control windows.Handle, value string) error {
	if p.tooltip == 0 || control == 0 || value == "" {
		return nil
	}
	text, err := windows.UTF16FromString(value)
	if err != nil {
		return err
	}
	p.tooltipText = append(p.tooltipText, text)
	info := toolInfo{Size: uint32(unsafe.Sizeof(toolInfo{})), Flags: ttfIDIsHwnd | ttfSubclass, Hwnd: p.hwnd, ID: uintptr(control), Text: &p.tooltipText[len(p.tooltipText)-1][0]}
	// Some common-controls builds return zero here even though the tooltip is
	// registered and displayed. HWND creation is the reliable lifecycle gate;
	// the tooltip remains optional if an individual tool registration is refused.
	pSendMessage.Call(uintptr(p.tooltip), ttmAddTool, 0, uintptr(unsafe.Pointer(&info)))
	return nil
}

func (p *picker) startLoad(mode processLoadMode) {
	if p.closed.Load() || p.hwnd == 0 {
		return
	}
	p.captureSelection()
	p.captureViewAnchors()
	p.loading = true
	p.enriching = false
	p.setText(idStatus, p.text("process_picker_loading"))
	p.setText(idEmpty, p.text("process_picker_loading"))
	p.show(idEmpty, len(p.visible) == 0)
	p.enable(idRefresh, false)
	p.enable(idConfirm, len(p.selected) > 0)
	generation := nextGeneration.Add(1)
	p.generation = generation
	var exactTargets []automation.ProcessTarget
	for _, target := range p.selected {
		if target.Match == automation.MatchPath {
			exactTargets = append(exactTargets, target)
		}
	}
	cachedDescriptions := make(map[string]string, len(p.descriptions))
	for key, value := range p.descriptions {
		cachedDescriptions[key] = value
	}
	attemptedDescriptions := make(map[string]struct{}, len(p.descriptionTried))
	for key := range p.descriptionTried {
		attemptedDescriptions[key] = struct{}{}
	}
	p.workers.Add(1)
	go func() {
		defer p.workers.Done()
		scanStarted := time.Now()
		instances, err := snapshotNamesForPicker()
		scanDuration := time.Since(scanStarted)
		if err != nil {
			postPickerUI(func() { p.finishLoad(generation, nil, err, true, scanDuration) })
			return
		}
		for index := range instances {
			key := (automation.ProcessTarget{Match: automation.MatchName, Executable: instances[index].Executable}).Key()
			instances[index].Description = cachedDescriptions[key]
		}
		candidates, nameAttempts := descriptionCandidates(instances, cachedDescriptions, attemptedDescriptions, mode == processLoadManual)
		exactAttempts := make([]automation.ProcessTarget, 0, len(exactTargets))
		for _, target := range exactTargets {
			key := target.Key()
			if cachedDescriptions[key] != "" {
				continue
			}
			if _, tried := attemptedDescriptions[key]; tried && mode != processLoadManual {
				continue
			}
			exactAttempts = append(exactAttempts, target)
		}
		hasMetadataWork := len(nameAttempts) > 0 || len(exactAttempts) > 0
		postPickerUI(func() { p.finishLoad(generation, instances, nil, !hasMetadataWork, scanDuration) })
		nameDescriptions := make(map[string]string, len(nameAttempts))
		if len(candidates) > 0 {
			for _, instance := range enrichDescriptionsForPicker(candidates) {
				if instance.Description == "" {
					continue
				}
				key := (automation.ProcessTarget{Match: automation.MatchName, Executable: instance.Executable}).Key()
				nameDescriptions[key] = instance.Description
			}
		}
		exactDescriptions := make(map[string]string, len(exactAttempts))
		for _, target := range exactAttempts {
			if description := fileDescriptionForPicker(target.Path); description != "" {
				exactDescriptions[target.Key()] = description
			}
		}
		if hasMetadataWork {
			postPickerUI(func() {
				p.finishDescriptionLoad(generation, instances, nameAttempts, nameDescriptions, exactAttempts, exactDescriptions)
			})
		}
	}()
}

func descriptionCandidates(instances []processcatalog.Instance, cached map[string]string, attempted map[string]struct{}, retry bool) ([]processcatalog.Instance, []string) {
	wanted := make(map[string]struct{})
	attempts := make([]string, 0)
	for _, instance := range instances {
		key := (automation.ProcessTarget{Match: automation.MatchName, Executable: instance.Executable}).Key()
		if cached[key] != "" {
			continue
		}
		if _, tried := attempted[key]; tried && !retry {
			continue
		}
		if _, exists := wanted[key]; !exists {
			wanted[key] = struct{}{}
			attempts = append(attempts, key)
		}
	}
	candidates := make([]processcatalog.Instance, 0, len(instances))
	for _, instance := range instances {
		key := (automation.ProcessTarget{Match: automation.MatchName, Executable: instance.Executable}).Key()
		if _, ok := wanted[key]; ok {
			candidates = append(candidates, instance)
		}
	}
	return candidates, attempts
}

func (p *picker) finishDescriptionLoad(generation uint64, instances []processcatalog.Instance, nameAttempts []string, nameDescriptions map[string]string, exactAttempts []automation.ProcessTarget, exactDescriptions map[string]string) {
	if p.closed.Load() || p.hwnd == 0 || p.generation != generation {
		return
	}
	for _, key := range nameAttempts {
		p.descriptionTried[key] = struct{}{}
	}
	for _, target := range exactAttempts {
		p.descriptionTried[target.Key()] = struct{}{}
	}
	for key, value := range nameDescriptions {
		p.descriptions[key] = value
	}
	for key, value := range exactDescriptions {
		p.descriptions[key] = value
	}
	for index := range instances {
		key := (automation.ProcessTarget{Match: automation.MatchName, Executable: instances[index].Executable}).Key()
		instances[index].Description = p.descriptions[key]
	}
	p.finishLoad(generation, instances, nil, true, 0)
}

func (p *picker) finishLoad(generation uint64, instances []processcatalog.Instance, err error, final bool, scanDuration time.Duration) {
	if p.closed.Load() || p.hwnd == 0 || p.generation != generation {
		return
	}
	if err != nil {
		p.loading = false
		p.enriching = false
		p.setText(idStatus, fmt.Sprintf(p.text("process_picker_error"), err.Error()))
		p.setText(idEmpty, p.text("process_picker_load_failed"))
		p.show(idEmpty, len(p.visible) == 0)
		p.enable(idRefresh, true)
		p.enable(idConfirm, len(p.selected) > 0)
		p.anchorTopKey, p.anchorFocusKey = "", ""
		return
	}
	if scanDuration > 0 {
		p.lastSnapshot = time.Now()
		p.lastScanDuration = scanDuration
	}
	groups := processcatalog.GroupInstances(instances)
	for index := range groups {
		key := (automation.ProcessTarget{Match: automation.MatchName, Executable: groups[index].Executable}).Key()
		if groups[index].Description != "" {
			p.descriptions[key] = groups[index].Description
		} else if cached := p.descriptions[key]; cached != "" {
			groups[index].Description = cached
		}
	}
	p.items = buildItems(groups, p.selected, p.options.Text)
	p.loading = false
	p.enriching = !final
	p.enable(idRefresh, true)
	p.applyFilter()
	p.restoreViewAnchors(final)
	if p.enriching {
		p.setText(idStatus, p.text("process_picker_loading_descriptions"))
	}
	p.updatePreview()
}

func (p *picker) captureViewAnchors() {
	list := p.controls[idList]
	if list == 0 || len(p.visible) == 0 {
		p.anchorTopKey, p.anchorFocusKey = "", ""
		return
	}
	top, _, _ := pSendMessage.Call(uintptr(list), lvmGetTopIndex, 0, 0)
	if int(top) >= 0 && int(top) < len(p.visible) {
		p.anchorTopKey = p.visible[int(top)].target.Key()
	}
	p.anchorFocusKey = ""
	for index, value := range p.visible {
		state, _, _ := pSendMessage.Call(uintptr(list), lvmGetItemState, uintptr(index), lvisFocused)
		if state&lvisFocused != 0 {
			p.anchorFocusKey = value.target.Key()
			break
		}
	}
}

func (p *picker) restoreViewAnchors(clear bool) {
	indexOf := func(key string) int {
		for index, value := range p.visible {
			if value.target.Key() == key {
				return index
			}
		}
		return -1
	}
	if index := indexOf(p.anchorTopKey); index >= 0 {
		p.scrollListTo(index)
	}
	if index := indexOf(p.anchorFocusKey); index >= 0 {
		entry := lvItem{StateMask: lvisFocused | lvisSelected, State: lvisFocused | lvisSelected}
		pSendMessage.Call(uintptr(p.controls[idList]), lvmSetItemState, uintptr(index), uintptr(unsafe.Pointer(&entry)))
		pSendMessage.Call(uintptr(p.controls[idList]), lvmEnsureVisible, uintptr(index), 0)
	}
	if clear {
		p.anchorTopKey, p.anchorFocusKey = "", ""
	}
}

func processPickerRefreshAge(scanDuration time.Duration) time.Duration {
	age := scanDuration * processPickerScanCostMultiplier
	if age < processPickerAutoRefreshMinAge {
		return processPickerAutoRefreshMinAge
	}
	if age > processPickerAutoRefreshMaxAge {
		return processPickerAutoRefreshMaxAge
	}
	return age
}

func shouldAutoRefreshProcessPicker(last, now time.Time, scanDuration time.Duration, loading bool) bool {
	return !loading && !last.IsZero() && now.Sub(last) >= processPickerRefreshAge(scanDuration)
}

func buildItems(groups []processcatalog.Group, selected map[string]automation.ProcessTarget, text func(string) string) []item {
	items := make([]item, 0, len(groups)+len(selected))
	present := make(map[string]struct{})
	sort.SliceStable(groups, func(i, j int) bool {
		return strings.ToLower(groups[i].Executable) < strings.ToLower(groups[j].Executable)
	})
	for _, group := range groups {
		target := automation.ProcessTarget{Match: automation.MatchName, Executable: group.Executable}
		items = append(items, item{
			target: target, name: group.Executable, description: group.Description,
			count:  strconv.Itoa(group.Count),
			search: strings.ToLower(group.Executable + " " + group.Description),
		})
		present[target.Key()] = struct{}{}
	}
	for key, target := range selected {
		if target.Match == automation.MatchPath {
			continue
		}
		if _, ok := present[key]; ok {
			continue
		}
		description := text("process_picker_not_running")
		items = append(items, item{target: target, name: target.Executable, description: description, search: strings.ToLower(target.Executable + " " + description)})
	}
	return items
}

func (p *picker) applyFilter() {
	filter := strings.ToLower(strings.TrimSpace(p.controlText(idSearch)))
	sorted := sortItems(p.items, p.sortColumn, p.sortAscending)
	next := filterItems(sorted, filter)
	previous := p.visible
	p.visible = next
	p.reconcileVisible(previous, next)
	if len(p.visible) == 0 {
		message := p.text("process_picker_empty")
		if filter != "" {
			message = p.text("process_picker_no_results")
		}
		p.setText(idEmpty, message)
		p.show(idEmpty, true)
	} else {
		p.show(idEmpty, false)
	}
	p.updateSelectionStatus()
}

func (p *picker) reconcileVisible(previous, next []item) {
	list := p.controls[idList]
	if list == 0 {
		return
	}
	p.populating = true
	pSendMessage.Call(uintptr(list), wmSetRedraw, 0, 0)
	if len(next) == 0 || !sameRelativeTargetOrder(previous, next) {
		pSendMessage.Call(uintptr(list), lvmDeleteAllItems, 0, 0)
		for index, value := range next {
			p.insertItem(index, value)
		}
	} else {
		nextKeys := make(map[string]struct{}, len(next))
		for _, value := range next {
			nextKeys[value.target.Key()] = struct{}{}
		}
		retained := make([]item, 0, min(len(previous), len(next)))
		for index := len(previous) - 1; index >= 0; index-- {
			if _, keep := nextKeys[previous[index].target.Key()]; keep {
				continue
			}
			pSendMessage.Call(uintptr(list), lvmDeleteItem, uintptr(index), 0)
		}
		for _, value := range previous {
			if _, keep := nextKeys[value.target.Key()]; keep {
				retained = append(retained, value)
			}
		}
		retainedIndex := 0
		for index, value := range next {
			if retainedIndex < len(retained) && retained[retainedIndex].target.Key() == value.target.Key() {
				p.updateItem(index, retained[retainedIndex], value)
				retainedIndex++
				continue
			}
			p.insertItem(index, value)
		}
	}
	pSendMessage.Call(uintptr(list), wmSetRedraw, 1, 0)
	p.populating = false
	// The vertical scrollbar appears only after rows are populated and reduces
	// the usable report width. Refit at that point so the list never creates a
	// horizontal scrollbar.
	p.syncListScrollbarBounds()
	p.syncListScrollbar()
	p.resizeColumns()
	pShowScrollBar.Call(uintptr(list), sbHorz, 0)
	pRedrawWindow.Call(uintptr(list), 0, 0, rdwInvalidate|rdwErase|rdwAllChildren|rdwUpdateNow|rdwFrame)
}

func sameRelativeTargetOrder(previous, next []item) bool {
	if len(previous) == 0 || len(next) == 0 {
		return true
	}
	nextKeys := make(map[string]struct{}, len(next))
	previousKeys := make(map[string]struct{}, len(previous))
	for _, value := range next {
		nextKeys[value.target.Key()] = struct{}{}
	}
	for _, value := range previous {
		previousKeys[value.target.Key()] = struct{}{}
	}
	commonPrevious := make([]string, 0, min(len(previous), len(next)))
	commonNext := make([]string, 0, cap(commonPrevious))
	for _, value := range previous {
		if _, ok := nextKeys[value.target.Key()]; ok {
			commonPrevious = append(commonPrevious, value.target.Key())
		}
	}
	for _, value := range next {
		if _, ok := previousKeys[value.target.Key()]; ok {
			commonNext = append(commonNext, value.target.Key())
		}
	}
	if len(commonPrevious) == 0 {
		return false
	}
	if len(commonPrevious) != len(commonNext) {
		return false
	}
	for index := range commonPrevious {
		if commonPrevious[index] != commonNext[index] {
			return false
		}
	}
	return true
}

func (p *picker) captureSelection() {
	list := p.controls[idList]
	if list == 0 {
		return
	}
	for index := range p.visible {
		state, _, _ := pSendMessage.Call(uintptr(list), lvmGetItemState, uintptr(index), lvisStateImageMask)
		key := p.visible[index].target.Key()
		if uint32(state)&lvisStateImageMask == 2<<12 {
			p.selected[key] = p.visible[index].target
		} else {
			delete(p.selected, key)
		}
	}
	p.selected = normalizeSelected(p.selected)
	p.syncCheckStates()
	p.updatePreview()
}

func (p *picker) insertItem(index int, value item) {
	columns := []string{value.name, value.description, value.count}
	text, _ := windows.UTF16PtrFromString(columns[0])
	entry := lvItem{Mask: lvifText, Item: int32(index), Text: text}
	pSendMessage.Call(uintptr(p.controls[idList]), lvmInsertItemW, 0, uintptr(unsafe.Pointer(&entry)))
	for column := 1; column < len(columns); column++ {
		value, _ := windows.UTF16PtrFromString(columns[column])
		entry = lvItem{SubItem: int32(column), Text: value}
		pSendMessage.Call(uintptr(p.controls[idList]), lvmSetItemTextW, uintptr(index), uintptr(unsafe.Pointer(&entry)))
	}
	checked := uint32(1 << 12)
	if _, ok := p.selected[value.target.Key()]; ok {
		checked = 2 << 12
	}
	entry = lvItem{StateMask: lvisStateImageMask, State: checked}
	pSendMessage.Call(uintptr(p.controls[idList]), lvmSetItemState, uintptr(index), uintptr(unsafe.Pointer(&entry)))
}

func (p *picker) updateItem(index int, previous, next item) {
	previousColumns := []string{previous.name, previous.description, previous.count}
	nextColumns := []string{next.name, next.description, next.count}
	for column, value := range nextColumns {
		if value == previousColumns[column] {
			continue
		}
		text, _ := windows.UTF16PtrFromString(value)
		entry := lvItem{SubItem: int32(column), Text: text}
		pSendMessage.Call(uintptr(p.controls[idList]), lvmSetItemTextW, uintptr(index), uintptr(unsafe.Pointer(&entry)))
	}
	state := uint32(1 << 12)
	if _, ok := p.selected[next.target.Key()]; ok {
		state = 2 << 12
	}
	entry := lvItem{StateMask: lvisStateImageMask, State: state}
	pSendMessage.Call(uintptr(p.controls[idList]), lvmSetItemState, uintptr(index), uintptr(unsafe.Pointer(&entry)))
}

func (p *picker) syncCheckStates() {
	if p.controls[idList] == 0 {
		return
	}
	p.populating = true
	defer func() { p.populating = false }()
	for index, value := range p.visible {
		state := uint32(1 << 12)
		if _, ok := p.selected[value.target.Key()]; ok {
			state = 2 << 12
		}
		entry := lvItem{StateMask: lvisStateImageMask, State: state}
		pSendMessage.Call(uintptr(p.controls[idList]), lvmSetItemState, uintptr(index), uintptr(unsafe.Pointer(&entry)))
	}
}

func (p *picker) updatePreview() {
	preview := p.controls[idPreview]
	if preview == 0 {
		return
	}
	targets := make([]automation.ProcessTarget, 0, len(p.selected))
	for _, target := range p.selected {
		targets = append(targets, target)
	}
	targets = automation.NormalizeTargets(targets)
	pSendMessage.Call(uintptr(preview), lbResetContent, 0, 0)
	for _, target := range targets {
		name := target.Executable
		if description := p.descriptions[target.Key()]; description != "" {
			name = fmt.Sprintf(p.text("process_name_description"), name, description)
		}
		label := fmt.Sprintf(p.text("process_picker_preview_name"), name)
		if target.Match == automation.MatchPath {
			label = fmt.Sprintf(p.text("process_picker_preview_path"), name, target.Path)
		}
		text, _ := windows.UTF16PtrFromString(label)
		pSendMessage.Call(uintptr(preview), lbAddString, 0, uintptr(unsafe.Pointer(text)))
	}
	if len(targets) == 0 {
		text, _ := windows.UTF16PtrFromString(p.text("process_picker_preview_empty"))
		pSendMessage.Call(uintptr(preview), lbAddString, 0, uintptr(unsafe.Pointer(text)))
	}
	p.setText(idPreviewTitle, fmt.Sprintf(p.text("process_picker_selection_title"), len(targets)))
	p.enable(idConfirm, len(targets) > 0 && len(targets) <= automation.MaxProcessesPerRule)
	if p.previewScroll != nil {
		p.previewScroll.Sync()
	}
}

func filterItems(values []item, filter string) []item {
	if filter == "" {
		return append([]item(nil), values...)
	}
	out := make([]item, 0, len(values))
	for _, value := range values {
		if strings.Contains(value.search, filter) {
			out = append(out, value)
		}
	}
	return out
}

func sortItems(values []item, column int, ascending bool) []item {
	out := append([]item(nil), values...)
	valueAt := func(value item) string {
		switch column {
		case 1:
			return value.description
		case 2:
			count, _ := strconv.Atoi(value.count)
			return fmt.Sprintf("%08d", count)
		default:
			return value.name
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		left := strings.ToLower(valueAt(out[i]))
		right := strings.ToLower(valueAt(out[j]))
		if ascending {
			return left < right
		}
		return left > right
	})
	return out
}

func normalizeSelected(values map[string]automation.ProcessTarget) map[string]automation.ProcessTarget {
	targets := make([]automation.ProcessTarget, 0, len(values))
	for _, target := range values {
		targets = append(targets, target)
	}
	normalized := automation.NormalizeTargets(targets)
	out := make(map[string]automation.ProcessTarget, len(normalized))
	for _, target := range normalized {
		out[target.Key()] = target
	}
	return out
}

func (p *picker) updateSelectionStatus() {
	if len(p.selected) > automation.MaxProcessesPerRule {
		p.setText(idStatus, fmt.Sprintf(p.text("process_picker_limit"), automation.MaxProcessesPerRule))
		return
	}
	p.setText(idStatus, fmt.Sprintf(p.text("process_picker_status_results"), len(p.visible), len(p.selected)))
}

func canAddSelection(values map[string]automation.ProcessTarget, target automation.ProcessTarget) bool {
	if _, exists := values[target.Key()]; exists {
		return true
	}
	targets := make([]automation.ProcessTarget, 0, len(values)+1)
	for _, existing := range values {
		targets = append(targets, existing)
	}
	targets = append(targets, target)
	return len(automation.NormalizeTargets(targets)) <= automation.MaxProcessesPerRule
}

func (p *picker) confirm() {
	p.captureSelection()
	if len(p.selected) > automation.MaxProcessesPerRule {
		p.updateSelectionStatus()
		return
	}
	targets := make([]automation.ProcessTarget, 0, len(p.selected))
	for _, target := range p.selected {
		targets = append(targets, target)
	}
	targets = automation.NormalizeTargets(targets)
	descriptions := make(map[string]string, len(targets))
	for _, target := range targets {
		if description := p.descriptions[target.Key()]; description != "" {
			descriptions[target.Key()] = description
		}
	}
	callback := p.options.OnConfirm
	p.destroy()
	if callback != nil {
		callback(targets, descriptions)
	}
}

func (p *picker) browseExecutable() {
	filter := utf16.Encode([]rune(p.text("process_picker_exe_filter") + "\x00*.exe\x00\x00"))
	file := make([]uint16, 32768)
	title, _ := windows.UTF16PtrFromString(p.text("process_picker_browse_title"))
	defaultExtension, _ := windows.UTF16PtrFromString("exe")
	dialog := openFileName{
		Size: uint32(unsafe.Sizeof(openFileName{})), Owner: p.hwnd,
		Filter: &filter[0], FilterIndex: 1, File: &file[0], MaxFile: uint32(len(file)),
		Title: title, DefaultExtension: defaultExtension,
		Flags: ofnHideReadOnly | ofnNoChangeDir | ofnPathMustExist | ofnFileMustExist | ofnExplorer | ofnDontAddToRecent,
	}
	chosen, _, _ := pGetOpenFileName.Call(uintptr(unsafe.Pointer(&dialog)))
	if chosen == 0 {
		return
	}
	path := filepath.Clean(windows.UTF16ToString(file))
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil || !strings.EqualFold(filepath.Ext(path), ".exe") {
		p.setText(idStatus, p.text("process_picker_invalid_exe"))
		return
	}
	var binaryType uint32
	valid, _, _ := pGetBinaryType.Call(uintptr(unsafe.Pointer(pathPtr)), uintptr(unsafe.Pointer(&binaryType)))
	if valid == 0 {
		p.setText(idStatus, p.text("process_picker_invalid_exe"))
		return
	}
	target := automation.ProcessTarget{Match: automation.MatchPath, Executable: filepath.Base(path), Path: path}
	candidate := make(map[string]automation.ProcessTarget, len(p.selected)+1)
	for key, existing := range p.selected {
		candidate[key] = existing
	}
	for key, existing := range candidate {
		if existing.Match == automation.MatchName && strings.EqualFold(existing.Executable, target.Executable) {
			delete(candidate, key)
		}
	}
	if !canAddSelection(candidate, target) {
		p.setText(idStatus, fmt.Sprintf(p.text("process_picker_limit"), automation.MaxProcessesPerRule))
		return
	}
	candidate[target.Key()] = target
	p.selected = normalizeSelected(candidate)
	p.syncCheckStates()
	p.updatePreview()
	p.updateSelectionStatus()
	key := target.Key()
	p.workers.Add(1)
	go func() {
		defer p.workers.Done()
		description := fileDescriptionForPicker(path)
		postPickerUI(func() {
			if p.closed.Load() || p.hwnd == 0 {
				return
			}
			p.descriptionTried[key] = struct{}{}
			if description != "" {
				p.descriptions[key] = description
			}
			p.updatePreview()
		})
	}()
}

func (p *picker) handleCommand(id, notification uint16) {
	if notification == bnSetFocus || notification == enSetFocus {
		p.ensureControlVisible(id)
	}
	switch id {
	case idSearch:
		if notification == enChange {
			pKillTimer.Call(uintptr(p.hwnd), searchTimerID)
			pSetTimer.Call(uintptr(p.hwnd), searchTimerID, 120, 0)
		}
	case idRefresh:
		p.startLoad(processLoadManual)
	case idBrowse:
		p.browseExecutable()
	case idCancel:
		p.destroy()
	case idConfirm:
		p.confirm()
	}
}

func (p *picker) handleNotify(lParam unsafe.Pointer) {
	if lParam == nil {
		return
	}
	header := (*nmHeader)(lParam)
	if header.IDFrom != idList {
		return
	}
	switch header.Code {
	case lvnSetFocus:
		p.ensureControlVisible(idList)
	case lvnColumnClick:
		notification := (*nmListView)(lParam)
		column := int(notification.SubItem)
		if p.sortColumn == column {
			p.sortAscending = !p.sortAscending
		} else {
			p.sortColumn, p.sortAscending = column, true
		}
		p.updateHeaderLabels()
		p.applyFilter()
	case lvnItemChanged:
		notification := (*nmListView)(lParam)
		if p.populating || notification.Changed&lvifState == 0 || notification.Item < 0 {
			return
		}
		if notification.NewState&lvisStateImageMask != notification.OldState&lvisStateImageMask {
			if notification.NewState&lvisStateImageMask == 2<<12 && int(notification.Item) < len(p.visible) {
				target := p.visible[notification.Item].target
				if !canAddSelection(p.selected, target) {
					p.populating = true
					entry := lvItem{StateMask: lvisStateImageMask, State: 1 << 12}
					pSendMessage.Call(uintptr(p.controls[idList]), lvmSetItemState, uintptr(notification.Item), uintptr(unsafe.Pointer(&entry)))
					p.populating = false
					p.setText(idStatus, fmt.Sprintf(p.text("process_picker_limit"), automation.MaxProcessesPerRule))
					return
				}
			}
			p.captureSelection()
			p.updateSelectionStatus()
		}
	case lvnGetInfoTipW:
		p.fillRowInfoTip((*nmLVGetInfoTip)(lParam))
	case nmClick:
		notification := (*nmListView)(lParam)
		if notification.Item < 0 {
			return
		}
		hit := lvHitTestInfo{Item: -1, SubItem: -1}
		hit.Point = notification.Action
		pSendMessage.Call(uintptr(p.controls[idList]), lvmSubItemHitTest, 0, uintptr(unsafe.Pointer(&hit)))
		if hit.Item < 0 || hit.SubItem != 0 || hit.Flags&lvhtOnItemLabel == 0 || hit.Flags&lvhtOnItemStateIcon != 0 {
			return
		}
		state, _, _ := pSendMessage.Call(uintptr(p.controls[idList]), lvmGetItemState, uintptr(hit.Item), lvisStateImageMask)
		checked := uint32(2 << 12)
		if uint32(state)&lvisStateImageMask == 2<<12 {
			checked = 1 << 12
		} else if !canAddSelection(p.selected, p.visible[hit.Item].target) {
			p.setText(idStatus, fmt.Sprintf(p.text("process_picker_limit"), automation.MaxProcessesPerRule))
			return
		}
		p.populating = true
		entry := lvItem{StateMask: lvisStateImageMask, State: checked}
		pSendMessage.Call(uintptr(p.controls[idList]), lvmSetItemState, uintptr(hit.Item), uintptr(unsafe.Pointer(&entry)))
		p.populating = false
		p.captureSelection()
		p.updateSelectionStatus()
	}
}

func (p *picker) fillRowInfoTip(info *nmLVGetInfoTip) {
	if info == nil || info.Text == nil || info.TextMax <= 1 || info.Item < 0 || int(info.Item) >= len(p.visible) {
		return
	}
	value := p.visible[info.Item]
	if !p.listCellTruncated(value.name, 0, int(40*p.scale()+0.5)) &&
		!p.listCellTruncated(value.description, 1, int(12*p.scale()+0.5)) {
		return
	}
	label := value.name
	if value.description != "" {
		label = fmt.Sprintf(p.text("process_name_description"), value.name, value.description)
	}
	writeUTF16Text(info.Text, info.TextMax, label)
}

func (p *picker) listCellTruncated(value string, column, inset int) bool {
	if value == "" || p.controls[idList] == 0 {
		return false
	}
	text, _ := windows.UTF16PtrFromString(value)
	width, _, _ := pSendMessage.Call(uintptr(p.controls[idList]), lvmGetStringWidthW, 0, uintptr(unsafe.Pointer(text)))
	columnWidth, _, _ := pSendMessage.Call(uintptr(p.controls[idList]), lvmGetColumnWidth, uintptr(column), 0)
	return int(width)+inset > int(columnWidth)
}

func writeUTF16Text(destination *uint16, capacity int32, value string) {
	if destination == nil || capacity <= 0 {
		return
	}
	buffer := unsafe.Slice(destination, int(capacity))
	encoded := utf16.Encode([]rune(value))
	count := min(len(encoded), len(buffer)-1)
	copy(buffer[:count], encoded[:count])
	buffer[count] = 0
}

func (p *picker) text(key string) string { return p.options.Text(key) }
func (p *picker) setText(id uint16, value string) {
	if p.controls[id] == 0 {
		return
	}
	p.labels[id] = value
	text, _ := windows.UTF16PtrFromString(value)
	pSetWindowText.Call(uintptr(p.controls[id]), uintptr(unsafe.Pointer(text)))
	pInvalidateRect.Call(uintptr(p.controls[id]), 0, 0)
}
func (p *picker) show(id uint16, visible bool) {
	command := uintptr(0)
	if visible {
		command = 5
	}
	pShowWindow.Call(uintptr(p.controls[id]), command)
}
func (p *picker) enable(id uint16, enabled bool) {
	value := uintptr(0)
	if enabled {
		value = 1
	}
	pEnableWindow.Call(uintptr(p.controls[id]), value)
	pInvalidateRect.Call(uintptr(p.controls[id]), 0, 0)
}
func (p *picker) controlText(id uint16) string {
	hwnd := p.controls[id]
	length, _, _ := pSendMessage.Call(uintptr(hwnd), wmGetTextLength, 0, 0)
	buffer := make([]uint16, int(length)+1)
	if len(buffer) > 0 {
		pSendMessage.Call(uintptr(hwnd), wmGetText, uintptr(len(buffer)), uintptr(unsafe.Pointer(&buffer[0])))
	}
	return windows.UTF16ToString(buffer)
}

func (p *picker) scale() float64 {
	if p.captureScale > 0 {
		return p.captureScale
	}
	if p.dpiScale > 0 {
		return p.dpiScale
	}
	return p.windowScale()
}

func (p *picker) windowScale() float64 {
	if p.hwnd == 0 {
		return 1
	}
	dpi, _, _ := pGetDpiForWindow.Call(uintptr(p.hwnd))
	if dpi == 0 {
		return 1
	}
	return float64(dpi) / 96
}

func (p *picker) positionInWorkArea(workArea *nativeform.Rect) {
	scale := p.scale()
	anchor := p.hwnd
	if p.options.Owner != 0 {
		anchor = p.options.Owner
	}
	suggested := p.pendingSuggested
	p.pendingSuggested = nil
	_, err := nativeform.PlaceWindow(nativeform.WindowPlacement{
		Window: p.hwnd, Anchor: anchor, Owner: p.options.Owner,
		Style: p.style, ExStyle: p.exStyle,
		ClientWidth: int(windowWidth*scale + 0.5), ClientHeight: int(windowHeight*scale + 0.5),
		DPI: uint32(scale*96 + 0.5), Suggested: suggested, WorkArea: workArea,
	})
	if err != nil {
		p.layoutErr = err
		return
	}
	physicalWidth, physicalHeight, err := nativeform.ClientSize(p.hwnd)
	if err != nil {
		p.layoutErr = err
		return
	}
	p.layoutErr = nil
	p.viewportWidth = max(1, int(float64(physicalWidth)/scale))
	p.viewportHeight = max(1, int(float64(physicalHeight)/scale))
	maximum := max(0, windowHeight-p.viewportHeight)
	p.contentOffset = max(0, min(p.contentOffset, maximum))
	if p.contentScroll != nil {
		p.contentScroll.SetScale(scale)
		barWidth := max(1, int(float64(nativeform.ScrollbarWidth)*scale+0.5))
		inset := max(1, int(2*scale+0.5))
		p.contentScroll.SetBounds(physicalWidth-barWidth-inset, inset, barWidth, max(1, physicalHeight-2*inset))
		p.contentScroll.SetMetrics(windowHeight, max(1, p.viewportHeight), p.contentOffset)
	}
	p.layout()
}

func (p *picker) layout() {
	const pad, gap = nativeform.FormPadding, nativeform.ControlGap
	layoutWidth := min(windowWidth, max(1, p.viewportWidth))
	reserve := 0
	if windowHeight > p.viewportHeight {
		reserve = nativeform.ScrollbarWidth + 4
	}
	contentWidth := max(1, layoutWidth-2*pad-reserve)
	refreshWidth, browseWidth := 132, 146
	if contentWidth < 500 {
		refreshWidth, browseWidth = 96, 112
	}
	searchWidth := max(80, contentWidth-refreshWidth-browseWidth-2*gap)
	if searchWidth+refreshWidth+browseWidth+2*gap > contentWidth {
		remaining := max(2, contentWidth-searchWidth-2*gap)
		refreshWidth = remaining / 2
		browseWidth = remaining - refreshWidth
	}

	p.place(idHeading, pad, 16, contentWidth, 20)
	p.place(idHelper, pad, 40, contentWidth, 30)
	p.place(idSearchSurface, pad, 78, searchWidth, 34)
	p.place(idSearch, pad+2, 85, max(1, searchWidth-4), 20)
	p.place(idRefresh, pad+searchWidth+gap, 78, refreshWidth, 34)
	p.place(idBrowse, pad+searchWidth+gap+refreshWidth+gap, 78, browseWidth, 34)
	p.place(idListSurface, pad, 120, contentWidth, 180)
	p.place(idList, pad+2, 122, max(1, contentWidth-4), 176)
	p.place(idEmpty, pad+24, 180, max(1, contentWidth-48), 40)
	p.place(idStatus, pad, 308, contentWidth, 20)
	p.place(idPreviewTitle, pad, 336, contentWidth, 20)
	p.place(idPreviewSurface, pad, 360, contentWidth, 58)
	p.place(idPreview, pad+2, 362, max(1, contentWidth-4), 54)
	p.place(idPrivacy, pad, 426, contentWidth, 22)
	buttonGap := gap
	buttonWidth := min(106, max(1, (contentWidth-buttonGap)/2))
	p.place(idCancel, pad+contentWidth-2*buttonWidth-buttonGap, 456, buttonWidth, 36)
	p.place(idConfirm, pad+contentWidth-buttonWidth, 456, buttonWidth, 36)
	p.resizeColumns()
	p.syncListScrollbarBounds()
	p.syncListScrollbar()
	p.syncPreviewScrollbarBounds()
	if p.previewScroll != nil {
		p.previewScroll.Sync()
	}
}

func (p *picker) place(id uint16, x, y, width, height int) {
	p.bounds[id] = logicalBounds{X: x, Y: y, Width: width, Height: height}
	control := p.controls[id]
	if control == 0 {
		return
	}
	scale := p.scale()
	pSetWindowPos.Call(uintptr(control), 0,
		uintptr(int(float64(x)*scale)), uintptr(int(float64(y-p.contentOffset)*scale)),
		uintptr(max(1, int(float64(width)*scale))), uintptr(max(1, int(float64(height)*scale))),
		swpNoZOrder|swpNoActivate)
}

func (p *picker) scrollContentTo(position int) {
	maximum := max(0, windowHeight-p.viewportHeight)
	position = max(0, min(position, maximum))
	if position == p.contentOffset {
		return
	}
	p.contentOffset = position
	p.layout()
	if p.contentScroll != nil {
		p.contentScroll.SetMetrics(windowHeight, max(1, p.viewportHeight), p.contentOffset)
	}
}

func (p *picker) ensureControlVisible(id uint16) {
	bounds, ok := p.bounds[id]
	if !ok || windowHeight <= p.viewportHeight {
		return
	}
	position := p.contentOffset
	if bounds.Y < position {
		position = bounds.Y
	} else if bounds.Y+bounds.Height > position+p.viewportHeight {
		position = bounds.Y + bounds.Height - p.viewportHeight
	}
	p.scrollContentTo(position)
}

func (p *picker) scrollWheel(wParam uintptr) bool {
	if windowHeight <= p.viewportHeight {
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

func (p *picker) applyTheme() {
	p.themeDark = theme.Current() == theme.ModeDark
	if p.themeOverride != nil {
		p.themeDark = *p.themeOverride
	}
	p.palette = colors.ForTheme(p.themeDark)
	p.releaseBrushes()
	p.windowBrush = processBrush(p.palette.WindowBackground)
	p.surfaceBrush = processBrush(p.palette.Surface)
	nativeform.ApplyFrame(p.hwnd, p.themeDark)
	scale := p.scale()
	p.icons.Apply(p.hwnd, p.themeDark, int(32*scale+0.5), int(16*scale+0.5), false)
	for _, control := range p.controls {
		nativeform.ApplyControl(control, p.themeDark)
		pInvalidateRect.Call(uintptr(control), 0, 0)
	}
	p.applyListTheme()
	_ = p.applyStateImages()
	if p.scrollbar != nil {
		p.scrollbar.SetTheme(p.palette, p.palette.Surface)
		p.syncListScrollbar()
	}
	if p.previewScroll != nil {
		p.previewScroll.SetTheme(p.palette, p.palette.Surface)
		p.previewScroll.Sync()
	}
	if p.contentScroll != nil {
		p.contentScroll.SetTheme(p.palette, p.palette.WindowBackground)
	}
	if p.searchCue != nil {
		p.searchCue.SetTheme(p.palette.MutedText)
	}
	nativeform.ApplyTooltip(p.tooltip, p.themeDark, p.palette)
	if p.hwnd != 0 {
		pInvalidateRect.Call(uintptr(p.hwnd), 0, 0)
	}
}

func (p *picker) applyListTheme() {
	list := p.controls[idList]
	if list == 0 {
		return
	}
	pSendMessage.Call(uintptr(list), lvmSetBackgroundColor, 0, uintptr(p.palette.Surface))
	pSendMessage.Call(uintptr(list), lvmSetTextColor, 0, uintptr(p.palette.PrimaryText))
	pSendMessage.Call(uintptr(list), lvmSetTextBackground, 0, uintptr(p.palette.Surface))
	header, _, _ := pSendMessage.Call(uintptr(list), lvmGetHeader, 0, 0)
	if header != 0 {
		nativeform.ApplyControl(windows.Handle(header), p.themeDark)
		pInvalidateRect.Call(header, 0, 0)
	}
}

func processBrush(color uint32) windows.Handle {
	return createBrushForPicker(color)
}

func (p *picker) releaseBrushes() {
	for _, brush := range []windows.Handle{p.windowBrush, p.surfaceBrush} {
		if brush != 0 {
			pDeleteObject.Call(uintptr(brush))
		}
	}
	p.windowBrush, p.surfaceBrush = 0, 0
}

func (p *picker) rebuildForDPI() {
	scale := p.scale()
	newFont, _ := newFontForPicker(int32(14*scale+0.5), 400, p.options.Chinese)
	if newFont == 0 {
		return
	}
	oldFont := p.font
	p.font = newFont
	for _, control := range p.controls {
		pSendMessage.Call(uintptr(control), wmSetFont, uintptr(p.font), 1)
	}
	p.positionInWorkArea(nil)
	_ = p.applyStateImages()
	if p.scrollbar != nil {
		p.scrollbar.SetScale(scale)
		p.syncListScrollbarBounds()
		p.syncListScrollbar()
	}
	if p.previewScroll != nil {
		p.previewScroll.SetScale(scale)
		p.syncPreviewScrollbarBounds()
		p.previewScroll.Sync()
	}
	if p.searchCue != nil {
		p.searchCue.SetScale(scale)
	}
	if p.tooltip != 0 {
		pSendMessage.Call(uintptr(p.tooltip), ttmSetMaxTipWidth, 0, uintptr(int(360*scale)))
	}
	if oldFont != 0 {
		pDeleteObject.Call(uintptr(oldFont))
	}
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
	case wmActivate:
		if uint16(wParam) != 0 && !p.captureHost && shouldAutoRefreshProcessPicker(p.lastSnapshot, time.Now(), p.lastScanDuration, p.loading) {
			p.startLoad(processLoadAutomatic)
		}
	case wmClose:
		p.destroy()
		return 0
	case wmCommand:
		p.handleCommand(uint16(wParam), uint16(wParam>>16))
		return 0
	case wmDrawItem:
		if p.drawOwnerItem((*drawItem)(nativeform.MessagePointer(lParam))) {
			return 1
		}
	case wmNotify:
		p.handleNotify(nativeform.MessagePointer(lParam))
		return 0
	case wmTimer:
		if wParam == searchTimerID {
			pKillTimer.Call(uintptr(hwnd), searchTimerID)
			p.applyFilter()
		}
		return 0
	case wmMouseWheel:
		if p.scrollWheel(wParam) {
			return 0
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
		color := p.palette.PrimaryText
		control := windows.Handle(lParam)
		if control == p.controls[idHelper] || control == p.controls[idStatus] || control == p.controls[idPrivacy] || control == p.controls[idPreviewTitle] {
			color = p.palette.SecondaryText
		}
		pSetTextColor.Call(wParam, uintptr(color))
		backgroundColor := p.palette.WindowBackground
		brush := p.windowBrush
		if lParam == uintptr(p.controls[idEmpty]) {
			backgroundColor = p.palette.Surface
			brush = p.surfaceBrush
		}
		pSetBkColor.Call(wParam, uintptr(backgroundColor))
		pSetBkMode.Call(wParam, opaque)
		return uintptr(brush)
	case wmCtlColorEdit, wmCtlColorList:
		pSetTextColor.Call(wParam, uintptr(p.palette.PrimaryText))
		pSetBkColor.Call(wParam, uintptr(p.palette.Surface))
		return uintptr(p.surfaceBrush)
	case wmSettingChange, wmSysColorChange, wmThemeChanged:
		p.applyTheme()
		return 0
	case wmDpiChanged:
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
		return 0
	case wmDestroy:
		p.releaseResources()
		return 0
	}
	result, _, _ := pDefWindowProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return result
}
