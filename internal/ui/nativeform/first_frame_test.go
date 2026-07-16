package nativeform

import (
	"reflect"
	"testing"

	"golang.org/x/sys/windows"
)

func TestFirstFrameGateCloaksPaintsAndRevealsInOrder(t *testing.T) {
	var events []string
	api := firstFrameAPI{
		setDWMBoolean: func(_ windows.Handle, attribute uint32, enabled bool) bool {
			events = append(events, eventName(attribute, enabled))
			return true
		},
		showWindow: func(windows.Handle) { events = append(events, "show") },
		present: func(_ windows.Handle, controls ...windows.Handle) {
			events = append(events, "present")
			if !reflect.DeepEqual(controls, []windows.Handle{2, 3}) {
				t.Fatalf("controls=%v", controls)
			}
		},
	}
	gate := beginFirstFrameWith(1, api)
	if err := gate.Reveal(FirstFrameOptions{RepeatShow: true, Controls: []windows.Handle{2, 3}}); err != nil {
		t.Fatal(err)
	}
	want := []string{"transitions-on", "cloak-on", "show", "show", "present", "cloak-off", "present", "transitions-off"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events=%v, want %v", events, want)
	}
	if err := gate.Reveal(FirstFrameOptions{}); err == nil {
		t.Fatal("second reveal unexpectedly succeeded")
	}
}

func TestFirstFrameGateFallsBackWhenCloakIsUnavailable(t *testing.T) {
	var uncloaked, presented bool
	api := firstFrameAPI{
		setDWMBoolean: func(_ windows.Handle, attribute uint32, enabled bool) bool {
			if attribute == firstFrameCloak && !enabled {
				uncloaked = true
			}
			return attribute != firstFrameCloak
		},
		showWindow: func(windows.Handle) {},
		present:    func(windows.Handle, ...windows.Handle) { presented = true },
	}
	gate := beginFirstFrameWith(1, api)
	if err := gate.Reveal(FirstFrameOptions{}); err != nil {
		t.Fatal(err)
	}
	if !presented || uncloaked {
		t.Fatalf("presented=%v uncloak_attempted=%v", presented, uncloaked)
	}
}

func TestFirstFrameGateReportsUncloakFailure(t *testing.T) {
	var transitionsRestored bool
	api := firstFrameAPI{
		setDWMBoolean: func(_ windows.Handle, attribute uint32, enabled bool) bool {
			if attribute == firstFrameTransitionsForcedDisabled && !enabled {
				transitionsRestored = true
			}
			return enabled || attribute != firstFrameCloak
		},
		showWindow: func(windows.Handle) {},
		present:    func(windows.Handle, ...windows.Handle) {},
	}
	gate := beginFirstFrameWith(1, api)
	if err := gate.Reveal(FirstFrameOptions{}); err == nil {
		t.Fatal("uncloak failure was not returned")
	}
	if !transitionsRestored {
		t.Fatal("window transitions were not restored after reveal failure")
	}
}

func eventName(attribute uint32, enabled bool) string {
	name := "transitions"
	if attribute == firstFrameCloak {
		name = "cloak"
	}
	if enabled {
		return name + "-on"
	}
	return name + "-off"
}
