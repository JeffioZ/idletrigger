package automationpanel

import (
	"golang.org/x/sys/windows"

	"github.com/JeffioZ/idletrigger/internal/ui/nativeform"
)

func (p *panel) beginRebuild() {
	p.rebuildSuspended = false
	if p.hwnd == 0 {
		return
	}
	// WM_SETREDRAW changes WS_VISIBLE in DefWindowProc. Suspending every child
	// therefore makes EndRebuild accidentally show controls belonging to the
	// other view. Suspend only an already-visible top-level window; a window
	// still under construction needs no batching because it has no visible
	// frame yet.
	if visible, _, _ := pIsWindowVisible.Call(uintptr(p.hwnd)); visible != 0 {
		pSendMessage.Call(uintptr(p.hwnd), wmSetRedraw, 0, 0)
		p.rebuildSuspended = true
	}
}

func (p *panel) endRebuild() {
	if p.hwnd == 0 {
		return
	}
	if !p.rebuildSuspended {
		return
	}
	p.rebuildSuspended = false
	pSendMessage.Call(uintptr(p.hwnd), wmSetRedraw, 1, 0)
	// DefWindowProc clears WS_VISIBLE while redraw is suspended. Restore that
	// style without moving, resizing or activating the dialog before the one
	// final repaint. This keeps both live transitions and PrintWindow captures
	// on a fully initialized frame.
	pSetWindowPos.Call(uintptr(p.hwnd), 0, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoZOrder|swpNoActivate|swpShowWindow)
	pRedrawWindow.Call(uintptr(p.hwnd), 0, 0, 0x0001|0x0080|0x0100|0x0400)
}

func (p *panel) setChoiceOptions(id uint16, labels []string) {
	p.closeChoice(false)
	choice := &choiceField{labels: append([]string(nil), labels...)}
	p.choices[id] = choice
	if len(labels) > 0 {
		p.selectCombo(id, 0)
	}
}

func (p *panel) toggleChoice(id uint16) {
	if p.choiceOpen == id {
		p.closeChoice(true)
		return
	}
	p.openChoice(id)
}

func (p *panel) openChoice(id uint16) {
	p.closeChoice(false)
	choice := p.choices[id]
	if choice == nil || len(choice.labels) == 0 || p.controls[id] == 0 {
		return
	}
	items := make([]nativeform.ChoicePopupItem, 0, len(choice.labels)+2)
	if id == idAction {
		items = append(items, nativeform.ChoicePopupItem{Label: p.t("automation_action_group_state"), Value: -1, Header: true})
	}
	for index, label := range choice.labels {
		if id == idAction && index == 4 {
			items = append(items, nativeform.ChoicePopupItem{Label: p.t("automation_action_group_system"), Value: -1, Header: true})
		}
		items = append(items, nativeform.ChoicePopupItem{Label: label, Value: index})
	}
	p.choiceOpen = id
	popup, err := nativeform.ShowChoicePopup(nativeform.ChoicePopupOptions{
		Owner: p.hwnd, Anchor: p.controls[id], Font: p.font, SelectedFont: p.sectionFont, Palette: p.palette, Dark: p.themeDark,
		Scale: p.scale(), Selected: choice.selected, MaxVisible: 6, Items: items,
		OnSelect: func(index int) {
			p.choicePopup = nil
			p.choiceOpen = 0
			p.selectCombo(id, index)
			p.handleChoiceChanged(id)
			pSetFocus.Call(uintptr(p.controls[id]))
		},
		OnClose: func() {
			p.choicePopup = nil
			p.choiceOpen = 0
			pInvalidateRect.Call(uintptr(p.controls[id]), 0, 0)
		},
	})
	if err != nil {
		p.choiceOpen = 0
		return
	}
	p.choicePopup = popup
	pInvalidateRect.Call(uintptr(p.controls[id]), 0, 0)
}

func (p *panel) closeChoice(returnFocus bool) {
	openID := p.choiceOpen
	popup := p.choicePopup
	if openID == 0 && popup == nil {
		return
	}
	p.choiceOpen = 0
	p.choicePopup = nil
	if popup != nil {
		popup.Close()
	}
	if p.controls[openID] != 0 {
		pInvalidateRect.Call(uintptr(p.controls[openID]), 0, 0)
		if returnFocus {
			pSetFocus.Call(uintptr(p.controls[openID]))
		}
	}
}

func (p *panel) drawStyledOwnerItem(value *drawItem) bool {
	if value == nil || value.HDC == 0 {
		return false
	}
	bounds := nativeform.Rect{Left: value.Rect.Left, Top: value.Rect.Top, Right: value.Rect.Right, Bottom: value.Rect.Bottom}
	painted := nativeform.DrawBuffered(value.HDC, bounds, func(dc windows.Handle, local nativeform.Rect) {
		buffered := *value
		buffered.HDC = dc
		buffered.Rect = rect{Left: local.Left, Top: local.Top, Right: local.Right, Bottom: local.Bottom}
		p.drawStyledOwnerItemDirect(&buffered)
	})
	if painted {
		return true
	}
	return p.drawStyledOwnerItemDirect(value)
}

func (p *panel) drawStyledOwnerItemDirect(value *drawItem) bool {
	if value == nil || value.HDC == 0 {
		return false
	}
	id := uint16(value.CtlID)
	bounds := nativeform.Rect{Left: value.Rect.Left, Top: value.Rect.Top, Right: value.Rect.Right, Bottom: value.Rect.Bottom}
	scale := int32(p.scale() + 0.5)
	if scale < 1 {
		scale = 1
	}
	radius := int32(6*p.scale() + 0.5)
	if radius < 3 {
		radius = 3
	}
	switch id {
	case idListSurface:
		nativeform.DrawSurface(value.HDC, bounds, p.palette, p.palette.WindowBackground, p.palette.Surface, p.palette.Border, radius)
		return true
	}
	if fieldID, ok := p.surfaceFields[id]; ok {
		state := p.interaction.State(p.controls[fieldID])
		nativeform.DrawField(value.HDC, bounds, p.palette, p.palette.WindowBackground, nativeform.ControlState{
			Hovered: state.Hovered, Focused: state.Focused, Disabled: !p.controlEnabled(fieldID),
		}, radius)
		return true
	}
	control := p.controls[id]
	if control == 0 {
		return false
	}
	interaction := p.interaction.State(control)
	state := nativeform.ControlState{
		Hovered:  interaction.Hovered,
		Pressed:  interaction.Pressed || value.ItemState&odsSelected != 0,
		Focused:  interaction.Focused || value.ItemState&odsFocus != 0,
		Disabled: value.ItemState&odsDisabled != 0 || !p.controlEnabled(id),
	}
	if _, ok := p.choices[id]; ok {
		state.Open = p.choiceOpen == id
		nativeform.DrawChoice(value.HDC, bounds, p.font, p.labels[id], p.palette, p.palette.WindowBackground, state, radius, scale)
		return true
	}
	if id == idKeepScreen {
		state.Active = p.checked(id)
		nativeform.DrawCheckbox(value.HDC, bounds, p.font, p.labels[id], p.palette, p.palette.WindowBackground, state, scale)
		return true
	}
	if id == idProcessInfo {
		nativeform.DrawButton(value.HDC, bounds, p.font, p.labels[id], p.palette, p.palette.WindowBackground, state, (bounds.Bottom-bounds.Top)/2, false)
		return true
	}
	if id >= idWeekdayBase && id < idWeekdayBase+uint16(len(editorWeekdays)) {
		state.Active = p.checked(id)
		nativeform.DrawButton(value.HDC, bounds, p.font, p.labels[id], p.palette, p.palette.WindowBackground, state, radius, false)
		return true
	}
	nativeform.DrawButton(value.HDC, bounds, p.font, p.labels[id], p.palette, p.palette.WindowBackground, state, radius, false)
	return true
}

func (p *panel) controlEnabled(id uint16) bool {
	control := p.controls[id]
	if control == 0 {
		return false
	}
	enabled, _, _ := pIsWindowEnabled.Call(uintptr(control))
	return enabled != 0
}

func (p *panel) invalidateControl(id uint16) {
	if control := p.controls[id]; control != 0 {
		pInvalidateRect.Call(uintptr(control), 0, 0)
	}
}
