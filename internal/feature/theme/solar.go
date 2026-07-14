package theme

import (
	"math"
	"time"
)

// CalcSunriseSunset returns light and dark times as minutes since midnight,
// calculated using the NOAA solar calculator.
func CalcSunriseSunset(t time.Time, lat, lon float64) (sunriseMinutes, sunsetMinutes int) {
	if math.IsNaN(lat) || math.IsInf(lat, 0) || math.IsNaN(lon) || math.IsInf(lon, 0) {
		return -1, -1
	}
	// Day of year
	doy := float64(t.YearDay())

	// Fractional year in radians
	gamma := 2 * math.Pi / 365 * (doy - 1 + float64(t.Hour()-12)/24)

	// Equation of time
	eqtime := 229.18 * (0.000075 + 0.001868*math.Cos(gamma) - 0.032077*math.Sin(gamma) -
		0.014615*math.Cos(2*gamma) - 0.040849*math.Sin(2*gamma))

	// Solar declination
	decl := 0.006918 - 0.399912*math.Cos(gamma) + 0.070257*math.Sin(gamma) -
		0.006758*math.Cos(2*gamma) + 0.000907*math.Sin(2*gamma) -
		0.002697*math.Cos(3*gamma) + 0.00148*math.Sin(3*gamma)

	// Hour angle
	latRad := lat * math.Pi / 180
	zenith := 90.833 * math.Pi / 180 // official sunrise/sunset zenith
	acosArg := math.Cos(zenith)/(math.Cos(latRad)*math.Cos(decl)) - math.Tan(latRad)*math.Tan(decl)
	if acosArg < -1 || acosArg > 1 {
		return -1, -1
	}
	ha := math.Acos(acosArg)

	// Solar noon in minutes (UTC)
	solarNoon := (720 - 4*lon - eqtime)
	// in minutes UTC
	_, offset := t.Zone()
	solarNoonLocal := solarNoon + float64(offset)/60 // convert to local minutes

	// Sunrise / sunset in local minutes
	sunrise := solarNoonLocal - ha*4*180/math.Pi
	sunset := solarNoonLocal + ha*4*180/math.Pi

	// Clamp to valid range and wrap
	sr := int(math.Round(sunrise))
	ss := int(math.Round(sunset))
	for sr < 0 {
		sr += 1440
	}
	for ss < 0 {
		ss += 1440
	}
	for sr >= 1440 {
		sr -= 1440
	}
	for ss >= 1440 {
		ss -= 1440
	}

	return sr, ss
}
