package controlpanel

import (
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/JeffioZ/idletrigger/internal/config"
)

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

func (p *panel) choiceFocusContains(hwnd windows.Handle) bool {
	if hwnd == 0 || hwnd == p.hwnd {
		return hwnd != 0
	}
	if p.choice.openID != 0 && hwnd == p.controls[p.choice.openID] {
		return true
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

func (p *panel) beginChoiceOpen(id uint16) bool {
	keyboardOpen := p.choice.focusOnOpen
	p.choice.restoreFocus = keyboardOpen
	p.choice.scroll[id] = 0
	if keyboardOpen {
		p.choice.scroll[id] = p.choice.selected[id]
		p.enterKeyboardNavigation()
	}
	return keyboardOpen
}

func (p *panel) openChoice(id uint16) {
	if p.disabled[id] {
		return
	}
	if p.choice.openID == id {
		p.choice.focusOnOpen = false
		p.closeChoice(false)
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
	keyboardOpen := p.beginChoiceOpen(id)
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
	if keyboardOpen {
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
	rowGap := p.metrics.style.Layout.QuickMenuRowGap
	surfaceInset := p.metrics.style.Control.MenuSurfaceInset
	var client rect
	pGetClientRect.Call(uintptr(p.hwnd), uintptr(unsafe.Pointer(&client)))
	availableDown := int(float64(client.Bottom-origin.Y) / p.metrics.scale)
	availableUp := int(float64(origin.Y) / p.metrics.scale)
	maxRows := menuRowsFit(availableDown, rowH, rowGap, surfaceInset)
	if maxRows < 1 && availableUp > availableDown {
		maxRows = menuRowsFit(availableUp, rowH, rowGap, surfaceInset)
		if maxRows < 1 {
			maxRows = 1
		}
		origin.Y = int32(origin.Y) - int32(p.sc(menuHeight(maxRows, rowH, rowGap, surfaceInset))) - margin
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
	menuH := menuHeight(maxRows, rowH, rowGap, surfaceInset)
	if surface := p.controls[idChoiceSurface]; surface != 0 {
		// Keep the surface above the panel's other child controls so its border
		// is not covered by the rows below/behind it.
		pSetWindowPos.Call(uintptr(surface), 0, uintptr(origin.X), uintptr(origin.Y), uintptr(width), uintptr(p.sc(menuH)), swpShowWindow)
		pUpdateWindow.Call(uintptr(surface))
	}
	for i, optionID := range p.choice.optionIDs[id] {
		if hwnd := p.choice.optionControls[optionID]; hwnd != 0 {
			if i < start || i >= start+maxRows {
				pShowWindow.Call(uintptr(hwnd), swHide)
				continue
			}
			ox := origin.X + int32(p.sc(surfaceInset))
			oy := origin.Y + int32(p.sc(menuRowOffset(i-start, rowH, rowGap, surfaceInset)))
			pSetWindowPos.Call(uintptr(hwnd), 0, uintptr(ox), uintptr(oy), uintptr(width-p.sc(2*surfaceInset)), uintptr(p.sc(rowH)), swpShowWindow)
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
	action, ok := config.IdleActionAt(index)
	if !ok {
		return
	}
	p.idleAction = string(action)
	if p.onAction != nil {
		p.onAction(ActIdleAction, index)
	}
}
