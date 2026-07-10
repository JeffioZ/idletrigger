package systray

import (
	"fmt"
	"strings"
	"testing"
)

func TestMissingMenuItemUsesErrorHandler(t *testing.T) {
	var message string
	SetErrorHandler(func(format string, args ...interface{}) {
		message = fmt.Sprintf(format, args...)
	})
	t.Cleanup(func() { SetErrorHandler(nil) })

	systrayMenuItemSelected(^uint32(0))
	if !strings.Contains(message, "No menu item with ID") {
		t.Fatalf("unexpected error message: %q", message)
	}
}
