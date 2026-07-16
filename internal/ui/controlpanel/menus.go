package controlpanel

import (
	"golang.org/x/sys/windows"
	"unsafe"
)

func (p *panel) openQuickMenu(focusFirst bool) {
	if p.choice.openID != 0 {
		p.closeChoice(false)
	}
	if p.quickMenuOpen {
		p.closeQuickMenu()
		return
	}
	p.quickMenuOpen = true
	p.closeLanguageMenu()
	p.showFixedMenu(idQuickMenu, quickActionIDs())
	if focusFirst {
		pSetFocus.Call(uintptr(p.controls[quickActionIDs()[0]]))
	}
}

func (p *panel) clearTrackedHover(ids []uint16) {
	for _, id := range ids {
		if p.hoverID == id {
			p.hoverID = 0
			p.invalidate(id)
			return
		}
	}
}

func (p *panel) closeQuickMenu() {
	p.closeFixedMenu(&p.quickMenuOpen, idQuickMenu, idQuickActions, quickActionIDs())
}

func (p *panel) openLanguageMenu(focusFirst bool) {
	if p.choice.openID != 0 {
		p.closeChoice(false)
	}
	if p.languageMenuOpen {
		p.closeLanguageMenu()
		return
	}
	p.closeQuickMenu()
	p.languageMenuOpen = true
	p.showFixedMenu(idLanguageMenu, languageIDs())
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

func (p *panel) closeLanguageMenu() {
	p.closeFixedMenu(&p.languageMenuOpen, idLanguageMenu, idLanguage, languageIDs())
}

func (p *panel) showFixedMenu(surfaceID uint16, optionIDs []uint16) {
	for _, id := range append([]uint16{surfaceID}, optionIDs...) {
		if hwnd := p.controls[id]; hwnd != 0 {
			pSetWindowPos.Call(uintptr(hwnd), 0, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate|swpShowWindow)
		}
	}
}

func (p *panel) closeFixedMenu(open *bool, surfaceID, triggerID uint16, optionIDs []uint16) {
	if !*open {
		return
	}
	*open = false
	p.clearTrackedHover(append([]uint16{triggerID}, optionIDs...))
	p.invalidate(triggerID)
	for _, id := range append([]uint16{surfaceID}, optionIDs...) {
		if hwnd := p.controls[id]; hwnd != 0 {
			pShowWindow.Call(uintptr(hwnd), swHide)
		}
	}
}

// closeOpenMenus handles an in-panel click outside the currently open menu.
func (p *panel) closeOpenMenus() {
	p.closeQuickMenu()
	p.closeLanguageMenu()
	p.closeChoice(false)
}

func (p *panel) menuClickKeepsOpen(id uint16) bool {
	if p.quickMenuOpen && (id == idQuickActions || id == idQuickMenu || containsQuickAction(id)) {
		return true
	}
	if p.languageMenuOpen && (id == idLanguage || id == idLanguageMenu || containsLanguageOption(id)) {
		return true
	}
	if p.choice.openID == 0 {
		return false
	}
	return id == p.choice.openID
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

func (p *panel) focusFixedMenuOption(ids []uint16, current uint16, delta int) {
	for index, id := range ids {
		if id == current {
			next := (index + delta + len(ids)) % len(ids)
			pSetFocus.Call(uintptr(p.controls[ids[next]]))
			return
		}
	}
}
