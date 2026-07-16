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
