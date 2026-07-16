package automationpanel

import (
	"testing"

	"github.com/JeffioZ/idletrigger/internal/automation"
)

func TestChoiceFocusNotificationDoesNotClosePopup(t *testing.T) {
	const bnKillFocus = 7
	p := &panel{
		choices:    map[uint16]*choiceField{idAction: {}},
		choiceOpen: idAction,
	}
	p.handleEditor(idAction, bnKillFocus)
	if p.choiceOpen != idAction {
		t.Fatal("a choice button focus notification closed the popup")
	}
}

func TestSystemActionOffersProcessLifecycleTriggers(t *testing.T) {
	p := &panel{
		choices: map[uint16]*choiceField{},
		text:    func(key string) string { return key },
	}
	p.setTriggerOptions(automation.ActionShutdown, automation.TriggerProcessStarted)

	if got := p.triggerValue(); got != automation.TriggerProcessStarted {
		t.Fatalf("selected trigger = %q, want %q", got, automation.TriggerProcessStarted)
	}
	want := []automation.Trigger{
		automation.TriggerOnce,
		automation.TriggerDaily,
		automation.TriggerWeekly,
		automation.TriggerProcessStarted,
		automation.TriggerProcessExited,
	}
	if len(p.triggerOptions) != len(want) {
		t.Fatalf("trigger count = %d, want %d", len(p.triggerOptions), len(want))
	}
	for index := range want {
		if p.triggerOptions[index] != want[index] {
			t.Fatalf("trigger[%d] = %q, want %q", index, p.triggerOptions[index], want[index])
		}
	}
}

func TestManagerUsesAuthoritativeStateWhenSaveIsRejected(t *testing.T) {
	latest := State{Revision: "new", Rules: []automation.Rule{{ID: "latest"}}}
	p := &panel{
		view:   managerView,
		state:  State{Revision: "old", Rules: []automation.Rule{{ID: "old"}}},
		rules:  []automation.Rule{{ID: "old"}},
		onSave: func(SaveRequest) SaveResult { return SaveResult{State: latest, Error: "conflict"} },
	}
	if ok, _ := p.notifySave([]automation.Rule{{ID: "stale"}}); ok {
		t.Fatal("rejected save reported success")
	}
	if p.state.Revision != "new" || len(p.rules) != 1 || p.rules[0].ID != "latest" || p.managerNotice != "conflict" {
		t.Fatalf("manager state = %+v, rules = %+v", p.state, p.rules)
	}
}

func TestEditorKeepsDraftBaseWhenExternalStateArrives(t *testing.T) {
	latest := State{Revision: "new", Rules: []automation.Rule{{ID: "latest"}}}
	p := &panel{
		view:   editorView,
		state:  State{Revision: "old", Rules: []automation.Rule{{ID: "old"}}},
		rules:  []automation.Rule{{ID: "old"}},
		onSave: func(SaveRequest) SaveResult { return SaveResult{State: latest, Error: "conflict"} },
	}
	if ok, _ := p.notifySave([]automation.Rule{{ID: "draft"}}); ok {
		t.Fatal("rejected save reported success")
	}
	if p.state.Revision != "old" || p.pendingState == nil || p.pendingState.Revision != "new" {
		t.Fatalf("editor state = %+v, pending = %+v", p.state, p.pendingState)
	}
}
