package nativeform

import (
	"testing"

	"github.com/JeffioZ/idletrigger/internal/ui/colors"
)

func TestScaledPixelsPreservesFractionalDPI(t *testing.T) {
	tests := []struct {
		scale float64
		want  int32
	}{
		{scale: 1, want: 18},
		{scale: 1.25, want: 23},
		{scale: 1.5, want: 27},
		{scale: 2, want: 36},
	}
	for _, test := range tests {
		if got := scaledPixels(18, test.scale); got != test.want {
			t.Errorf("scaledPixels(18, %.2f) = %d, want %d", test.scale, got, test.want)
		}
	}
}

func TestButtonVisualStatePriority(t *testing.T) {
	palette := colors.ForTheme(false)
	tests := []struct {
		name       string
		state      ControlState
		wantFill   uint32
		wantBorder uint32
	}{
		{name: "hover", state: ControlState{Hovered: true}, wantFill: palette.HoverSurface, wantBorder: palette.Accent},
		{name: "active hover", state: ControlState{Active: true, Hovered: true}, wantFill: palette.SelectedHover, wantBorder: palette.SelectedHover},
		{name: "pressed beats hover", state: ControlState{Hovered: true, Pressed: true}, wantFill: palette.AccentPressed, wantBorder: palette.AccentPressed},
		{name: "pressed beats active hover", state: ControlState{Active: true, Hovered: true, Pressed: true}, wantFill: palette.AccentPressed, wantBorder: palette.AccentPressed},
		{name: "disabled beats pressed", state: ControlState{Pressed: true, Disabled: true}, wantFill: palette.DisabledSurface, wantBorder: palette.SubtleBorder},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fill, border, _ := buttonVisual(palette, test.state)
			if fill != test.wantFill || border != test.wantBorder {
				t.Fatalf("buttonVisual(%+v) = fill %#x border %#x, want fill %#x border %#x", test.state, fill, border, test.wantFill, test.wantBorder)
			}
		})
	}
}
