package database

import (
	"context"
	"errors"
	"time"

	"github.com/romitou/previsix/database/models"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// SaveForecast stores a new forecast of a slot and releases its pending flag.
func (s *Store) SaveForecast(ctx context.Context, forecast models.DayForecast) (models.DayForecast, error) {
	inserted, err := s.forecasts.InsertOne(ctx, forecast)
	if err != nil {
		return forecast, err
	}
	forecast.ID = inserted.InsertedID.(bson.ObjectID)

	_, err = s.calendar.UpdateByID(ctx, forecast.CalendarID, bson.D{
		{Key: "$set", Value: bson.D{
			{Key: "latestForecast", Value: forecast},
			{Key: "lastForecast", Value: forecast.ForecastedAt},
			{Key: "forecastPending", Value: false},
		}},
		{Key: "$unset", Value: bson.D{{Key: "pendingSince", Value: ""}}},
	})
	return forecast, err
}

// Forecasts returns the forecast history of a slot, most recent first.
func (s *Store) Forecasts(ctx context.Context, calendarID bson.ObjectID, limit int64) ([]models.DayForecast, error) {
	cursor, err := s.forecasts.Find(ctx,
		bson.D{{Key: "calendarId", Value: calendarID}},
		options.Find().SetSort(bson.D{{Key: "forecastedAt", Value: -1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	forecasts := []models.DayForecast{}
	if err = cursor.All(ctx, &forecasts); err != nil {
		return nil, err
	}
	return forecasts, nil
}

// LastForecastBefore returns the latest forecast of a slot made before a time,
// or nil when there is none.
func (s *Store) LastForecastBefore(ctx context.Context, calendarID bson.ObjectID, before time.Time) (*models.DayForecast, error) {
	var forecast models.DayForecast
	err := s.forecasts.FindOne(ctx,
		bson.D{{Key: "calendarId", Value: calendarID}, {Key: "forecastedAt", Value: bson.D{{Key: "$lt", Value: before}}}},
		options.FindOne().SetSort(bson.D{{Key: "forecastedAt", Value: -1}})).Decode(&forecast)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &forecast, nil
}
