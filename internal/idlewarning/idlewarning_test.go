package idlewarning

import "testing"

func TestShow_NoPanic(t *testing.T)     { Show("test", "body") }
func TestHide_NoPanic(t *testing.T)     { Hide() }
func TestSetOnDismiss_Nil(t *testing.T) { SetOnDismiss(nil) }
func TestShowHideRepeat(t *testing.T) {
	for i := 0; i < 3; i++ {
		Show("t", "b")
		Hide()
	}
}

func TestWarningMinimumHeightIsCompact(t *testing.T) {
	if warningMinHeight >= 112 {
		t.Fatalf("minimum warning height = %d, expected a more compact card", warningMinHeight)
	}
}

func TestCloseGlyphMetricsScaleWithoutFonts(t *testing.T) {
	inset, stroke := closeGlyphMetrics(28)
	if inset != 9 || stroke != 2 {
		t.Fatalf("28px glyph = inset %d stroke %d", inset, stroke)
	}
	_, stroke = closeGlyphMetrics(1)
	if stroke != 1 {
		t.Fatalf("small glyph stroke = %d", stroke)
	}
}

func TestWarningContentRectPreservesFormerBorderInset(t *testing.T) {
	got := warningContentRect(rect{Left: 0, Top: 0, Right: 348, Bottom: 92})
	want := rect{Left: 1, Top: 1, Right: 347, Bottom: 91}
	if got != want {
		t.Fatalf("warningContentRect() = %#v, want %#v", got, want)
	}
}
