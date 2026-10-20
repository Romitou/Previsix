// Package dates gathers the calendar helpers shared by the whole project. Every
// "day" handled by Previsix is a day in the park's timezone (Europe/Paris).
package dates

import (
	"time"
	// Embed the timezone database so the binary works in minimal containers.
	_ "time/tzdata"
)

const DayFormat = "2006-01-02"

var Paris = mustLoad("Europe/Paris")

func mustLoad(name string) *time.Location {
	location, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return location
}

// DayKey returns the Paris calendar day of t, formatted as YYYY-MM-DD.
func DayKey(t time.Time) string {
	return t.In(Paris).Format(DayFormat)
}

// ParseDay parses a YYYY-MM-DD day and returns its midnight in Paris.
func ParseDay(day string) (time.Time, error) {
	return time.ParseInLocation(DayFormat, day, Paris)
}

// StartOfDay returns the midnight in Paris of the day containing t.
func StartOfDay(t time.Time) time.Time {
	t = t.In(Paris)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, Paris)
}

// AddDays adds n calendar days to a YYYY-MM-DD day.
func AddDays(day string, n int) string {
	t, err := ParseDay(day)
	if err != nil {
		return day
	}
	return t.AddDate(0, 0, n).Format(DayFormat)
}
