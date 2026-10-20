// Package holidays knows the French public holidays and school holidays, two of
// the strongest drivers of the park attendance.
package holidays

import (
	"time"

	"github.com/romitou/previsix/dates"
)

// easter returns the Easter Sunday of a year (anonymous Gregorian algorithm).
func easter(year int) time.Time {
	a := year % 19
	b := year / 100
	c := year % 100
	d := b / 4
	e := b % 4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i := c / 4
	k := c % 4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := (h+l-7*m+114)%31 + 1
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, dates.Paris)
}

// PublicHolidays returns the public holidays of metropolitan France for a year,
// indexed by YYYY-MM-DD.
func PublicHolidays(year int) map[string]string {
	fixed := func(month time.Month, day int) string {
		return time.Date(year, month, day, 0, 0, 0, 0, dates.Paris).Format(dates.DayFormat)
	}
	easterSunday := easter(year)
	relative := func(days int) string {
		return easterSunday.AddDate(0, 0, days).Format(dates.DayFormat)
	}
	return map[string]string{
		fixed(time.January, 1):   "Jour de l'an",
		relative(1):              "Lundi de Pâques",
		fixed(time.May, 1):       "Fête du travail",
		fixed(time.May, 8):       "Victoire 1945",
		relative(39):             "Ascension",
		relative(50):             "Lundi de Pentecôte",
		fixed(time.July, 14):     "Fête nationale",
		fixed(time.August, 15):   "Assomption",
		fixed(time.November, 1):  "Toussaint",
		fixed(time.November, 11): "Armistice 1918",
		fixed(time.December, 25): "Noël",
	}
}

// PublicHoliday returns the name of the public holiday falling on day, if any.
func PublicHoliday(day time.Time) (string, bool) {
	day = day.In(dates.Paris)
	name, ok := PublicHolidays(day.Year())[day.Format(dates.DayFormat)]
	return name, ok
}

// IsBridge reports whether day is a "pont": a working day squeezed between a
// public holiday and a weekend, that many people take off.
func IsBridge(day time.Time) bool {
	day = day.In(dates.Paris)
	switch day.Weekday() {
	case time.Monday:
		_, ok := PublicHoliday(day.AddDate(0, 0, 1))
		return ok
	case time.Friday:
		_, ok := PublicHoliday(day.AddDate(0, 0, -1))
		return ok
	}
	return false
}
