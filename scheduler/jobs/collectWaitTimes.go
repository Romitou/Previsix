package jobs

import (
	"log/slog"

	"github.com/romitou/previsix/database/models"
	"github.com/romitou/previsix/dates"
	"github.com/romitou/previsix/themeparks"
)

// summarize computes the amount of operating attractions and their average
// standby wait time.
func summarize(rides []themeparks.RideWait) (operating int, avgWait *float64) {
	var sum float64
	var count int
	for _, ride := range rides {
		if !ride.Operating() {
			continue
		}
		operating++
		if ride.WaitTime != nil && *ride.WaitTime >= 0 {
			sum += *ride.WaitTime
			count++
		}
	}
	if count == 0 {
		return operating, nil
	}
	average := sum / float64(count)
	return operating, &average
}

// CollectWaitTimes stores a snapshot of the live wait times while the park is open.
func (r *Runner) CollectWaitTimes() {
	ctx, cancel := r.context()
	defer cancel()

	rides, err := r.ThemeParks.Live(ctx)
	if err != nil {
		slog.Warn("cannot fetch the live wait times", "error", err)
		return
	}

	operating, avgWait := summarize(rides)
	if operating == 0 {
		// The park is closed.
		return
	}

	now := r.Now()
	err = r.Store.InsertSnapshot(ctx, models.WaitSnapshot{
		ObservedAt: now,
		Date:       dates.DayKey(now),
		Rides:      rides,
		Operating:  operating,
		AvgWait:    avgWait,
	})
	if err != nil {
		slog.Error("cannot store the wait times", "error", err)
	}
}
