package popup

import "testing"

func TestTimeoutIndex(t *testing.T) {
	for index, timeout := range []int{5, 10, 30, 60, 120} {
		if got := timeoutIndex(timeout); got != index {
			t.Fatalf("timeoutIndex(%d) = %d, want %d", timeout, got, index)
		}
	}
	if got := timeoutIndex(15); got != 2 {
		t.Fatalf("timeoutIndex fallback = %d, want 2", got)
	}
}

func TestActionIndex(t *testing.T) {
	for index, action := range []string{"sleep", "hibernate", "shutdown", "lock"} {
		if got := actionIndex(action); got != index {
			t.Fatalf("actionIndex(%q) = %d, want %d", action, got, index)
		}
	}
	if got := actionIndex("invalid"); got != 0 {
		t.Fatalf("actionIndex fallback = %d, want 0", got)
	}
}

func TestTimeoutLabelsUseLocalization(t *testing.T) {
	labels := timeoutLabels(func(key string) string { return "translated:" + key })
	if labels[0] != "translated:menu_timeout_5" || labels[4] != "translated:menu_timeout_120" {
		t.Fatalf("unexpected localized labels: %#v", labels)
	}
}
