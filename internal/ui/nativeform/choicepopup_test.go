package nativeform

import "testing"

func TestChoicePopupRowHitTestingSkipsHeadersAndGaps(t *testing.T) {
	p := &ChoicePopup{
		options: ChoicePopupOptions{
			MaxVisible: 3,
			Items: []ChoicePopupItem{
				{Label: "State", Header: true},
				{Label: "Enable", Value: 1},
				{Label: "Disable", Value: 2},
			},
		},
		rowHeight: 34,
		rowGap:    1,
		inset:     4,
	}
	if got := p.rowAt(4 + 17); got != -1 {
		t.Fatalf("header hit = %d, want -1", got)
	}
	if got := p.rowAt(4 + 34); got != -1 {
		t.Fatalf("row gap hit = %d, want -1", got)
	}
	if got := p.rowAt(4 + 35 + 17); got != 1 {
		t.Fatalf("option hit = %d, want 1", got)
	}
}

func TestChoicePopupKeyboardNavigationAndViewport(t *testing.T) {
	p := &ChoicePopup{
		options: ChoicePopupOptions{
			MaxVisible: 3,
			Items: []ChoicePopupItem{
				{Header: true}, {Value: 0}, {Value: 1},
				{Header: true}, {Value: 2}, {Value: 3},
			},
		},
	}
	if got := p.nextSelectable(2, 1); got != 4 {
		t.Fatalf("next selectable = %d, want 4", got)
	}
	p.ensureVisible(5, p.visibleRows())
	if p.first != 3 {
		t.Fatalf("viewport first = %d, want 3", p.first)
	}
	p.ensureVisible(1, p.visibleRows())
	if p.first != 1 {
		t.Fatalf("viewport first after moving up = %d, want 1", p.first)
	}
}
