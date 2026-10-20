package jobs

import (
	"context"
	"log/slog"
	"math"
	"time"

	"github.com/romitou/previsix/database/models"
	"github.com/romitou/previsix/forecast"
)

// Slots are observed for a few days after their end, in case the collector
// was late.
const observationWindow = 3 * 24 * time.Hour

// Minimum amount of snapshots for a slot to be observed.
const minSnapshots = 6

// observe summarizes the snapshots taken during a slot. It returns false when
// there are not enough of them.
func observe(day models.DayCalendar, snapshots []models.WaitSnapshot) (models.Observation, bool) {
	duration := day.CloseDate.Sub(day.StartDate)
	binSums := make([]float64, forecast.ProfileBins)
	binCounts := make([]int, forecast.ProfileBins)
	hourSums := map[time.Time]float64{}
	hourCounts := map[time.Time]int{}
	var hours []time.Time

	var sum float64
	var count int
	for _, snapshot := range snapshots {
		if snapshot.AvgWait == nil || snapshot.ObservedAt.Before(day.StartDate) || !snapshot.ObservedAt.Before(day.CloseDate) {
			continue
		}
		wait := *snapshot.AvgWait
		sum += wait
		count++

		bin := int(float64(forecast.ProfileBins) * float64(snapshot.ObservedAt.Sub(day.StartDate)) / float64(duration))
		bin = max(0, min(forecast.ProfileBins-1, bin))
		binSums[bin] += wait
		binCounts[bin]++

		hour := snapshot.ObservedAt.Truncate(time.Hour)
		if _, ok := hourCounts[hour]; !ok {
			hours = append(hours, hour)
		}
		hourSums[hour] += wait
		hourCounts[hour]++
	}
	if count < minSnapshots {
		return models.Observation{}, false
	}

	profile := make([]float64, forecast.ProfileBins)
	for i := range profile {
		profile[i] = -1
		if binCounts[i] > 0 {
			profile[i] = round(binSums[i] / float64(binCounts[i]))
		}
	}

	waitTimes := make([]forecast.HourlyWait, 0, len(hours))
	for _, hour := range hours {
		waitTimes = append(waitTimes, forecast.HourlyWait{Time: hour, Minutes: round(hourSums[hour] / float64(hourCounts[hour]))})
	}

	avgWait := sum / float64(count)
	return models.Observation{
		AvgWait:    round(avgWait),
		CrowdScore: round(forecast.CrowdScore(avgWait)),
		Samples:    count,
		Profile:    profile,
		WaitTimes:  waitTimes,
	}, true
}

func round(value float64) float64 {
	return math.Round(value*100) / 100
}

// Learn summarizes the wait times of the slots that just ended, then trains the
// model again with every observed slot.
func (r *Runner) Learn() {
	ctx, cancel := r.context()
	defer cancel()

	r.observeSlots(ctx)
	r.Train(ctx)
}

func (r *Runner) observeSlots(ctx context.Context) {
	now := r.Now()
	days, err := r.Store.SlotsToObserve(ctx, now.Add(-observationWindow), now)
	if err != nil {
		slog.Error("cannot list the slots to observe", "error", err)
		return
	}
	if len(days) == 0 {
		return
	}

	periods, err := r.Store.Holidays(ctx)
	if err != nil {
		slog.Error("cannot load the holidays", "error", err)
		return
	}

	for _, day := range days {
		snapshots, err := r.Store.Snapshots(ctx, day.StartDate, day.CloseDate)
		if err != nil {
			slog.Error("cannot load the wait times", "date", day.Date, "error", err)
			continue
		}
		observation, ok := observe(day, snapshots)
		if !ok {
			continue
		}

		if observation.Features, err = r.slotFeatures(ctx, day, periods); err != nil {
			slog.Error("cannot describe the slot", "date", day.Date, "error", err)
			continue
		}
		previous, err := r.Store.LastForecastBefore(ctx, day.ID, day.StartDate)
		if err != nil {
			slog.Error("cannot load the last forecast", "date", day.Date, "error", err)
			continue
		}
		if previous != nil {
			observation.ForecastAvgWait = &previous.AvgWait
		}
		observation.ObservedAt = now

		if err = r.Store.SetObservation(ctx, day.ID, observation); err != nil {
			slog.Error("cannot store the observation", "date", day.Date, "error", err)
			continue
		}
		slog.Info("slot observed", "date", day.Date, "type", day.DayType, "avgWait", observation.AvgWait, "samples", observation.Samples)
	}
}

// Train fits the model on every observed slot and makes it the current one.
func (r *Runner) Train(ctx context.Context) {
	days, err := r.Store.ObservedSlots(ctx)
	if err != nil {
		slog.Error("cannot load the observed slots", "error", err)
		return
	}

	samples := make([]forecast.Sample, 0, len(days))
	for _, day := range days {
		samples = append(samples, forecast.Sample{
			Features: day.Observed.Features,
			AvgWait:  day.Observed.AvgWait,
			Profile:  day.Observed.Profile,
		})
	}

	model, err := forecast.Train(samples, r.Config.Model.Regularization, r.Config.Model.MinTrainingDays)
	if err != nil {
		slog.Error("cannot train the model", "error", err)
		return
	}
	forecast.SetCurrent(model)
	slog.Info("model trained", "samples", model.Samples, "sigma", model.Sigma, "trainingMae", model.TrainingMAE)
}
