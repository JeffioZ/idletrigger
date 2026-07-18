package actionwarning

import "testing"

func TestWarningPhysicalBoundsUseOneDPITransform(t *testing.T) {
	got := warningPhysicalBounds(warningControlLayout{x: warningBodyX, y: warningBodyY, width: warningBodyWidth, height: warningBodyHeight}, 1.5)
	want := rect{Left: 24, Top: 24, Right: 561, Bottom: 138}
	if got != want {
		t.Fatalf("scaled warning bounds = %+v, want %+v", got, want)
	}
}

func TestWarningLayoutUsesSharedRhythm(t *testing.T) {
	if warningBodyX != warningPadding || warningBodyY != warningPadding || warningWidth-warningBodyX-warningBodyWidth != warningPadding {
		t.Fatal("warning body should use the shared edge inset")
	}
	if warningButtonsY-(warningBodyY+warningBodyHeight) != warningPadding {
		t.Fatal("warning body and actions should use the shared section gap")
	}
	if warningExecuteX-(warningCancelX+warningButtonWidth) != warningButtonGap {
		t.Fatal("warning actions should use the shared control gap")
	}
	if warningWidth-(warningExecuteX+warningButtonWidth) != warningPadding || warningHeight-(warningButtonsY+warningButtonHeight) != warningPadding {
		t.Fatal("warning actions should keep the shared right and bottom inset")
	}
}

func TestWarningOriginUsesTargetMonitorWorkArea(t *testing.T) {
	work := rect{Left: -1920, Top: 0, Right: 0, Bottom: 1080}
	x, y := warningOrigin(work, 600, 300, 27)
	if x != -627 || y != 753 {
		t.Fatalf("warning origin = (%d,%d), want (-627,753)", x, y)
	}
}
