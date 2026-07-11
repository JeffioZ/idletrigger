package themeswitch

import (
	"strings"
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
	sunrise, sunset := CalcSunriseSunset(date, 39.9, 116.4)
	if sunrise < 240 || sunrise > 360 {
		t.Fatalf("unexpected sunrise: %d", sunrise)
	}
	if sunset < 1140 || sunset > 1260 {
		t.Fatalf("unexpected sunset: %d", sunset)
	}
}

func TestSunriseSunsetPolarCondition(t *testing.T) {
	date := time.Date(2026, 12, 21, 12, 0, 0, 0, time.UTC)
	sunrise, sunset := CalcSunriseSunset(date, 89, 0)
	if sunrise != -1 || sunset != -1 {
		t.Fatalf("polar condition should be unavailable, got %d/%d", sunrise, sunset)
	}
}

func TestScheduleInfoFallsBackToFixedTimeWhenSunriseUnavailable(t *testing.T) {
	date := time.Date(2026, 12, 21, 12, 0, 0, 0, time.UTC)
	info := ScheduleInfoFor("sunrise", "08:30", "20:45", 89, 0, date)
	if !info.OK {
		t.Fatal("schedule should fall back to fixed time")
	}
	if !info.FixedFallback {
		t.Fatal("schedule should report fixed fallback")
	}
	if info.LightTime != "08:30" || info.DarkTime != "20:45" {
		t.Fatalf("schedule = %s/%s, want 08:30/20:45", info.LightTime, info.DarkTime)
	}
}

func TestSchedulerFallsBackToFixedTimeWhenSunriseUnavailable(t *testing.T) {
	date := time.Date(2026, 12, 21, 12, 0, 0, 0, time.UTC)
	s := NewScheduler("sunrise", "08:30", "20:45", 89, 0, false, false)
	light, dark := s.scheduleMinutes(date)
	if light != 510 || dark != 1245 {
		t.Fatalf("schedule minutes = %d/%d, want 510/1245", light, dark)
	}
}

func TestLocationFromTimezoneFallsBackToUTCOffset(t *testing.T) {
	info := locationFromTimezone("Unknown Localized Time", -480, true)
	if info.Source != LocationSourceUTCOffset {
		t.Fatalf("source = %s, want %s", info.Source, LocationSourceUTCOffset)
	}
	if info.Longitude != 120 {
		t.Fatalf("longitude = %v, want 120", info.Longitude)
	}
	if info.Latitude != 35 {
		t.Fatalf("latitude = %v, want 35", info.Latitude)
	}
}

func TestLocationFromTimezoneUsesDefaultOnlyWhenTimezoneInvalid(t *testing.T) {
	info := locationFromTimezone("Unknown Localized Time", 0, false)
	if info.Source != LocationSourceDefault {
		t.Fatalf("source = %s, want %s", info.Source, LocationSourceDefault)
	}
}

func TestParseIPLocation(t *testing.T) {
	info, err := parseIPLocation(strings.NewReader(`{"success":true,"latitude":31.2304,"longitude":121.4737,"city":"Shanghai","region":"Shanghai","country":"China"}`))
	if err != nil {
		t.Fatalf("parseIPLocation: %v", err)
	}
	if info.Source != LocationSourceIP {
		t.Fatalf("source = %s, want %s", info.Source, LocationSourceIP)
	}
	if info.Latitude != 31.2304 || info.Longitude != 121.4737 {
		t.Fatalf("coordinates = %.4f/%.4f", info.Latitude, info.Longitude)
	}
	if info.LocationLabel != "Shanghai, Shanghai, China" {
		t.Fatalf("label = %q", info.LocationLabel)
	}
}

func TestParseIPLocationRejectsFailedResponse(t *testing.T) {
	if _, err := parseIPLocation(strings.NewReader(`{"success":false,"message":"rate limited"}`)); err == nil {
		t.Fatal("expected failed service response to return an error")
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
