package asterix

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/romitou/previsix/calendar"
	"github.com/romitou/previsix/dates"
	"github.com/romitou/previsix/httputil"
)

type calendarItem struct {
	Day   string `json:"day"`
	Times string `json:"times"`
}

type calendarResponse struct {
	Data struct {
		CalendarItems []calendarItem `json:"calendar"`
	} `json:"data"`
	Errors []graphQlError `json:"errors"`
}

// Client reads the opening calendar from the official Parc Astérix GraphQL API.
type Client struct {
	Endpoint string
	Query    string
}

func (c Client) Name() string {
	return "asterix"
}

func (c Client) fetchCalendar(ctx context.Context) ([]calendarItem, error) {
	var response calendarResponse
	err := httputil.PostJSON(ctx, c.Endpoint, GraphQlQuery{Query: c.Query, Variables: map[string]any{}}, &response)
	if err != nil {
		return nil, err
	}
	if len(response.Errors) > 0 {
		return nil, fmt.Errorf("graphql error: %s", response.Errors[0].Message)
	}
	return response.Data.CalendarItems, nil
}

func (c Client) Slots(ctx context.Context) ([]calendar.Slot, error) {
	items, err := c.fetchCalendar(ctx)
	if err != nil {
		return nil, err
	}

	var slots []calendar.Slot
	var errs []error
	for _, item := range items {
		daySlots, err := ParseTimes(item.Day, item.Times)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		slots = append(slots, daySlots...)
	}
	if len(slots) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return slots, nil
}

// Matches "10h", "10h30", "10 H 30", "10:30".
var hourPattern = regexp.MustCompile(`^(\d{1,2})\s*[hH:]\s*(\d{2})?$`)

func parseHour(value string) (hour int, minute int, err error) {
	matches := hourPattern.FindStringSubmatch(strings.TrimSpace(value))
	if matches == nil {
		return 0, 0, fmt.Errorf("invalid hour %q", value)
	}
	hour, _ = strconv.Atoi(matches[1])
	if matches[2] != "" {
		minute, _ = strconv.Atoi(matches[2])
	}
	if hour > 24 || minute > 59 {
		return 0, 0, fmt.Errorf("invalid hour %q", value)
	}
	return hour, minute, nil
}

// ParseTimes converts a calendar entry such as ("2024-10-26", "10h - 18h et
// 19h30 - 1h") into operating slots. Closed days return no slot.
func ParseTimes(day string, times string) ([]calendar.Slot, error) {
	date, err := dates.ParseDay(day)
	if err != nil {
		return nil, err
	}

	times = strings.TrimSpace(times)
	if times == "" || strings.Contains(strings.ToLower(times), "ferm") {
		return nil, nil
	}

	var slots []calendar.Slot
	for _, period := range strings.Split(times, " et ") {
		bounds := strings.FieldsFunc(period, func(r rune) bool { return r == '-' || r == '–' || r == '—' })
		if len(bounds) != 2 {
			return nil, fmt.Errorf("invalid time format for %s: %q", day, times)
		}

		openingHour, openingMinute, err := parseHour(bounds[0])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", day, err)
		}
		closingHour, closingMinute, err := parseHour(bounds[1])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", day, err)
		}

		openingTime := time.Date(date.Year(), date.Month(), date.Day(), openingHour, openingMinute, 0, 0, dates.Paris)
		closingTime := time.Date(date.Year(), date.Month(), date.Day(), closingHour, closingMinute, 0, 0, dates.Paris)

		// A closing time before the opening time means the park closes after midnight.
		if !closingTime.After(openingTime) {
			closingTime = closingTime.AddDate(0, 0, 1)
		}

		// Second periods of a day and openings ending after midnight are the
		// ticketed night events (Peur sur le Parc, ...).
		nextMidnight := date.AddDate(0, 0, 1)
		slots = append(slots, calendar.Slot{
			Date:  day,
			Open:  openingTime,
			Close: closingTime,
			Event: len(slots) > 0 || closingTime.After(nextMidnight),
		})
	}
	return slots, nil
}
