package controlpanel

import (
	"github.com/JeffioZ/idletrigger/internal/config"
	"github.com/JeffioZ/idletrigger/internal/ui/nativeform"
)

func (p *panel) openChoice(id uint16) {
	if p.disabled[id] {
		return
	}
	if p.choice.openID == id && p.choice.popup != nil && p.choice.popup.IsOpen() {
		p.closeChoice(true)
		return
	}
	p.closeChoice(false)
	p.closeQuickMenu()
	p.closeLanguageMenu()
	options := p.choice.options[id]
	if len(options) == 0 || p.controls[id] == 0 {
		return
	}
	selected := p.choice.selected[id]
	if selected < 0 || selected >= len(options) {
		selected = 0
		p.choice.selected[id] = selected
	}
	items := make([]nativeform.ChoicePopupItem, len(options))
	for index, label := range options {
		items[index] = nativeform.ChoicePopupItem{Label: label, Value: index}
	}
	p.choice.serial++
	serial := p.choice.serial
	p.choice.openID = id
	popup, err := nativeform.ShowChoicePopup(nativeform.ChoicePopupOptions{
		Owner: p.hwnd, Anchor: p.controls[id], Font: p.font, SelectedFont: p.choiceSelectedFont,
		Palette: p.palette, Dark: p.themeDark, Scale: p.metrics.scale,
		Selected: selected, MaxVisible: 6, Items: items,
		KeepOpenOnReselect: true, RestoreAnchorOnCancel: true,
		OnSelect: func(index int) {
			p.applyChoice(id, index)
		},
		OnClose: func() {
			if p.choice.serial != serial {
				return
			}
			p.choice.popup = nil
			if p.choice.openID == id {
				p.choice.openID = 0
			}
			p.invalidate(id)
		},
	})
	if err != nil {
		p.choice.openID = 0
		p.invalidate(id)
		return
	}
	p.choice.popup = popup
	p.invalidate(id)
}

func (p *panel) requestChoice(id uint16, _ bool) {
	pPostMessage.Call(uintptr(p.hwnd), wmOpenChoice, uintptr(id), 0)
}

func (p *panel) closeChoice(returnFocus bool) {
	openID := p.choice.openID
	popup := p.choice.popup
	if openID == 0 && popup == nil {
		return
	}
	// Invalidate callbacks from the popup being closed before clearing the
	// local state. A delayed WM_DESTROY from an older popup must never clear a
	// newer selector.
	p.choice.serial++
	p.choice.openID = 0
	p.choice.popup = nil
	if popup != nil {
		popup.Close()
	}
	if openID != 0 {
		p.invalidate(openID)
	}
	if returnFocus && p.hwnd != 0 && openID != 0 {
		pSetFocus.Call(uintptr(p.controls[openID]))
	}
}

func (p *panel) applyChoice(owner uint16, index int) {
	options := p.choice.options[owner]
	if index < 0 || index >= len(options) || p.choice.selected[owner] == index {
		return
	}
	p.choice.selected[owner] = index
	p.labels[owner] = options[index]
	p.invalidate(owner)
	if owner == idIdleTimeout {
		p.applyTimeoutChoice(index)
	} else if owner == idIdleAction {
		p.applyActionChoice(index)
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
