package jobs

import (
	"math"
	"testing"
	"time"

	"github.com/romitou/previsix/calendar"
	"github.com/romitou/previsix/database/models"
	"github.com/romitou/previsix/dates"
	"github.com/romitou/previsix/forecast"
	"github.com/romitou/previsix/themeparks"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func at(day string, hour int) time.Time {
	d, _ := dates.ParseDay(day)
	return d.Add(time.Duration(hour) * time.Hour)
}

func TestMergeSlots(t *testing.T) {
	primary := []calendar.Slot{
		{Date: "2026-10-21", Open: at("2026-10-21", 10), Close: at("2026-10-21", 18)},
		{Date: "2026-10-21", Open: at("2026-10-21", 19), Close: at("2026-10-22", 1), Event: true},
	}
	secondary := []calendar.Slot{
		// Already covered by the primary source.
		{Date: "2026-10-21", Open: at("2026-10-21", 9), Close: at("2026-10-21", 18)},
		// Two operating periods on the same day are merged.
		{Date: "2026-10-23", Open: at("2026-10-23", 10), Close: at("2026-10-23", 12)},
		{Date: "2026-10-23", Open: at("2026-10-23", 14), Close: at("2026-10-23", 18)},
	}

	merged := mergeSlots(primary, secondary)
	if len(merged) != 3 {
		t.Fatalf("got %d slots, want 3: %+v", len(merged), merged)
	}
	if !merged[0].Open.Equal(at("2026-10-21", 10)) {
		t.Errorf("the primary source must win: %+v", merged[0])
	}
	if !merged[1].Event {
		t.Errorf("slots must be sorted by opening: %+v", merged)
	}
	if !merged[2].Open.Equal(at("2026-10-23", 10)) || !merged[2].Close.Equal(at("2026-10-23", 18)) {
		t.Errorf("unexpected merged slot %+v", merged[2])
	}
}

func TestPriority(t *testing.T) {
	now := at("2026-10-19", 12)
	ago := func(hours int) *time.Time {
		t := now.Add(-time.Duration(hours) * time.Hour)
		return &t
	}

	never := models.DayCalendar{StartDate: now.Add(1000 * time.Hour)}
	if !math.IsInf(Priority(never, now, 0.6), 1) {
		t.Error("a slot never forecast must come first")
	}

	// Same staleness: the closest slot comes first.
	near := models.DayCalendar{StartDate: now.Add(24 * time.Hour), LastForecast: ago(6)}
	far := models.DayCalendar{StartDate: now.Add(60 * 24 * time.Hour), LastForecast: ago(6)}
	if Priority(near, now, 0.6) <= Priority(far, now, 0.6) {
		t.Error("a close slot must have a higher priority than a distant one")
	}

	// A distant slot not forecast for a long time beats a close one just forecast.
	staleFar := models.DayCalendar{StartDate: now.Add(60 * 24 * time.Hour), LastForecast: ago(24 * 10)}
	freshNear := models.DayCalendar{StartDate: now.Add(24 * time.Hour), LastForecast: ago(1)}
	if Priority(staleFar, now, 0.6) <= Priority(freshNear, now, 0.6) {
		t.Error("staleness must eventually win over distance")
	}

	// Ongoing slots and forecasts in the future must not produce NaN.
	ongoing := models.DayCalendar{StartDate: now.Add(-2 * time.Hour), LastForecast: ago(-1)}
	if p := Priority(ongoing, now, 0.6); math.IsNaN(p) || p < 0 {
		t.Errorf("invalid priority %v", p)
	}
}

func TestSelectForecasts(t *testing.T) {
	now := at("2026-10-19", 12)
	recent := now.Add(-time.Hour)
	days := []models.DayCalendar{
		{Date: "a", StartDate: now.Add(48 * time.Hour), LastForecast: &recent},
		{Date: "b", StartDate: now.Add(72 * time.Hour)},
		{Date: "c", StartDate: now.Add(24 * time.Hour), LastForecast: &recent},
	}
	selected := selectForecasts(days, now, 0.6, 2)
	if len(selected) != 2 || selected[0].Date != "b" || selected[1].Date != "c" {
		t.Errorf("unexpected selection %+v", selected)
	}
	if len(selectForecasts(days, now, 0.6, 10)) != 3 {
		t.Error("the amount must be capped by the candidates")
	}
}

func TestSummarize(t *testing.T) {
	wait := func(v float64) *float64 { return &v }
	operating, avgWait := summarize([]themeparks.RideWait{
		{Status: "OPERATING", WaitTime: wait(10)},
		{Status: "OPERATING", WaitTime: wait(30)},
		{Status: "OPERATING"},
		{Status: "CLOSED", WaitTime: wait(90)},
	})
	if operating != 3 || avgWait == nil || *avgWait != 20 {
		t.Errorf("operating = %d, avgWait = %v", operating, avgWait)
	}
	if operating, avgWait = summarize([]themeparks.RideWait{{Status: "CLOSED"}}); operating != 0 || avgWait != nil {
		t.Errorf("closed park: operating = %d, avgWait = %v", operating, avgWait)
	}
}

func TestObserve(t *testing.T) {
	day := models.DayCalendar{ID: bson.NewObjectID(), StartDate: at("2026-07-14", 10), CloseDate: at("2026-07-14", 16)}
	var snapshots []models.WaitSnapshot
	// One snapshot every 30 minutes, from 9:00 to 16:30, waits growing with time.
	for minute := 0; minute <= 450; minute += 30 {
		wait := float64(minute) / 10
		snapshots = append(snapshots, models.WaitSnapshot{ObservedAt: at("2026-07-14", 9).Add(time.Duration(minute) * time.Minute), AvgWait: &wait})
	}
	snapshots = append(snapshots, models.WaitSnapshot{ObservedAt: at("2026-07-14", 12)})

	observation, ok := observe(day, snapshots)
	if !ok {
		t.Fatal("expected an observation")
	}
	// Snapshots from 10:00 to 15:30 (12 of them), waits from 6 to 39.
	if observation.Samples != 12 || observation.AvgWait != 22.5 {
		t.Errorf("samples = %d, avgWait = %v", observation.Samples, observation.AvgWait)
	}
	if len(observation.Profile) != forecast.ProfileBins || observation.Profile[0] != 6 || observation.Profile[forecast.ProfileBins-1] != 39 {
		t.Errorf("unexpected profile %v", observation.Profile)
	}
	if len(observation.WaitTimes) != 6 || observation.WaitTimes[0].Minutes != 7.5 {
		t.Errorf("unexpected hourly wait times %+v", observation.WaitTimes)
	}

	if _, ok = observe(day, snapshots[:4]); ok {
		t.Error("a handful of snapshots must not be enough")
	}
}
