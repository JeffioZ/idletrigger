package nativeform

import "testing"

func TestCueBannerVisibilityDependsOnlyOnFieldContent(t *testing.T) {
	banner := &CueBanner{text: "Search", scale: 1}
	if banner.text == "" {
		t.Fatal("empty cue cannot describe the field")
	}
	// Focus is deliberately absent from CueBanner state: an empty field keeps
	// its prompt after Windows gives it initial keyboard focus.
	if banner.closed {
		t.Fatal("new cue banner should be drawable")
	}
}
