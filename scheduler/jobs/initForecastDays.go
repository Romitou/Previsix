package jobs

import (
	"log/slog"
	"sort"
	"strings"

	"github.com/romitou/previsix/asterix"
	"github.com/romitou/previsix/calendar"
	"github.com/romitou/previsix/config"
	"github.com/romitou/previsix/dates"
)

func asterixSource(c *config.Config) calendar.Source {
	return asterix.Client{Endpoint: c.Asterix.Endpoint, Query: c.Asterix.Queries.Calendar}
}

// mergeSlots combines the slots of several sources. A source only fills the
// days missing from the previous ones. Slots of the same day and type are
// merged into a single one spanning all of them.
func mergeSlots(sources ...[]calendar.Slot) []calendar.Slot {
	type key struct {
		date  string
		event bool
	}
	merged := map[key]calendar.Slot{}
	coveredDays := map[string]bool{}

	for _, slots := range sources {
		sourceDays := map[string]bool{}
		for _, slot := range slots {
			if coveredDays[slot.Date] {
				continue
			}
			sourceDays[slot.Date] = true
			k := key{slot.Date, slot.Event}
			if existing, ok := merged[k]; ok {
				if existing.Open.Before(slot.Open) {
					slot.Open = existing.Open
				}
				if existing.Close.After(slot.Close) {
					slot.Close = existing.Close
				}
			}
			merged[k] = slot
		}
		for day := range sourceDays {
			coveredDays[day] = true
		}
	}

	result := make([]calendar.Slot, 0, len(merged))
	for _, slot := range merged {
		result = append(result, slot)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Open.Before(result[j].Open) })
	return result
}

// SyncCalendar fetches the opening calendar and stores the upcoming slots.
func (r *Runner) SyncCalendar() {
	ctx, cancel := r.context()
	defer cancel()

	now := r.Now()
	today := dates.DayKey(now)

	var fetched [][]calendar.Slot
	var names []string
	complete := true
	for _, source := range r.Calendars {
		slots, err := source.Slots(ctx)
		if err != nil {
			slog.Error("cannot fetch the calendar", "source", source.Name(), "error", err)
			complete = false
			continue
		}
		fetched = append(fetched, slots)
		names = append(names, source.Name())
	}
	if len(fetched) == 0 {
		return
	}

	var upcoming []calendar.Slot
	for _, slot := range mergeSlots(fetched...) {
		if slot.Date >= today {
			upcoming = append(upcoming, slot)
		}
	}

	// Slots missing from the calendar are only deactivated when every source
	// answered, otherwise a failing source would hide valid days.
	deactivateFrom := ""
	if complete {
		deactivateFrom = today
	}
	result, err := r.Store.SyncCalendar(ctx, upcoming, strings.Join(names, "+"), deactivateFrom, now)
	if err != nil {
		slog.Error("cannot store the calendar", "error", err)
		return
	}
	slog.Info("calendar synchronized", "slots", len(upcoming), "created", result.Created, "updated", result.Updated, "deactivated", result.Deactivated)
}
