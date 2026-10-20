// Package themeparks reads the schedule and live wait times of the park from the
// ThemeParks.wiki API (https://api.themeparks.wiki).
package themeparks

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"time"

	"github.com/romitou/previsix/calendar"
	"github.com/romitou/previsix/dates"
	"github.com/romitou/previsix/httputil"
)

type Client struct {
	Endpoint string
	ParkID   string
}

func (c Client) Name() string {
	return "themeparks"
}

func (c Client) entityURL(path string) string {
	return c.Endpoint + "/entity/" + url.PathEscape(c.ParkID) + "/" + path
}

type scheduleResponse struct {
	Schedule []struct {
		Date        string    `json:"date"`
		Type        string    `json:"type"`
		OpeningTime time.Time `json:"openingTime"`
		ClosingTime time.Time `json:"closingTime"`
	} `json:"schedule"`
}

// Slots returns the upcoming operating slots. ThemeParks.wiki only publishes
// the next few weeks, the official API is preferred when available.
func (c Client) Slots(ctx context.Context) ([]calendar.Slot, error) {
	var response scheduleResponse
	if err := httputil.GetJSON(ctx, c.entityURL("schedule"), &response); err != nil {
		return nil, err
	}

	var slots []calendar.Slot
	for _, item := range response.Schedule {
		var event bool
		switch item.Type {
		case "OPERATING":
		case "TICKETED_EVENT":
			event = true
		default:
			// EXTRA_HOURS, INFO, CLOSED...
			continue
		}
		if !item.ClosingTime.After(item.OpeningTime) {
			return nil, fmt.Errorf("invalid schedule item for %s: %s - %s", item.Date, item.OpeningTime, item.ClosingTime)
		}
		slots = append(slots, calendar.Slot{
			Date:  dates.DayKey(item.OpeningTime),
			Open:  item.OpeningTime.In(dates.Paris),
			Close: item.ClosingTime.In(dates.Paris),
			Event: event,
		})
	}
	return slots, nil
}

type liveResponse struct {
	LiveData []struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		EntityType string `json:"entityType"`
		Status     string `json:"status"`
		Queue      struct {
			Standby *struct {
				WaitTime *float64 `json:"waitTime"`
			} `json:"STANDBY"`
		} `json:"queue"`
	} `json:"liveData"`
}

// RideWait is the live state of an attraction.
type RideWait struct {
	ID     string `json:"id" bson:"id"`
	Name   string `json:"name" bson:"name"`
	Status string `json:"status" bson:"status"`
	// Standby wait time in minutes, nil when not reported.
	WaitTime *float64 `json:"waitTime,omitempty" bson:"waitTime,omitempty"`
}

func (r RideWait) Operating() bool {
	return r.Status == "OPERATING"
}

// Live returns the current state of every attraction of the park, sorted by name.
func (c Client) Live(ctx context.Context) ([]RideWait, error) {
	var response liveResponse
	if err := httputil.GetJSON(ctx, c.entityURL("live"), &response); err != nil {
		return nil, err
	}

	var rides []RideWait
	for _, entity := range response.LiveData {
		if entity.EntityType != "ATTRACTION" {
			continue
		}
		ride := RideWait{ID: entity.ID, Name: entity.Name, Status: entity.Status}
		if entity.Queue.Standby != nil {
			ride.WaitTime = entity.Queue.Standby.WaitTime
		}
		rides = append(rides, ride)
	}
	sort.Slice(rides, func(i, j int) bool { return rides[i].Name < rides[j].Name })
	return rides, nil
}
