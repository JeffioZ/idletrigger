package font

import "testing"

func TestCandidateOrderFavorsCurrentUILanguage(t *testing.T) {
	zh := candidates(true)
	if zh[0] != "Microsoft YaHei UI" {
		t.Fatalf("Chinese first choice = %q", zh[0])
	}
	en := candidates(false)
	if en[0] != "Segoe UI Variable Text" || en[1] != "Segoe UI" {
		t.Fatalf("Latin candidate order = %#v", en)
	}
}

func TestSameFaceIsCaseInsensitive(t *testing.T) {
	if !sameFace("Segoe UI", "segoe ui") || sameFace("Segoe UI", "Microsoft YaHei UI") {
		t.Fatal("unexpected font face comparison")
	}
}

func TestFirstAvailableExercisesMissingPreferredFontFallback(t *testing.T) {
	got := FirstAvailable(false, func(face string) bool { return face == "Segoe UI" })
	if got != "Segoe UI" {
		t.Fatalf("fallback = %q", got)
	}
}
