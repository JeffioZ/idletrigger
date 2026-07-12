package uicolors

import (
	"math"
	"testing"
)

func TestAccentTextContrast(t *testing.T) {
	for _, dark := range []bool{false, true} {
		palette := ForTheme(dark)
		if got := contrast(palette.AccentText, palette.Accent); got < 4.5 {
			t.Fatalf("dark=%v accent text contrast = %.2f, want >= 4.5", dark, got)
		}
		if palette.Focus == palette.Accent || palette.FocusOnAccent == palette.AccentText {
			t.Fatalf("dark=%v focus colors must remain distinct from selected-control colors", dark)
		}
		if got := contrast(palette.Focus, palette.Surface); got < 3 {
			t.Fatalf("dark=%v standard focus contrast = %.2f, want >= 3", dark, got)
		}
		if got := contrast(palette.FocusOnAccent, palette.Accent); got < 3 {
			t.Fatalf("dark=%v selected focus contrast = %.2f, want >= 3", dark, got)
		}
	}
}

func TestDangerColorsRemainUnchanged(t *testing.T) {
	light := ForTheme(false)
	if light.Danger != RGB(255, 239, 240) || light.DangerHover != RGB(255, 225, 228) || light.DangerPressed != RGB(255, 207, 211) || light.DangerText != RGB(190, 24, 34) {
		t.Fatal("light danger palette changed")
	}
	dark := ForTheme(true)
	if dark.Danger != RGB(88, 34, 39) || dark.DangerHover != RGB(116, 43, 50) || dark.DangerPressed != RGB(72, 28, 33) || dark.DangerText != RGB(255, 162, 168) {
		t.Fatal("dark danger palette changed")
	}
}

func contrast(a, b uint32) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func luminance(c uint32) float64 {
	r := channel(c, 0)
	g := channel(c, 8)
	b := channel(c, 16)
	return 0.2126*linear(r) + 0.7152*linear(g) + 0.0722*linear(b)
}

func channel(c uint32, shift uint) float64 { return float64((c>>shift)&0xff) / 255 }
func linear(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}
