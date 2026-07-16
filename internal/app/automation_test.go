package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JeffioZ/idletrigger/internal/automation"
	"github.com/JeffioZ/idletrigger/internal/config"
	"github.com/JeffioZ/idletrigger/internal/ui/automationpanel"
)

func TestPostUntilStopsWaitingOnFullQueue(t *testing.T) {
	state := runtimeState{requestCh: make(chan runtimeRequest, 1)}
	state.requestCh <- runtimeRequest{fn: func() string { return "" }}
	cancel := make(chan struct{})
	done := make(chan bool, 1)
	go func() { done <- state.postUntil(cancel, func() {}) }()
	close(cancel)
	select {
	case posted := <-done:
		if posted {
			t.Fatal("cancelled callback was posted to a saturated queue")
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled callback remained blocked on a saturated queue")
	}
}

func TestAutomationSaveRejectsStaleManagerRevision(t *testing.T) {
	originalSave := saveConfigAtRevision
	defer func() { saveConfigAtRevision = originalSave }()
	called := false
	saveConfigAtRevision = func(config.Config, string) (string, error) {
		called = true
		return "", nil
	}

	cfg := config.DefaultConfig()
	cfg.SourceRevision = "latest"
	cfg.AutomationRules = []automation.Rule{{
		ID: "current", Enabled: true, Action: automation.ActionLock,
		Trigger: automation.TriggerDaily, Time: "12:00", WarningSeconds: 60,
	}}
	state := runtimeState{cfg: cfg, lang: "en"}
	result := state.saveAutomationRules(automationpanel.SaveRequest{
		BaseRevision: "stale",
		Rules: []automation.Rule{{
			ID: "stale", Enabled: true, Action: automation.ActionLock,
			Trigger: automation.TriggerDaily, Time: "13:00", WarningSeconds: 60,
		}},
	})
	if result.Error == "" || called {
		t.Fatalf("stale save result = %+v, persistence called = %v", result, called)
	}
	if len(state.cfg.AutomationRules) != 1 || state.cfg.AutomationRules[0].ID != "current" {
		t.Fatalf("stale manager overwrote current rules: %+v", state.cfg.AutomationRules)
	}
	if result.State.Revision != "latest" {
		t.Fatalf("result revision = %q, want latest", result.State.Revision)
	}
}

func TestAutomationSaveFailureIsReturnedToManager(t *testing.T) {
	originalSave := saveConfigAtRevision
	defer func() { saveConfigAtRevision = originalSave }()
	wantErr := errors.New("disk denied")
	saveConfigAtRevision = func(config.Config, string) (string, error) { return "", wantErr }

	cfg := config.DefaultConfig()
	cfg.SourceRevision = "current"
	state := runtimeState{cfg: cfg, lang: "en"}
	result := state.saveAutomationRules(automationpanel.SaveRequest{
		BaseRevision: "current",
		Rules: []automation.Rule{{
			ID: "new", Enabled: true, Action: automation.ActionLock,
			Trigger: automation.TriggerDaily, Time: "13:00", WarningSeconds: 60,
		}},
	})
	if !strings.Contains(result.Error, wantErr.Error()) {
		t.Fatalf("save error was not returned to manager: %+v", result)
	}
	if len(state.cfg.AutomationRules) != 0 || state.cfg.SourceRevision != "current" {
		t.Fatalf("failed save changed runtime config: %+v", state.cfg)
	}
}

func TestOneTimeDisablePublishesLatestManagerState(t *testing.T) {
	originalSave := saveConfigAtRevision
	originalPost := postAutomationPanelState
	defer func() {
		saveConfigAtRevision = originalSave
		postAutomationPanelState = originalPost
	}()
	saveConfigAtRevision = func(config.Config, string) (string, error) { return "after-disable", nil }
	var published automationpanel.State
	postAutomationPanelState = func(state automationpanel.State) { published = state }

	cfg := config.DefaultConfig()
	cfg.SourceRevision = "before-disable"
	cfg.AutomationEnabled = false
	cfg.AutomationRules = []automation.Rule{{
		ID: "once", Name: "Once", Enabled: true, Action: automation.ActionLock,
		Trigger: automation.TriggerOnce, Date: "2026-07-16", Time: "12:00", WarningSeconds: 60,
	}}
	state := runtimeState{cfg: cfg, lang: "en"}
	state.finishOneTimeRule("once")

	if state.cfg.AutomationRules[0].Enabled || state.cfg.SourceRevision != "after-disable" {
		t.Fatalf("one-time rule was not disabled authoritatively: %+v", state.cfg.AutomationRules[0])
	}
	if len(published.Rules) != 1 || published.Rules[0].Enabled || published.Revision != "after-disable" {
		t.Fatalf("manager update = %+v", published)
	}
}
