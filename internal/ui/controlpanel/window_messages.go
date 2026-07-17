package controlpanel

import (
	"fmt"
	mylog "github.com/JeffioZ/idletrigger/internal/logging"
	"github.com/JeffioZ/idletrigger/internal/ui/font"
	"github.com/JeffioZ/idletrigger/internal/ui/nativeform"
	"golang.org/x/sys/windows"
	"unsafe"
)

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

func (p *panel) setProjectHomeCursor(onText bool) {
	const (
		idcArrow = 32512
		idcHand  = 32649
	)
	cursorID := uintptr(idcArrow)
	if onText {
		cursorID = idcHand
	}
	if cursor, _, _ := pLoadCursor.Call(0, cursorID); cursor != 0 {
		pSetCursor.Call(cursor)
	}
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

func cursorWorkArea(fallback windows.Handle) (rect, bool) {
	var cursor struct{ X, Y int32 }
	monitor := uintptr(0)
	if result, _, _ := pGetCursorPos.Call(uintptr(unsafe.Pointer(&cursor))); result != 0 {
		anchor := rect{Left: cursor.X, Top: cursor.Y, Right: cursor.X + 1, Bottom: cursor.Y + 1}
		monitor, _, _ = pMonitorFromRect.Call(uintptr(unsafe.Pointer(&anchor)), monitorNearest)
	}
	if monitor == 0 && fallback != 0 {
		monitor, _, _ = pMonitorFromWindow.Call(uintptr(fallback), monitorNearest)
	}
	if monitor == 0 {
		return rect{}, false
	}
	info := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if result, _, _ := pGetMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info))); result == 0 {
		return rect{}, false
	}
	return info.Work, info.Work.Right > info.Work.Left && info.Work.Bottom > info.Work.Top
}

func (p *panel) position(style, exStyle uint32) error {
	clientWidth, clientHeight := p.sc(p.metrics.style.Layout.PanelWidth), p.sc(p.clientH)
	windowDPI := uint32(dpiForWindow(p.hwnd)*96 + 0.5)
	width, height, err := nativeform.WindowSizeForClient(clientWidth, clientHeight, uintptr(style), uintptr(exStyle), windowDPI)
	if err != nil {
		return fmt.Errorf("calculate control panel bounds: %w", err)
	}
	work, ok := cursorWorkArea(p.hwnd)
	if !ok {
		return fmt.Errorf("locate control panel monitor work area")
	}
	x, y := panelOrigin(work, width, height, int32(p.sc(16)))
	insertAfter := ^uintptr(0)
	if p.developerCapturePanel || p.captureHost {
		insertAfter = 0
	}
	// Keep placement and visibility as two distinct commits. Combining them
	// with SWP_SHOWWINDOW can let DWM compose the newly visible cold-start frame
	// at CreateWindowEx's temporary coordinates before the final position is
	// applied. The caller explicitly shows the fully positioned window.
	if result, _, callErr := pSetWindowPos.Call(uintptr(p.hwnd), insertAfter, uintptr(x), uintptr(y), uintptr(width), uintptr(height), swpNoActivate); result == 0 {
		return fmt.Errorf("position control panel: %w", callErr)
	}
	return nil
}

func wndProc(hwnd windows.Handle, msg uint32, wp, lp uintptr) uintptr {
	p := panelFor(hwnd)
	if p != nil {
		switch msg {
		case wmClose:
			Hide()
			return 0
		case wmActivate:
			if uint16(wp) == waInactive && !nativeform.IsChoicePopupOwnedBy(windows.Handle(lp), p.hwnd) {
				p.closeOpenMenus()
			}
		case wmNcLButtonDown:
			p.closeOpenMenus()
		case wmLButtonDown:
			p.leaveKeyboardNavigation()
			p.closeOpenMenus()
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
			if err := p.position(p.style, p.exStyle); err != nil {
				mylog.Info("Control panel DPI placement failed: %v", err)
			}
			mylog.Info("UI font: surface=popup rebuilt reason=dpi-change dpi=%d face=%q client_px=%dx%d", int(p.metrics.scale*96+0.5), p.fontChoice.Face, p.sc(p.metrics.style.Layout.PanelWidth), p.sc(p.clientH))
			return 0
		case wmCommand:
			id, notification := uint16(wp), uint16(wp>>16)
			if notification == bnClicked {
				if id == idIdleTimeout || id == idIdleAction {
					p.openChoice(id)
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
	p.metrics = newPanelMetrics(p.metrics.style, newScale)
	p.fontChoice = font.Choice{}
	newFont := p.makeFont(p.metrics.style.Fonts.BodySize, p.metrics.style.Fonts.BodyWeight)
	newSectionFont := p.makeFont(p.metrics.style.Fonts.SectionSize, p.metrics.style.Fonts.SectionWeight)
	newSubtitleFont := p.makeFont(p.metrics.style.Fonts.SubtitleSize, p.metrics.style.Fonts.SubtitleWeight)
	newChoiceSelectedFont := p.makeFont(p.metrics.style.Fonts.BodySize, p.metrics.style.Fonts.SectionWeight)
	if newFont == 0 || newSectionFont == 0 || newSubtitleFont == 0 || newChoiceSelectedFont == 0 {
		for _, font := range []windows.Handle{newFont, newSectionFont, newSubtitleFont, newChoiceSelectedFont} {
			if font != 0 {
				pDeleteObject.Call(uintptr(font))
			}
		}
		p.metrics, p.fontChoice = oldMetrics, oldChoice
		return
	}
	oldFont, oldSection, oldSubtitle, oldChoiceSelected := p.font, p.sectionFont, p.subtitleFont, p.choiceSelectedFont
	p.font, p.sectionFont, p.subtitleFont, p.choiceSelectedFont = newFont, newSectionFont, newSubtitleFont, newChoiceSelectedFont
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
	for _, font := range []windows.Handle{oldFont, oldSection, oldSubtitle, oldChoiceSelected} {
		if font != 0 {
			pDeleteObject.Call(uintptr(font))
		}
	}
}

type drawItemPointer *drawItem

func drawItemFromLParam(lp uintptr) *drawItem {
	return *(*drawItemPointer)(unsafe.Pointer(&lp))
}
