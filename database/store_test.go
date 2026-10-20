package database

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/romitou/previsix/calendar"
	"github.com/romitou/previsix/database/models"
	"github.com/romitou/previsix/dates"
	"github.com/romitou/previsix/holidays"
)

// NewTestStore connects to the database of PREVISIX_TEST_MONGO_URI, in a
// dedicated database dropped at the end of the test.
func NewTestStore(t *testing.T) *Store {
	t.Helper()
	uri := os.Getenv("PREVISIX_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("PREVISIX_TEST_MONGO_URI is not set")
	}
	ctx := context.Background()
	store, err := Connect(ctx, uri, "previsix_test_"+strconv.FormatInt(time.Now().UnixNano(), 36))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = store.Drop(ctx)
		_ = store.Close(ctx)
	})
	return store
}

func at(day string, hour int) time.Time {
	d, _ := dates.ParseDay(day)
	return d.Add(time.Duration(hour) * time.Hour)
}

func TestSyncCalendar(t *testing.T) {
	store := NewTestStore(t)
	ctx := context.Background()
	now := at("2026-10-19", 8)

	slots := []calendar.Slot{
		{Date: "2026-10-19", Open: at("2026-10-19", 10), Close: at("2026-10-19", 18)},
		{Date: "2026-10-20", Open: at("2026-10-20", 10), Close: at("2026-10-20", 18)},
		{Date: "2026-10-20", Open: at("2026-10-20", 19), Close: at("2026-10-21", 1), Event: true},
		{Date: "2026-10-21", Open: at("2026-10-21", 10), Close: at("2026-10-21", 18)},
	}
	result, err := store.SyncCalendar(ctx, slots, "test", "2026-10-19", now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 4 {
		t.Fatalf("unexpected result %+v", result)
	}

	// Same calendar: nothing changes.
	if result, err = store.SyncCalendar(ctx, slots, "test", "2026-10-19", now); err != nil || result != (SyncResult{}) {
		t.Fatalf("unexpected result %+v (%v)", result, err)
	}

	// New hours on the 20th, the 21st disappears.
	changed := []calendar.Slot{
		slots[0],
		{Date: "2026-10-20", Open: at("2026-10-20", 9), Close: at("2026-10-20", 19)},
		slots[2],
		{Date: "2026-10-22", Open: at("2026-10-22", 10), Close: at("2026-10-22", 18)},
	}
	if result, err = store.SyncCalendar(ctx, changed, "test", "2026-10-19", now); err != nil {
		t.Fatal(err)
	}
	if result.Created != 1 || result.Updated != 1 || result.Deactivated != 1 {
		t.Fatalf("unexpected result %+v", result)
	}

	days, err := store.Days(ctx, "2026-10-19", "2026-10-31")
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 4 {
		t.Fatalf("got %d active slots, want 4", len(days))
	}
	if !days[1].StartDate.Equal(at("2026-10-20", 9)) {
		t.Errorf("hours not updated: %+v", days[1])
	}

	events, err := store.EventDates(ctx, "2026-10-19", "2026-10-31")
	if err != nil || len(events) != 1 || !events["2026-10-20"] {
		t.Errorf("unexpected events %v (%v)", events, err)
	}
	all, err := store.Day(ctx, "2026-10-21")
	if err != nil || len(all) != 1 || all[0].Active {
		t.Errorf("the 21st must be kept but inactive: %+v (%v)", all, err)
	}
}

func TestForecastLifecycle(t *testing.T) {
	store := NewTestStore(t)
	ctx := context.Background()
	now := at("2026-10-19", 8)
	timeout := 15 * time.Minute

	_, err := store.SyncCalendar(ctx, []calendar.Slot{
		{Date: "2026-10-18", Open: at("2026-10-18", 10), Close: at("2026-10-18", 18)},
		{Date: "2026-10-19", Open: at("2026-10-19", 10), Close: at("2026-10-19", 18)},
	}, "test", "", now)
	if err != nil {
		t.Fatal(err)
	}

	candidates, err := store.ForecastCandidates(ctx, now, timeout)
	if err != nil || len(candidates) != 1 || candidates[0].Date != "2026-10-19" {
		t.Fatalf("past slots must not be candidates: %+v (%v)", candidates, err)
	}
	id := candidates[0].ID

	if claimed, err := store.ClaimForecast(ctx, id, now, timeout); err != nil || !claimed {
		t.Fatalf("claim failed: %v", err)
	}
	if claimed, _ := store.ClaimForecast(ctx, id, now, timeout); claimed {
		t.Fatal("a slot must not be claimed twice")
	}
	if candidates, _ = store.ForecastCandidates(ctx, now, timeout); len(candidates) != 0 {
		t.Fatal("a pending slot must not be a candidate")
	}
	// A forgotten pending forecast becomes a candidate again.
	if candidates, _ = store.ForecastCandidates(ctx, now.Add(time.Hour), timeout); len(candidates) != 1 {
		t.Fatal("a stale pending slot must be a candidate")
	}

	saved, err := store.SaveForecast(ctx, models.DayForecast{CalendarID: id, Date: "2026-10-19", AvgWait: 25, ForecastedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	slot, err := store.Slot(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if slot.ForecastPending || slot.LatestForecast == nil || slot.LatestForecast.ID != saved.ID || slot.LastForecast == nil {
		t.Fatalf("forecast not saved on the slot: %+v", slot)
	}

	_, _ = store.SaveForecast(ctx, models.DayForecast{CalendarID: id, Date: "2026-10-19", AvgWait: 30, ForecastedAt: at("2026-10-19", 12)})
	history, err := store.Forecasts(ctx, id, 10)
	if err != nil || len(history) != 2 || history[0].AvgWait != 30 {
		t.Fatalf("unexpected history %+v (%v)", history, err)
	}
	before, err := store.LastForecastBefore(ctx, id, slot.StartDate)
	if err != nil || before == nil || before.AvgWait != 25 {
		t.Fatalf("unexpected forecast before the opening %+v (%v)", before, err)
	}

	if _, err = store.ClaimForecast(ctx, id, now, timeout); err != nil {
		t.Fatal(err)
	}
	if reset, err := store.ResetPending(ctx); err != nil || reset != 1 {
		t.Fatalf("reset %d (%v)", reset, err)
	}
}

func TestObservationsAndSnapshots(t *testing.T) {
	store := NewTestStore(t)
	ctx := context.Background()

	_, err := store.SyncCalendar(ctx, []calendar.Slot{
		{Date: "2026-10-18", Open: at("2026-10-18", 10), Close: at("2026-10-18", 18)},
	}, "test", "", at("2026-10-17", 0))
	if err != nil {
		t.Fatal(err)
	}

	for hour := 9; hour < 20; hour++ {
		wait := float64(hour)
		if err = store.InsertSnapshot(ctx, models.WaitSnapshot{ObservedAt: at("2026-10-18", hour), Date: "2026-10-18", AvgWait: &wait, Operating: 10}); err != nil {
			t.Fatal(err)
		}
	}
	snapshots, err := store.Snapshots(ctx, at("2026-10-18", 10), at("2026-10-18", 18))
	if err != nil || len(snapshots) != 8 {
		t.Fatalf("got %d snapshots (%v)", len(snapshots), err)
	}
	latest, err := store.LatestSnapshot(ctx)
	if err != nil || latest == nil || *latest.AvgWait != 19 {
		t.Fatalf("unexpected latest snapshot %+v (%v)", latest, err)
	}

	toObserve, err := store.SlotsToObserve(ctx, at("2026-10-16", 0), at("2026-10-19", 0))
	if err != nil || len(toObserve) != 1 {
		t.Fatalf("got %d slots to observe (%v)", len(toObserve), err)
	}
	if err = store.SetObservation(ctx, toObserve[0].ID, models.Observation{AvgWait: 14, Samples: 8}); err != nil {
		t.Fatal(err)
	}
	if toObserve, _ = store.SlotsToObserve(ctx, at("2026-10-16", 0), at("2026-10-19", 0)); len(toObserve) != 0 {
		t.Fatal("an observed slot must not be observed again")
	}
	observed, err := store.ObservedSlots(ctx)
	if err != nil || len(observed) != 1 || observed[0].Observed.AvgWait != 14 {
		t.Fatalf("unexpected observed slots %+v (%v)", observed, err)
	}
}

func TestHolidays(t *testing.T) {
	store := NewTestStore(t)
	ctx := context.Background()
	periods := []holidays.Period{
		{Zone: "Zone C", Description: "Toussaint", Start: "2026-10-17", End: "2026-11-02"},
		{Zone: "Zone B", Description: "Toussaint", Start: "2026-10-17", End: "2026-11-02"},
	}
	for range 2 {
		if err := store.SaveHolidays(ctx, periods); err != nil {
			t.Fatal(err)
		}
	}
	saved, err := store.Holidays(ctx)
	if err != nil || len(saved) != 2 {
		t.Fatalf("got %+v (%v)", saved, err)
	}
}
