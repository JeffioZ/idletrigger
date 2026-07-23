package darkmode

import (
	"testing"
)

// Enable must not panic on any Windows version — it's called unconditionally at startup.
func TestEnable_NoPanic(t *testing.T) {
	Enable()
}

// Enable must be idempotent.
func TestEnable_Idempotent(t *testing.T) {
	Enable()
	Enable()
	Enable()
}

func TestAppsUseDark_NoPanic(t *testing.T) {
	AppsUseDark()
}
