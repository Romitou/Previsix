package jobs

import (
	"context"
	"log/slog"

	"github.com/romitou/previsix/database/models"
	"github.com/romitou/previsix/forecast"
	"github.com/romitou/previsix/holidays"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// MakeForecast forecasts a slot with the current model and stores the result.
func (r *Runner) MakeForecast(id bson.ObjectID) {
	ctx, cancel := r.context()
	defer cancel()

	day, err := r.Store.Slot(ctx, id)
	if err != nil {
		slog.Error("cannot load the slot to forecast", "id", id.Hex(), "error", err)
		_ = r.Store.ReleaseForecast(ctx, id)
		return
	}

	result, err := r.forecast(ctx, *day)
	if err != nil {
		slog.Error("cannot forecast", "date", day.Date, "error", err)
		_ = r.Store.ReleaseForecast(ctx, id)
		return
	}

	if _, err = r.Store.SaveForecast(ctx, result); err != nil {
		slog.Error("cannot save the forecast", "date", day.Date, "error", err)
		_ = r.Store.ReleaseForecast(ctx, id)
		return
	}
	slog.Debug("forecast made", "date", day.Date, "type", day.DayType, "avgWait", result.AvgWait, "crowdScore", result.CrowdScore)
}

// slotFeatures describes a slot with the known holidays and weather.
func (r *Runner) slotFeatures(ctx context.Context, day models.DayCalendar, periods []holidays.Period) (forecast.Features, error) {
	events, err := r.Store.EventDates(ctx, day.Date, day.Date)
	if err != nil {
		return forecast.Features{}, err
	}
	slot := forecast.SlotInfo{
		Date:       day.Date,
		Open:       day.StartDate,
		Close:      day.CloseDate,
		NightEvent: day.DayType == models.DayForecastTypeEvent,
		EventDay:   events[day.Date],
	}
	return forecast.BuildFeatures(slot, periods, r.Weather.Day(ctx, day.Date)), nil
}

func (r *Runner) forecast(ctx context.Context, day models.DayCalendar) (models.DayForecast, error) {
	periods, err := r.Store.Holidays(ctx)
	if err != nil {
		return models.DayForecast{}, err
	}
	features, err := r.slotFeatures(ctx, day, periods)
	if err != nil {
		return models.DayForecast{}, err
	}

	now := r.Now()
	model := forecast.Current()
	prediction := model.Predict(features, day.StartDate, day.CloseDate, now)
	return models.DayForecast{
		CalendarID:      day.ID,
		Date:            day.Date,
		AvgWait:         prediction.AvgWait,
		CrowdScore:      prediction.CrowdScore,
		ConfidenceScore: prediction.Confidence,
		WaitTimes:       prediction.WaitTimes,
		Features:        features,
		ModelSamples:    model.Samples,
		ForecastedAt:    now,
	}, nil
}
