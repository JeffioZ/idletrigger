package autorules

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/JeffioZ/idletrigger/internal/automation"
)

func TestProcessExitedWaitsForEverySameNameInstance(t *testing.T) {
	target := automation.ProcessTarget{Match: automation.MatchName, Executable: "worker.exe"}
	rule := automation.Rule{ID: "done", Name: "Done", Enabled: true, Action: automation.ActionLock, Trigger: automation.TriggerProcessExited, Processes: []automation.ProcessTarget{target}, WarningSeconds: 60}
	var events []Event
	runner := New([]automation.Rule{rule}, nil, Callbacks{OnEvent: func(event Event) { events = append(events, event) }})
	state := loopState{counts: map[string]int{target.Key(): 2}, processKnown: true, previousRunning: make(map[string]bool), pending: make(map[string]pendingEvent)}
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.Local)
	runner.evaluateEvents(now, &state)
	state.counts[target.Key()] = 1
	runner.evaluateEvents(now.Add(time.Second), &state)
	if len(events) != 0 {
		t.Fatalf("task ended while one same-name instance remained: %+v", events)
	}
	delete(state.counts, target.Key())
	runner.evaluateEvents(now.Add(2*time.Second), &state)
	if len(events) != 1 || events[0].RuleID != rule.ID {
		t.Fatalf("last instance exit did not fire exactly once: %+v", events)
	}
}

func TestProcessStartedFiresOnlyOnNoneToAnyTransition(t *testing.T) {
	target := automation.ProcessTarget{Match: automation.MatchName, Executable: "worker.exe"}
	rule := automation.Rule{ID: "started", Name: "Started", Enabled: true, Action: automation.ActionLock, Trigger: automation.TriggerProcessStarted, Processes: []automation.ProcessTarget{target}, WarningSeconds: 60}
	var events []Event
	runner := New([]automation.Rule{rule}, nil, Callbacks{OnEvent: func(event Event) { events = append(events, event) }})
	state := loopState{counts: map[string]int{target.Key(): 1}, processKnown: true, previousRunning: make(map[string]bool), pending: make(map[string]pendingEvent)}
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.Local)
	runner.evaluateEvents(now, &state)
	if len(events) != 0 {
		t.Fatalf("an already-running process fired during baseline: %+v", events)
	}
	state.counts = map[string]int{}
	runner.evaluateEvents(now.Add(time.Second), &state)
	state.counts[target.Key()] = 1
	runner.evaluateEvents(now.Add(2*time.Second), &state)
	state.counts[target.Key()] = 2
	runner.evaluateEvents(now.Add(3*time.Second), &state)
	if len(events) != 1 || events[0].RuleID != rule.ID {
		t.Fatalf("none-to-any transition events = %+v, want one", events)
	}
}

func TestProcessExitGracePreventsTransientStop(t *testing.T) {
	target := automation.ProcessTarget{Match: automation.MatchName, Executable: "worker.exe"}
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.Local)
	absentSince := make(map[string]time.Time)
	lastCounts := map[string]int{target.Key(): 3}
	firstMissing := stabilizeCounts(now, []automation.ProcessTarget{target}, nil, absentSince, lastCounts)
	if firstMissing[target.Key()] != 3 {
		t.Fatalf("count was cleared on first missing scan: %+v", firstMissing)
	}
	withinGrace := stabilizeCounts(now.Add(processExitGrace-time.Millisecond), []automation.ProcessTarget{target}, nil, absentSince, lastCounts)
	if withinGrace[target.Key()] != 3 {
		t.Fatalf("count was cleared inside exit grace: %+v", withinGrace)
	}
	afterGrace := stabilizeCounts(now.Add(processExitGrace), []automation.ProcessTarget{target}, nil, absentSince, lastCounts)
	if afterGrace[target.Key()] != 0 {
		t.Fatalf("count remained after exit grace: %+v", afterGrace)
	}
}

func TestCrossMidnightWindowUsesStartingDay(t *testing.T) {
	rule := automation.Rule{Time: "23:00", EndTime: "02:00", Days: []string{"mon"}}
	monday := time.Date(2026, 7, 13, 23, 30, 0, 0, time.Local)
	tuesday := time.Date(2026, 7, 14, 1, 30, 0, 0, time.Local)
	if !withinWindow(rule, monday) || !withinWindow(rule, tuesday) {
		t.Fatal("cross-midnight window did not remain active after midnight")
	}
	if withinWindow(rule, tuesday.Add(2*time.Hour)) {
		t.Fatal("cross-midnight window remained active after its end")
	}
	tuesdayOnly := automation.Rule{Time: "23:00", EndTime: "02:00", Days: []string{"tue"}}
	if withinWindow(tuesdayOnly, tuesday) || !withinWindow(tuesdayOnly, tuesday.Add(22*time.Hour)) {
		t.Fatal("after-midnight time was attributed to the wrong starting day")
	}
}

func TestScheduledOccurrenceAllowsOnlyShortTickDelay(t *testing.T) {
	rule := automation.Rule{Trigger: automation.TriggerDaily, Time: "12:00"}
	base := time.Date(2026, 7, 15, 12, 0, 0, 0, time.Local)
	if _, due := scheduledOccurrence(rule, base.Add(90*time.Second)); !due {
		t.Fatal("short scheduler delay lost the occurrence")
	}
	if _, due := scheduledOccurrence(rule, base.Add(scheduleDueGrace)); due {
		t.Fatal("stale scheduled occurrence fired outside the grace window")
	}
}

func TestNextScheduledIncludesFarFutureOneTimeRule(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.Local)
	rule := automation.Rule{ID: "future", Enabled: true, Action: automation.ActionRestart, Trigger: automation.TriggerOnce, Date: "2026-12-31", Time: "23:30", WarningSeconds: 60}
	id, next := nextScheduled([]automation.Rule{rule}, now)
	if id != rule.ID || next.Format("2006-01-02 15:04") != "2026-12-31 23:30" {
		t.Fatalf("next occurrence = %q %s", id, next)
	}
}

func TestWaitingOccurrenceKeepsItsOriginalDeadline(t *testing.T) {
	target := automation.ProcessTarget{Match: automation.MatchName, Executable: "worker.exe"}
	rule := automation.Rule{ID: "wait", Enabled: true, Action: automation.ActionLock, Trigger: automation.TriggerDaily, Time: "12:00", Processes: []automation.ProcessTarget{target}, ProcessLogic: automation.ProcessAny, BlockedPolicy: automation.BlockedWait, MaxWaitMinutes: 1, WarningSeconds: 60}
	runner := New([]automation.Rule{rule}, nil, Callbacks{})
	state := loopState{counts: make(map[string]int), processKnown: true, previousRunning: make(map[string]bool), pending: make(map[string]pendingEvent)}
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.Local)
	runner.evaluateEvents(now, &state)
	first := state.pending[rule.ID].expires
	runner.evaluateEvents(now.Add(30*time.Second), &state)
	if got := state.pending[rule.ID].expires; !got.Equal(first) {
		t.Fatalf("wait deadline moved from %s to %s", first, got)
	}
}

func TestPauseStateActionsOverrideTheirEnableRequests(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.Local)
	rules := []automation.Rule{
		{ID: "awake", Enabled: true, Action: automation.ActionStayAwake, Trigger: automation.TriggerTimeWindow, Time: "09:00", EndTime: "17:00", KeepScreenOn: true},
		{ID: "pause-awake", Enabled: true, Action: automation.ActionPauseStayAwake, Trigger: automation.TriggerTimeWindow, Time: "09:00", EndTime: "17:00"},
		{ID: "idle", Enabled: true, Action: automation.ActionEnableIdle, Trigger: automation.TriggerTimeWindow, Time: "09:00", EndTime: "17:00", IdleMinutes: 10},
		{ID: "pause-idle", Enabled: true, Action: automation.ActionPauseIdle, Trigger: automation.TriggerTimeWindow, Time: "09:00", EndTime: "17:00"},
	}
	state := New(rules, nil, Callbacks{}).evaluateState(now, nil)
	if !state.StayAwake || !state.PauseStayAwake || !state.EnableIdle || !state.PauseIdle {
		t.Fatalf("state requests were not collected: %+v", state)
	}
}

func TestStopRemainsResponsiveWhenCallbackQueueIsSaturated(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.Local)
	rules := make([]automation.Rule, 0, automation.MaxRules)
	for index := 0; index < automation.MaxRules; index++ {
		rules = append(rules, automation.Rule{
			ID: fmt.Sprintf("due-%02d", index), Name: "Due", Enabled: true,
			Action: automation.ActionLock, Trigger: automation.TriggerDaily,
			Time: now.Format("15:04"), WarningSeconds: 60,
		})
	}

	queue := make(chan struct{}, automation.MaxRules)
	var runner *Runner
	enqueue := func() {
		select {
		case queue <- struct{}{}:
		case <-runner.Stopping():
		}
	}
	runner = New(rules, nil, Callbacks{
		OnState: func(EffectiveState) { enqueue() },
		OnEvent: func(Event) { enqueue() },
	})
	runner.now = func() time.Time { return now }
	runner.Start()

	deadline := time.After(2 * time.Second)
	for len(queue) < cap(queue) {
		select {
		case <-deadline:
			t.Fatalf("callback queue did not saturate: %d/%d", len(queue), cap(queue))
		default:
			time.Sleep(time.Millisecond)
		}
	}

	var stops sync.WaitGroup
	stops.Add(3)
	done := make(chan struct{})
	for range 3 {
		go func() {
			defer stops.Done()
			runner.Stop()
		}()
	}
	go func() {
		stops.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("concurrent Stop calls blocked behind the saturated callback queue")
	}
}

func TestInvalidRuleDoesNotBlockValidOccurrence(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.Local)
	rules := []automation.Rule{
		{ID: "valid", Enabled: true, Action: automation.ActionLock, Trigger: automation.TriggerDaily, Time: "12:00", WarningSeconds: 60},
		{ID: "invalid", Enabled: true, Action: automation.ActionLock, Trigger: automation.TriggerDaily, Time: "12:00", WarningSeconds: 5},
	}
	var events []Event
	runner := New(rules, nil, Callbacks{OnEvent: func(event Event) { events = append(events, event) }})
	state := loopState{counts: make(map[string]int), processKnown: true, previousRunning: make(map[string]bool), pending: make(map[string]pendingEvent)}
	runner.evaluateEvents(now, &state)
	if len(events) != 1 || events[0].RuleID != "valid" {
		t.Fatalf("events = %+v, want only the valid rule", events)
	}
}
