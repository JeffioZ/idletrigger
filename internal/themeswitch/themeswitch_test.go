package themeswitch

import (
	"testing"
	"time"
)

func TestParseTime(t *testing.T) {
	for input, want := range map[string]int{
		"00:00": 0,
		"07:30": 450,
		"23:59": 1439,
		"bad":   -1,
		"aa:10": -1,
		"24:00": -1,
	} {
		if got := parseTime(input); got != want {
			t.Errorf("parseTime(%q) = %d, want %d", input, got, want)
		}
	}
}

func TestSunriseSunsetBeijingSummer(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	date := time.Date(2026, 6, 21, 12, 0, 0, 0, loc)
	sunrise, sunset := sunriseSunset(date, 39.9, 116.4)
	if sunrise < 240 || sunrise > 360 {
		t.Fatalf("unexpected sunrise: %d", sunrise)
	}
	if sunset < 1140 || sunset > 1260 {
		t.Fatalf("unexpected sunset: %d", sunset)
	}
}

func TestSunriseSunsetPolarCondition(t *testing.T) {
	date := time.Date(2026, 12, 21, 12, 0, 0, 0, time.UTC)
	sunrise, sunset := sunriseSunset(date, 89, 0)
	if sunrise != -1 || sunset != -1 {
		t.Fatalf("polar condition should be unavailable, got %d/%d", sunrise, sunset)
	}
}

func TestManualOverrideEndsAtNextScheduledTransition(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	s := NewScheduler("fixed", "07:00", "19:00", 0, 0, false, false)
	now := time.Date(2026, 7, 11, 10, 0, 0, 0, loc)
	s.HoldManualOverride(now)
	want := time.Date(2026, 7, 11, 19, 0, 0, 0, loc)
	if got := time.Unix(0, s.manualUntil.Load()); !got.Equal(want) {
		t.Fatalf("manual override ends at %s, want %s", got, want)
	}
}

func TestManualOverrideCrossesMidnightToNextTransition(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	s := NewScheduler("fixed", "19:00", "07:00", 0, 0, false, false)
	now := time.Date(2026, 7, 11, 22, 0, 0, 0, loc)
	s.HoldManualOverride(now)
	want := time.Date(2026, 7, 12, 7, 0, 0, 0, loc)
	if got := time.Unix(0, s.manualUntil.Load()); !got.Equal(want) {
		t.Fatalf("manual override ends at %s, want %s", got, want)
	}
}
