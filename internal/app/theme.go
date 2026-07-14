package app

import (
	"github.com/JeffioZ/idletrigger/internal/feature/theme"
	mylog "github.com/JeffioZ/idletrigger/internal/logging"
)

func (s *runtimeState) startThemeScheduler() {
	s.stopThemeScheduler()
	if s.cfg.ThemeLightTime == "" || s.cfg.ThemeDarkTime == "" {
		return
	}
	loc := theme.LocationInfo{Latitude: s.cfg.ThemeLatitude, Longitude: s.cfg.ThemeLongitude, Source: theme.LocationSourceConfigured}
	if s.cfg.ThemeMode == "sunrise" {
		loc = s.themeLocationInfo(false)
	}
	s.themeSched = theme.NewScheduler(s.cfg.ThemeMode, s.cfg.ThemeLightTime, s.cfg.ThemeDarkTime, loc.Latitude, loc.Longitude, s.cfg.ThemeSkipFullscreen, s.cfg.ThemeDarkOnBattery)
	s.themeSched.Start()
	mylog.Info("Theme scheduler started: mode=%s light=%s dark=%s lat=%.4f lon=%.4f source=%s", s.cfg.ThemeMode, s.cfg.ThemeLightTime, s.cfg.ThemeDarkTime, loc.Latitude, loc.Longitude, loc.Source)
}

func (s *runtimeState) stopThemeScheduler() {
	if s.themeSched != nil {
		s.themeSched.Stop()
		s.themeSched = nil
	}
}
