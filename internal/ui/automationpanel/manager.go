package automationpanel

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/JeffioZ/idletrigger/internal/ui/nativeform"
)

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
