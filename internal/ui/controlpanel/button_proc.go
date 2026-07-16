package controlpanel

import "golang.org/x/sys/windows"

func buttonWndProc(hwnd windows.Handle, msg uint32, wp, lp uintptr) uintptr {
	p := panelForButton(hwnd)
	var old uintptr
	if p != nil {
		old = p.oldButtonProc[hwnd]
		id := p.controlID(hwnd)
		if handled, result := p.handleButtonMessage(hwnd, id, msg, wp); handled {
			return result
		}
	}
	if old != 0 {
		result, _, _ := pCallWindowProc.Call(old, uintptr(hwnd), uintptr(msg), wp, lp)
		return result
	}
	result, _, _ := pDefWindowProc.Call(uintptr(hwnd), uintptr(msg), wp, lp)
	return result
}

func (p *panel) handleButtonMessage(hwnd windows.Handle, id uint16, msg uint32, wp uintptr) (bool, uintptr) {
	switch msg {
	case wmMouseMove:
		if id == idProjectHome {
			p.setProjectHomeCursor(true)
		}
		p.setHover(hwnd)
	case wmMouseLeave:
		p.clearHover(hwnd)
		if id == idProjectHome {
			p.setProjectHomeCursor(false)
		}
	case wmSetCursor:
		if id == idProjectHome {
			p.setProjectHomeCursor(true)
			return true, 1
		}
	case wmLButtonDown:
		p.leaveKeyboardNavigation()
		if !p.menuClickKeepsOpen(id) {
			p.closeOpenMenus()
		}
	case wmKeyDown:
		return p.handleButtonKeyDown(id, wp)
	case wmSysKeyDown:
		if (wp == vkDown || wp == vkF4) && (id == idIdleTimeout || id == idIdleAction) {
			p.enterKeyboardNavigation()
			p.requestChoice(id, true)
			return true, 0
		}
	}
	return false, 0
}

func (p *panel) handleButtonKeyDown(id uint16, key uintptr) (bool, uintptr) {
	switch key {
	case vkUp, vkDown, vkHome, vkEnd, vkReturn, vkSpace, vkF4, vkEscape:
		// Programmatic SetFocus is not a modality change. Actual keyboard
		// navigation is the only path that enables focus-visible drawing.
		p.enterKeyboardNavigation()
	}
	if (id == idIdleTimeout || id == idIdleAction) && isChoiceOpenKey(key) {
		p.requestChoice(id, true)
		return true, 0
	}
	if containsQuickAction(id) && p.handleQuickActionKey(id, key) {
		return true, 0
	}
	if containsLanguageOption(id) && p.handleLanguageKey(id, key) {
		return true, 0
	}
	return false, 0
}

func isChoiceOpenKey(key uintptr) bool {
	switch key {
	case vkReturn, vkSpace, vkUp, vkDown, vkF4:
		return true
	default:
		return false
	}
}

func (p *panel) handleQuickActionKey(id uint16, key uintptr) bool {
	switch key {
	case vkUp:
		p.focusFixedMenuOption(quickActionIDs(), id, -1)
	case vkDown:
		p.focusFixedMenuOption(quickActionIDs(), id, 1)
	case vkEscape:
		p.closeQuickMenu()
		pSetFocus.Call(uintptr(p.controls[idQuickActions]))
	default:
		return false
	}
	return true
}

func (p *panel) handleLanguageKey(id uint16, key uintptr) bool {
	switch key {
	case vkUp:
		p.focusFixedMenuOption(languageIDs(), id, -1)
	case vkDown:
		p.focusFixedMenuOption(languageIDs(), id, 1)
	case vkEscape:
		p.closeLanguageMenu()
		pSetFocus.Call(uintptr(p.controls[idLanguage]))
	default:
		return false
	}
	return true
}
