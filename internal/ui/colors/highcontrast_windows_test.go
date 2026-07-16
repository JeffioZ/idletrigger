package colors

import "testing"

func TestForThemeUsesSystemColorsInHighContrast(t *testing.T) {
	originalQuery := queryHighContrastForColors
	originalColor := getSystemColorForColors
	defer func() {
		queryHighContrastForColors = originalQuery
		getSystemColorForColors = originalColor
	}()
	queryHighContrastForColors = func() bool { return true }
	getSystemColorForColors = func(index int) uint32 { return uint32(0x100000 + index) }

	light := ForTheme(false)
	dark := ForTheme(true)
	if light != dark {
		t.Fatalf("high contrast palette depended on app theme: light=%+v dark=%+v", light, dark)
	}
	if light.WindowBackground != 0x100000+colorWindow || light.PrimaryText != 0x100000+colorWindowText {
		t.Fatalf("window roles did not use system colors: %+v", light)
	}
	if light.Selected != 0x100000+colorHighlight || light.AccentText != 0x100000+colorHighlightText {
		t.Fatalf("selection roles did not use system colors: %+v", light)
	}
	if light.DisabledText != 0x100000+colorGrayText || light.TooltipBackground != 0x100000+colorInfoBackground {
		t.Fatalf("disabled or tooltip roles did not use system colors: %+v", light)
	}
}

func TestForThemePreservesNormalLightAndDarkPalettes(t *testing.T) {
	originalQuery := queryHighContrastForColors
	defer func() { queryHighContrastForColors = originalQuery }()
	queryHighContrastForColors = func() bool { return false }
	if ForTheme(false) != themePalette(false) {
		t.Fatal("normal light palette changed when high contrast was disabled")
	}
	if ForTheme(true) != themePalette(true) {
		t.Fatal("normal dark palette changed when high contrast was disabled")
	}
}
