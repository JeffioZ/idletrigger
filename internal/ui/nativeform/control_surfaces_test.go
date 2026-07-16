package nativeform

import (
	"errors"
	"testing"

	"golang.org/x/sys/windows"
)

func TestControlSurfaceSetKeepsBidirectionalSingleSource(t *testing.T) {
	var fields ControlSurfaceSet
	field, err := fields.Add(ControlSurfaceOptions{
		ControlID: 10, SurfaceID: 20,
		Control: windows.Handle(100), Surface: windows.Handle(200),
	})
	if err != nil {
		t.Fatal(err)
	}
	if byControl, ok := fields.ForControl(10); !ok || byControl != field {
		t.Fatal("control lookup did not return the registered field")
	}
	if bySurface, ok := fields.ForSurface(20); !ok || bySurface != field {
		t.Fatal("surface lookup did not return the registered field")
	}
	if _, err := fields.Add(ControlSurfaceOptions{
		ControlID: 10, SurfaceID: 21,
		Control: windows.Handle(101), Surface: windows.Handle(201),
	}); err == nil {
		t.Fatal("duplicate control ID was accepted")
	}
	fields.Close()
	if _, ok := fields.ForControl(10); ok {
		t.Fatal("closed field registry retained its bindings")
	}
}

func TestControlSurfaceSetRetainsUsableBindingWhenOptionalCueFails(t *testing.T) {
	var fields ControlSurfaceSet
	wantErr := errors.New("injected cue failure")
	field, err := fields.Add(ControlSurfaceOptions{
		ControlID: 10, SurfaceID: 20,
		Control: windows.Handle(100), Surface: windows.Handle(200),
		CueText: "Search", NewCue: func(windows.Handle, string, uint32, float64) (*CueBanner, error) {
			return nil, wantErr
		},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("cue error = %v, want %v", err, wantErr)
	}
	if field == nil {
		t.Fatal("optional cue failure discarded the native field binding")
	}
	if registered, ok := fields.ForControl(10); !ok || registered != field {
		t.Fatal("optional cue failure installed a partial fallback path")
	}
	fields.Close()
}

func TestControlSurfaceSetSeparatesCueDisplayFromLogicalText(t *testing.T) {
	const edit = windows.Handle(100)
	cue := &CueBanner{edit: edit, color: 0x123456, displaying: true}
	fields := ControlSurfaceSet{byControl: map[uint16]*ControlSurface{
		10: {ControlID: 10, Control: edit, cue: cue},
	}}
	if got := fields.LogicalText(edit, "Search processes"); got != "" {
		t.Fatalf("logical cue text = %q, want empty", got)
	}
	if color, ok := fields.CueColor(edit); !ok || color != cue.color {
		t.Fatalf("cue color = %06x, %v", color, ok)
	}
	cue.displaying = false
	if got := fields.LogicalText(edit, "player"); got != "player" {
		t.Fatalf("logical user text = %q, want player", got)
	}
}
