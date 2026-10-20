package jobs

import (
	"log/slog"
	"math"
	"sort"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/romitou/previsix/database/models"
)

// Priority tells how much a slot needs a new forecast: the time elapsed since
// its last forecast, divided by its distance raised to the priority exponent.
// A slot never forecast comes first, and among the others, close slots are
// refreshed more often than distant ones.
func Priority(day models.DayCalendar, now time.Time, exponent float64) float64 {
	if day.LastForecast == nil {
		return math.Inf(1)
	}
	staleness := math.Max(0, now.Sub(*day.LastForecast).Hours())
	horizon := math.Max(1, day.StartDate.Sub(now).Hours())
	return staleness / math.Pow(horizon, exponent)
}

// selectForecasts returns the amount slots with the highest priority.
func selectForecasts(days []models.DayCalendar, now time.Time, exponent float64, amount int) []models.DayCalendar {
	priorities := make(map[int]float64, len(days))
	indexes := make([]int, len(days))
	for i, day := range days {
		indexes[i] = i
		priorities[i] = Priority(day, now, exponent)
	}
	sort.SliceStable(indexes, func(a, b int) bool {
		return priorities[indexes[a]] > priorities[indexes[b]]
	})

	selected := make([]models.DayCalendar, 0, min(amount, len(days)))
	for _, index := range indexes[:min(amount, len(days))] {
		selected = append(selected, days[index])
	}
	return selected
}

// QueueForecasts picks the slots needing a forecast the most, and schedules
// their forecast on the forecast scheduler, which limits the concurrency.
func (r *Runner) QueueForecasts(forecastScheduler gocron.Scheduler) {
	ctx, cancel := r.context()
	defer cancel()

	now := r.Now()
	pendingTimeout := time.Duration(r.Config.Forecasts.PendingTimeout) * time.Minute
	candidates, err := r.Store.ForecastCandidates(ctx, now, pendingTimeout)
	if err != nil {
		slog.Error("cannot list the forecast candidates", "error", err)
		return
	}

	queued := 0
	for _, day := range selectForecasts(candidates, now, r.Config.Forecasts.PriorityExponent, r.Config.Forecasts.Amount) {
		claimed, err := r.Store.ClaimForecast(ctx, day.ID, now, pendingTimeout)
		if err != nil {
			slog.Error("cannot claim the forecast", "date", day.Date, "error", err)
			continue
		}
		if !claimed {
			continue
		}

		_, err = forecastScheduler.NewJob(
			gocron.OneTimeJob(gocron.OneTimeJobStartImmediately()),
			gocron.NewTask(r.MakeForecast, day.ID),
			gocron.WithName("forecast-"+day.ID.Hex()),
		)
		if err != nil {
			slog.Error("cannot schedule the forecast", "date", day.Date, "error", err)
			_ = r.Store.ReleaseForecast(ctx, day.ID)
			continue
		}
		queued++
	}
	slog.Info("forecasts queued", "queued", queued, "candidates", len(candidates))
}
