package automation

import (
	"path/filepath"
	"testing"
)

func TestRuntimeStateRoundTripAndReplace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "IdleTrigger.state.json")
	first := RuntimeState{LastOccurrences: map[string]string{"daily": "2026-07-15T03:00"}}
	if err := SaveRuntimeState(path, first); err != nil {
		t.Fatalf("first save: %v", err)
	}
	second := RuntimeState{LastOccurrences: map[string]string{"weekly": "2026-07-19T23:00"}}
	if err := SaveRuntimeState(path, second); err != nil {
		t.Fatalf("replacement save: %v", err)
	}
	got, err := LoadRuntimeState(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got.LastOccurrences) != 1 || got.LastOccurrences["weekly"] != "2026-07-19T23:00" {
		t.Fatalf("state = %+v", got)
	}
}

func TestMissingRuntimeStateStartsEmpty(t *testing.T) {
	got, err := LoadRuntimeState(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil || got.LastOccurrences == nil || len(got.LastOccurrences) != 0 {
		t.Fatalf("missing state = %+v, %v", got, err)
	}
}
