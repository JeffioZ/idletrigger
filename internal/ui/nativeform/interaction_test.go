package nativeform

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestInteractionTrackerSeparatesNativeAndVisibleFocus(t *testing.T) {
	tracker := &InteractionTracker{}
	control := windows.Handle(42)
	interactionMu.Lock()
	interactionControls[control] = &trackedControl{owner: tracker, state: InteractionState{Focused: true}}
	interactionMu.Unlock()
	t.Cleanup(func() {
		interactionMu.Lock()
		delete(interactionControls, control)
		interactionMu.Unlock()
	})

	state := tracker.State(control)
	if !state.Focused || state.FocusVisible {
		t.Fatalf("mouse focus state = %+v, want native focus without an outline", state)
	}
	tracker.SetFocusVisible(true)
	state = tracker.State(control)
	if !state.Focused || !state.FocusVisible {
		t.Fatalf("keyboard focus state = %+v, want a visible outline", state)
	}
	tracker.SetFocusVisible(false)
	state = tracker.State(control)
	if !state.Focused || state.FocusVisible {
		t.Fatalf("returning to mouse input changed native focus: %+v", state)
	}
}
