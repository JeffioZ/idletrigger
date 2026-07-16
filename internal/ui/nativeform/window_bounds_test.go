package nativeform

import "testing"

func TestConstrainRectKeepsWindowInsideWorkArea(t *testing.T) {
	work := Rect{Left: 100, Top: 50, Right: 1466, Bottom: 818}
	for _, test := range []struct {
		name string
		in   Rect
		want Rect
	}{
		{"already visible", Rect{Left: 200, Top: 100, Right: 900, Bottom: 610}, Rect{Left: 200, Top: 100, Right: 900, Bottom: 610}},
		{"oversized", Rect{Left: -50, Top: -80, Right: 1550, Bottom: 920}, work},
		{"off right bottom", Rect{Left: 1300, Top: 700, Right: 1800, Bottom: 1000}, Rect{Left: 966, Top: 518, Right: 1466, Bottom: 818}},
		{"off left top", Rect{Left: -300, Top: -200, Right: 400, Bottom: 300}, Rect{Left: 100, Top: 50, Right: 800, Bottom: 550}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := ConstrainRect(test.in, work); got != test.want {
				t.Fatalf("ConstrainRect(%+v, %+v) = %+v, want %+v", test.in, work, got, test.want)
			}
		})
	}
}

func TestCenteredRectPreservesRequestedSize(t *testing.T) {
	got := CenteredRect(Rect{Left: 100, Top: 50, Right: 1466, Bottom: 818}, 700, 510)
	if got != (Rect{Left: 433, Top: 179, Right: 1133, Bottom: 689}) {
		t.Fatalf("CenteredRect = %+v", got)
	}
}

func TestInitialWindowPointFallbackCannotReachTheDesktop(t *testing.T) {
	if windowFallbackPoint > -30000 {
		t.Fatalf("window fallback point %d is not safely outside the desktop", windowFallbackPoint)
	}
}
