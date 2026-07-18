package actionwarning

import "testing"

func TestWarningPhysicalBoundsUseOneDPITransform(t *testing.T) {
	got := warningPhysicalBounds(warningControlLayout{x: 18, y: 20, width: 354, height: 76}, 1.5)
	want := rect{Left: 27, Top: 30, Right: 558, Bottom: 144}
	if got != want {
		t.Fatalf("scaled warning bounds = %+v, want %+v", got, want)
	}
}

func TestWarningOriginUsesTargetMonitorWorkArea(t *testing.T) {
	work := rect{Left: -1920, Top: 0, Right: 0, Bottom: 1080}
	x, y := warningOrigin(work, 600, 300, 27)
	if x != -627 || y != 753 {
		t.Fatalf("warning origin = (%d,%d), want (-627,753)", x, y)
	}
}
