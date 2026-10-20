package database

import (
	"context"

	"github.com/romitou/previsix/holidays"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// SaveHolidays upserts school holidays periods. Periods are never deleted, so
// that past ones remain available to train the model.
func (s *Store) SaveHolidays(ctx context.Context, periods []holidays.Period) error {
	if len(periods) == 0 {
		return nil
	}
	writes := make([]mongo.WriteModel, 0, len(periods))
	for _, period := range periods {
		writes = append(writes, mongo.NewReplaceOneModel().
			SetFilter(bson.D{
				{Key: "zone", Value: period.Zone},
				{Key: "start", Value: period.Start},
				{Key: "end", Value: period.End},
				{Key: "description", Value: period.Description},
			}).
			SetReplacement(period).
			SetUpsert(true))
	}
	_, err := s.holidays.BulkWrite(ctx, writes)
	return err
}

// Holidays returns every known school holidays period.
func (s *Store) Holidays(ctx context.Context) ([]holidays.Period, error) {
	cursor, err := s.holidays.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "start", Value: 1}}))
	if err != nil {
		return nil, err
	}
	periods := []holidays.Period{}
	if err = cursor.All(ctx, &periods); err != nil {
		return nil, err
	}
	return periods, nil
}
