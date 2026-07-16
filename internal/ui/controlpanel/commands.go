package controlpanel

import (
	"fmt"

	"github.com/JeffioZ/idletrigger/internal/ui/idlewarning"
)

func (p *panel) handleCommand(id uint16) {
	if p.disabled[id] {
		return
	}
	if p.handleMenuCommand(id) {
		return
	}
	action, value, ok := p.commandAction(id)
	if !ok {
		return
	}
	if actionClosesPanel(action) {
		Hide()
	}
	if p.onAction != nil {
		p.onAction(action, value)
	}
}

func (p *panel) commandAction(id uint16) (Action, int, bool) {
	if action, ok := p.toggleCommand(id); ok {
		return action, 0, true
	}
	switch id {
	case idSleep:
		return ActSleep, 0, true
	case idHibernate:
		return ActHibernate, 0, true
	case idShutdown:
		return ActShutdown, 0, true
	case idLock:
		return ActLock, 0, true
	case idRestart:
		return ActRestart, 0, true
	case idAutomation:
		return ActAutomationOpen, 0, true
	case idThemeSwitch:
		return ActSwitchTheme, 0, true
	case idThemeRepair:
		return ActRepairTheme, 0, true
	case idConfig:
		return ActConfig, 0, true
	case idProjectHome:
		return ActProjectHome, 0, true
	case idExit:
		return ActExit, 0, true
	case idTestWarning:
		p.showWarningPreview()
	}
	return 0, 0, false
}

func (p *panel) handleMenuCommand(id uint16) bool {
	switch id {
	case idQuickActions:
		p.openQuickMenu(p.keyboardNavigation)
		return true
	case idLanguage:
		p.openLanguageMenu(p.keyboardNavigation)
		return true
	case idLangEN:
		p.selectLanguage(idLangEN, 0)
		return true
	case idLangZH:
		p.selectLanguage(idLangZH, 1)
		return true
	default:
		return false
	}
}

func (p *panel) selectLanguage(id uint16, value int) {
	if p.selected[id] {
		return
	}
	p.choose(languageIDs(), id)
	p.closeLanguageMenu()
	if p.onAction != nil {
		p.onAction(ActLanguage, value)
	}
}

func (p *panel) toggleCommand(id uint16) (Action, bool) {
	var action Action
	switch id {
	case idNoSleep:
		action = ActNoSleepToggle
	case idAutomationEnabled:
		action = ActAutomationToggle
	case idIdle:
		action = ActIdleToggle
	case idIdleWarning:
		action = ActIdleWarningToggle
	case idIdleEnhanced:
		action = ActIdleEnhancedMonitorToggle
	case idTheme:
		action = ActThemeToggle
	case idBattery:
		action = ActBatteryToggle
	case idFullscreen:
		action = ActFullscreenToggle
	case idIPLocation:
		action = ActIPLocationToggle
	case idHotkeys:
		action = ActHotkeyToggle
	case idAutostart:
		action = ActAutostartToggle
	case idLogging:
		action = ActLoggingToggle
	default:
		return 0, false
	}
	p.toggle(id)
	if id == idNoSleep && p.toggles[idNoSleep] {
		p.setToggle(idIdle, false)
	}
	if id == idIdle && p.toggles[idIdle] {
		p.setToggle(idNoSleep, false)
	}
	if id == idNoSleep || id == idIdle || id == idTheme {
		p.applyDependentStates()
	}
	return action, true
}

func (p *panel) showWarningPreview() {
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
}

func actionClosesPanel(action Action) bool {
	return action <= ActRestart || action == ActConfig || action == ActExit
}
