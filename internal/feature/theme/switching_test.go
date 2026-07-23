package theme

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
)

type fakeThemePreferenceStore struct {
	values      map[string]uint64
	valueTypes  map[string]uint32
	failSet     map[string]int
	discardSet  map[string]bool
	setCalls    []string
	deleteCalls []string
}

func newFakeThemePreferenceStore(apps, system uint64) *fakeThemePreferenceStore {
	return &fakeThemePreferenceStore{
		values: map[string]uint64{
			"AppsUseLightTheme":    apps,
			"SystemUsesLightTheme": system,
		},
		valueTypes: map[string]uint32{
			"AppsUseLightTheme":    registry.DWORD,
			"SystemUsesLightTheme": registry.DWORD,
		},
		failSet:    make(map[string]int),
		discardSet: make(map[string]bool),
	}
}

func (s *fakeThemePreferenceStore) GetIntegerValue(name string) (uint64, uint32, error) {
	value, ok := s.values[name]
	if !ok {
		return 0, 0, registry.ErrNotExist
	}
	return value, s.valueTypes[name], nil
}

func (s *fakeThemePreferenceStore) SetDWordValue(name string, value uint32) error {
	s.setCalls = append(s.setCalls, name)
	if s.failSet[name] > 0 {
		s.failSet[name]--
		return errors.New("injected write failure")
	}
	if !s.discardSet[name] {
		s.values[name] = uint64(value)
		s.valueTypes[name] = registry.DWORD
	}
	return nil
}

func (s *fakeThemePreferenceStore) DeleteValue(name string) error {
	s.deleteCalls = append(s.deleteCalls, name)
	delete(s.values, name)
	delete(s.valueTypes, name)
	return nil
}

func TestApplyThemeModeValuesUpdatesBothPreferences(t *testing.T) {
	store := newFakeThemePreferenceStore(1, 1)
	if err := applyThemeModeValues(store, 0); err != nil {
		t.Fatalf("applyThemeModeValues: %v", err)
	}
	if store.values["AppsUseLightTheme"] != 0 || store.values["SystemUsesLightTheme"] != 0 {
		t.Fatalf("theme preferences = %v, want both dark", store.values)
	}
}

func TestApplyThemePreferenceValuesPreservesIndependentModes(t *testing.T) {
	store := newFakeThemePreferenceStore(0, 1)
	desired := map[string]uint32{"AppsUseLightTheme": 1, "SystemUsesLightTheme": 0}
	if err := applyThemePreferenceValues(store, desired); err != nil {
		t.Fatalf("applyThemePreferenceValues: %v", err)
	}
	if store.values["AppsUseLightTheme"] != 1 || store.values["SystemUsesLightTheme"] != 0 {
		t.Fatalf("theme preferences = %v, want independent target values", store.values)
	}
}

func TestApplyThemeModeValuesRollsBackPartialWrite(t *testing.T) {
	store := newFakeThemePreferenceStore(1, 0)
	store.failSet["SystemUsesLightTheme"] = 1

	err := applyThemeModeValues(store, 0)
	if err == nil || !strings.Contains(err.Error(), "set SystemUsesLightTheme") {
		t.Fatalf("applyThemeModeValues error = %v, want system write failure", err)
	}
	if store.values["AppsUseLightTheme"] != 1 || store.values["SystemUsesLightTheme"] != 0 {
		t.Fatalf("theme preferences after rollback = %v, want original values", store.values)
	}
	wantCalls := []string{"AppsUseLightTheme", "SystemUsesLightTheme", "SystemUsesLightTheme", "AppsUseLightTheme"}
	if !slices.Equal(store.setCalls, wantCalls) {
		t.Fatalf("write order = %v, want %v", store.setCalls, wantCalls)
	}
}

func TestApplyThemeModeValuesRestoresMissingValue(t *testing.T) {
	store := newFakeThemePreferenceStore(1, 1)
	delete(store.values, "AppsUseLightTheme")
	delete(store.valueTypes, "AppsUseLightTheme")
	store.failSet["SystemUsesLightTheme"] = 1

	err := applyThemeModeValues(store, 0)
	if err == nil || !strings.Contains(err.Error(), "set SystemUsesLightTheme") {
		t.Fatalf("applyThemeModeValues error = %v, want system write failure", err)
	}
	if _, exists := store.values["AppsUseLightTheme"]; exists {
		t.Fatalf("missing app preference was recreated after rollback: %v", store.values)
	}
	if !slices.Equal(store.deleteCalls, []string{"AppsUseLightTheme"}) {
		t.Fatalf("delete calls = %v, want missing app preference restored", store.deleteCalls)
	}
}

func TestApplyThemeModeValuesRollsBackVerificationFailure(t *testing.T) {
	store := newFakeThemePreferenceStore(1, 1)
	store.discardSet["SystemUsesLightTheme"] = true

	err := applyThemeModeValues(store, 0)
	if err == nil || !strings.Contains(err.Error(), "verify SystemUsesLightTheme") {
		t.Fatalf("applyThemeModeValues error = %v, want verification failure", err)
	}
	if store.values["AppsUseLightTheme"] != 1 || store.values["SystemUsesLightTheme"] != 1 {
		t.Fatalf("theme preferences after rollback = %v, want original values", store.values)
	}
}

func TestApplyThemeModeValuesRejectsInvalidInitialStateBeforeWriting(t *testing.T) {
	store := newFakeThemePreferenceStore(2, 1)
	err := applyThemeModeValues(store, 0)
	if err == nil || !strings.Contains(err.Error(), "invalid Windows light/dark setting") {
		t.Fatalf("applyThemeModeValues error = %v, want invalid-state error", err)
	}
	if len(store.setCalls) != 0 {
		t.Fatalf("writes performed for invalid initial state: %v", store.setCalls)
	}
}

func TestApplyThemePreferenceValuesRejectsInvalidTargetBeforeWriting(t *testing.T) {
	store := newFakeThemePreferenceStore(1, 1)
	desired := map[string]uint32{"AppsUseLightTheme": 2}

	err := applyThemePreferenceValues(store, desired)
	if err == nil || !strings.Contains(err.Error(), "target AppsUseLightTheme") {
		t.Fatalf("applyThemePreferenceValues error = %v, want invalid-target error", err)
	}
	if len(store.setCalls) != 0 {
		t.Fatalf("writes performed for invalid target: %v", store.setCalls)
	}
}

func TestApplyThemeModeValuesReportsRollbackFailure(t *testing.T) {
	store := newFakeThemePreferenceStore(1, 1)
	store.failSet["AppsUseLightTheme"] = 2

	err := applyThemeModeValues(store, 0)
	if err == nil || !strings.Contains(err.Error(), "set AppsUseLightTheme") ||
		!strings.Contains(err.Error(), "roll back partial theme switch") ||
		!strings.Contains(err.Error(), "restore AppsUseLightTheme") {
		t.Fatalf("applyThemeModeValues error = %v, want update and rollback failures", err)
	}
}

func TestRestoreThemePreferenceValuesKeepsSuccessfulRecovery(t *testing.T) {
	store := newFakeThemePreferenceStore(0, 0)
	store.failSet["SystemUsesLightTheme"] = 1
	desired := map[string]uint32{"AppsUseLightTheme": 1, "SystemUsesLightTheme": 1}

	err := restoreThemePreferenceValues(store, desired)
	if err == nil || !strings.Contains(err.Error(), "set SystemUsesLightTheme") {
		t.Fatalf("restoreThemePreferenceValues error = %v, want system restore failure", err)
	}
	if store.values["AppsUseLightTheme"] != 1 {
		t.Fatalf("successful app-mode recovery was rolled back: %v", store.values)
	}
	wantCalls := []string{"AppsUseLightTheme", "SystemUsesLightTheme"}
	if !slices.Equal(store.setCalls, wantCalls) {
		t.Fatalf("write order = %v, want one best-effort write per value", store.setCalls)
	}
}

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
