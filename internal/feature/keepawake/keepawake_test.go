package keepawake

import "testing"

func TestEnableUpdateDisable(t *testing.T) {
	Enable(false)
	if !IsEnabled() || IsKeepingScreenOn() {
		t.Fatal("sleep-only request was not applied")
	}
	Enable(true)
	if !IsEnabled() || !IsKeepingScreenOn() {
		t.Fatal("display request was not updated")
	}
	Disable()
	if IsEnabled() || IsKeepingScreenOn() {
		t.Fatal("request was not cleared")
	}
}
