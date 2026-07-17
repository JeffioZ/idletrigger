package app

import (
	"fmt"

	"github.com/JeffioZ/idletrigger/internal/feature/theme"
	mylog "github.com/JeffioZ/idletrigger/internal/logging"
)

func (s *runtimeState) detectThemeSupport() {
	s.themeSupportChecked = true
	s.themeSupportErr = theme.DetectSupport()
	s.themeSupported = s.themeSupportErr == nil
	if s.themeSupportErr != nil {
		mylog.Info("Day/night theme unavailable: %v", s.themeSupportErr)
	}
}

// themeAvailable treats an undetected zero-value runtimeState as available so
// focused unit tests and helpers do not accidentally model an unsupported OS.
func (s *runtimeState) themeAvailable() bool {
	return !s.themeSupportChecked || s.themeSupported
}

func (s *runtimeState) disableThemeForRuntime(err error) {
	if err == nil {
		return
	}
	s.themeSupportChecked = true
	s.themeSupported = false
	s.themeSupportErr = err
	s.stopThemeScheduler()
	s.stopIPLocationCycle()
	s.syncBatteryLoop()
	s.updateIcon()
	mylog.Info("Day/night theme disabled after a runtime failure: %v", err)
	s.refreshControlPanel()
}

func (s *runtimeState) themeUnavailableDetail() string {
	if s.themeSupportErr == nil {
		return ""
	}
	return fmt.Sprintf("%v", s.themeSupportErr)
}

func (s *runtimeState) startThemeScheduler() {
	s.stopThemeScheduler()
	if !s.themeAvailable() || s.cfg.ThemeLightTime == "" || s.cfg.ThemeDarkTime == "" {
		return
	}
	loc := theme.LocationInfo{Latitude: s.cfg.ThemeLatitude, Longitude: s.cfg.ThemeLongitude, Source: theme.LocationSourceConfigured}
	if s.cfg.ThemeMode == "sunrise" {
		loc = s.themeLocationInfo(false)
	}
	scheduler := theme.NewScheduler(s.cfg.ThemeMode, s.cfg.ThemeLightTime, s.cfg.ThemeDarkTime, loc.Latitude, loc.Longitude, s.cfg.ThemeSkipFullscreen, s.cfg.ThemeDarkOnBattery)
	scheduler.SetFailureHandler(func(err error) {
		s.post(func() {
			if s.themeSched == scheduler {
				s.disableThemeForRuntime(err)
			}
		})
	})
	s.themeSched = scheduler
	scheduler.Start()
	mylog.Info("Theme scheduler started: mode=%s light=%s dark=%s lat=%.4f lon=%.4f source=%s", s.cfg.ThemeMode, s.cfg.ThemeLightTime, s.cfg.ThemeDarkTime, loc.Latitude, loc.Longitude, loc.Source)
}

func (s *runtimeState) stopThemeScheduler() {
	if s.themeSched != nil {
		s.themeSched.Stop()
		s.themeSched = nil
	}
}
