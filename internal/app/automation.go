package app

import (
	"fmt"
	"path/filepath"

	"github.com/JeffioZ/idletrigger/internal/automation"
	"github.com/JeffioZ/idletrigger/internal/config"
	"github.com/JeffioZ/idletrigger/internal/feature/autorules"
	"github.com/JeffioZ/idletrigger/internal/i18n"
	mylog "github.com/JeffioZ/idletrigger/internal/logging"
	"github.com/JeffioZ/idletrigger/internal/platform/windows/systemaction"
	"github.com/JeffioZ/idletrigger/internal/ui/actionwarning"
	"github.com/JeffioZ/idletrigger/internal/ui/automationpanel"
	"github.com/JeffioZ/idletrigger/internal/ui/controlpanel"
	"github.com/JeffioZ/idletrigger/internal/ui/processpicker"
	"github.com/JeffioZ/idletrigger/internal/ui/trayicon"
)

func (s *runtimeState) showAutomationManager() {
	rules := append([]automation.Rule(nil), s.cfg.AutomationRules...)
	state := automationpanel.State{
		Rules:    rules,
		Chinese:  i18n.ResolveLanguage(s.lang) == "zh-CN",
		Owner:    controlpanel.WindowHandle(),
		NextText: nextAutomationText(s.autoState, s.lang),
	}
	lang := s.lang
	trayicon.Post(func() {
		err := automationpanel.Show(state, func(rules []automation.Rule) {
			s.post(func() {
				normalized := automation.NormalizeRules(rules)
				if err := automation.ValidateRules(normalized); err != nil {
					mylog.Info("Automation rules rejected: %v", err)
					return
				}
				s.cfg.AutomationRules = normalized
				s.restartAutomation()
				s.saveConfig()
				s.refreshControlPanelAutomationStatus()
			})
		}, func(key string) string { return i18n.T(lang, key) })
		if err != nil {
			mylog.Info("Automation manager open failed: %v", err)
		}
	})
}

func hideAutomationUI() {
	automationpanel.Hide()
	processpicker.Hide()
	actionwarning.Hide()
}

func (s *runtimeState) startAutomation() {
	s.stopAutomation()
	s.autoState = autorules.EffectiveState{IdleMinutes: automation.DefaultIdleMinutes}
	if !s.cfg.AutomationEnabled || len(s.cfg.AutomationRules) == 0 {
		return
	}
	configPath, err := config.Path()
	if err == nil {
		s.automationStatePath = filepath.Join(filepath.Dir(configPath), "IdleTrigger.state.json")
	}
	runtimeState, err := automation.LoadRuntimeState(s.automationStatePath)
	if err != nil {
		mylog.Info("Automation state load failed; continuing without history: %v", err)
		runtimeState = automation.RuntimeState{LastOccurrences: make(map[string]string)}
	}
	runner := autorules.New(s.cfg.AutomationRules, runtimeState.LastOccurrences, autorules.Callbacks{
		OnState: func(state autorules.EffectiveState) {
			s.post(func() {
				s.autoState = state
				s.reconcileRuntime()
				s.updateIcon()
				s.refreshControlPanelAutomationStatus()
			})
		},
		OnEvent: func(event autorules.Event) {
			s.post(func() { s.enqueueAutomationEvent(event) })
		},
		OnCheckpoint: func(last map[string]string) {
			if s.automationStatePath == "" {
				return
			}
			if err := automation.SaveRuntimeState(s.automationStatePath, automation.RuntimeState{LastOccurrences: last}); err != nil {
				mylog.Info("Automation state save failed: %v", err)
			}
		},
		OnError: func(err error) {
			mylog.Info("Automation process scan failed: %v", err)
		},
	})
	s.autoRunner = runner
	runner.Start()
	mylog.Info("Automation started: enabled_rules=%d", s.enabledAutomationCount())
}

func (s *runtimeState) stopAutomation() {
	if s.autoRunner != nil {
		s.autoRunner.Stop()
		s.autoRunner = nil
		mylog.Info("Automation stopped")
	}
	s.autoState = autorules.EffectiveState{IdleMinutes: automation.DefaultIdleMinutes}
	s.automationEvents = nil
	s.automationWarningOpen = false
	actionwarning.Hide()
}

func (s *runtimeState) restartAutomation() {
	s.startAutomation()
	s.reconcileRuntime()
	s.updateIcon()
}

func (s *runtimeState) enabledAutomationCount() int {
	if !s.cfg.AutomationEnabled {
		return 0
	}
	count := 0
	for _, rule := range s.cfg.AutomationRules {
		if rule.Enabled {
			count++
		}
	}
	return count
}

func (s *runtimeState) automationOverviewText() string {
	configured := len(s.cfg.AutomationRules)
	if configured == 0 {
		return i18n.T(s.lang, "automation_overview_empty")
	}
	if !s.cfg.AutomationEnabled {
		return fmt.Sprintf(i18n.T(s.lang, "automation_overview_paused"), configured)
	}
	enabled := s.enabledAutomationCount()
	if s.autoState.NextOccurrence.IsZero() {
		return fmt.Sprintf(i18n.T(s.lang, "automation_overview_enabled"), enabled)
	}
	return fmt.Sprintf(i18n.T(s.lang, "automation_overview_next"), enabled, s.autoState.NextOccurrence.Format("2006-01-02 15:04"))
}

func (s *runtimeState) enqueueAutomationEvent(event autorules.Event) {
	if s.exiting.Load() {
		return
	}
	s.automationEvents = append(s.automationEvents, event)
	s.showNextAutomationEvent()
}

func (s *runtimeState) showNextAutomationEvent() {
	if s.automationWarningOpen || len(s.automationEvents) == 0 || s.exiting.Load() {
		return
	}
	event := s.automationEvents[0]
	s.automationEvents = s.automationEvents[1:]
	s.automationWarningOpen = true
	seconds := event.WarningSeconds
	if seconds < 0 {
		seconds = 0
	}
	actionName := i18n.T(s.lang, automationActionKey(event.Action))
	actionwarning.SetLanguage(i18n.ResolveLanguage(s.lang) == "zh-CN")
	actionwarning.Show(actionwarning.Options{
		Title:       i18n.T(s.lang, "automation_warning_title"),
		Seconds:     seconds,
		CancelText:  i18n.T(s.lang, "automation_cancel_once"),
		ExecuteText: i18n.T(s.lang, "automation_execute_now"),
		Body: func(remaining int) string {
			return fmt.Sprintf(i18n.T(s.lang, "automation_warning_body"), event.RuleName, actionName, remaining)
		},
		OnCancel: func() {
			s.post(func() {
				mylog.Info("Automation occurrence cancelled: rule=%s occurrence=%s", event.RuleID, event.Occurrence)
				s.automationWarningOpen = false
				s.finishOneTimeRule(event.RuleID)
				s.showNextAutomationEvent()
			})
		},
		OnExecute: func() {
			s.post(func() {
				s.automationWarningOpen = false
				s.finishOneTimeRule(event.RuleID)
				if queued := len(s.automationEvents); queued > 0 {
					mylog.Info("Automation events cleared after a system action was confirmed: count=%d", queued)
					s.automationEvents = nil
				}
				if err := s.executeAutomationAction(event.Action); err != nil {
					mylog.Info("Automation action failed: rule=%s action=%s error=%v", event.RuleID, event.Action, err)
					s.showError(automationActionKey(event.Action), err)
				} else {
					mylog.Info("Automation action accepted: rule=%s action=%s", event.RuleID, event.Action)
				}
			})
		},
	})
}

func (s *runtimeState) finishOneTimeRule(ruleID string) {
	changed := false
	for index := range s.cfg.AutomationRules {
		if s.cfg.AutomationRules[index].ID == ruleID && s.cfg.AutomationRules[index].Trigger == automation.TriggerOnce && s.cfg.AutomationRules[index].Enabled {
			s.cfg.AutomationRules[index].Enabled = false
			changed = true
		}
	}
	if changed {
		s.saveConfig()
		// Do not restart the runner from inside its event callback; the consumed
		// occurrence is already checkpointed, and this rule cannot fire again.
	}
}

func (s *runtimeState) executeAutomationAction(action automation.Action) error {
	switch action {
	case automation.ActionLock:
		return systemaction.Lock()
	case automation.ActionSleep:
		return s.executeAction(config.ActionSleep)
	case automation.ActionHibernate:
		return s.executeAction(config.ActionHibernate)
	case automation.ActionShutdown:
		return s.executeAction(config.ActionShutdown)
	case automation.ActionRestart:
		return systemaction.Restart()
	default:
		return fmt.Errorf("unsupported automatic event action %q", action)
	}
}

func automationActionKey(action automation.Action) string {
	switch action {
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
	default:
		return "menu_more"
	}
}

func nextAutomationText(state autorules.EffectiveState, lang string) string {
	if state.NextOccurrence.IsZero() {
		return i18n.T(lang, "automation_no_upcoming")
	}
	return fmt.Sprintf(i18n.T(lang, "automation_next_format"), state.NextOccurrence.Format("2006-01-02 15:04"))
}
