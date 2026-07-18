package theme

import "testing"

func TestThemeChangeNotificationsAreUnique(t *testing.T) {
	notifications := themeChangeNotifications()
	if len(notifications) != 5 {
		t.Fatalf("theme notification count = %d, want 5", len(notifications))
	}
	seen := make(map[themeChangeNotification]struct{}, len(notifications))
	for _, notification := range notifications {
		if _, exists := seen[notification]; exists {
			t.Fatalf("duplicate theme notification: %+v", notification)
		}
		seen[notification] = struct{}{}
	}
}
