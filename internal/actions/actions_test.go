package actions

import "testing"

// Sleep/Hibernate will actually suspend the system if called.
// Tests only verify that exported functions exist and return predictable types.
func TestExportsExist(t *testing.T) {
	// Compile-time check only — we cannot call these in tests.
	_ = Sleep
	_ = Hibernate
	_ = Shutdown
	_ = Lock
}
