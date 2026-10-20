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

func (s *Store) InsertSnapshot(ctx context.Context, snapshot models.WaitSnapshot) error {
	_, err := s.waitTimes.InsertOne(ctx, snapshot)
	return err
}

// LatestSnapshot returns the most recent wait times snapshot, or nil when there is none.
func (s *Store) LatestSnapshot(ctx context.Context) (*models.WaitSnapshot, error) {
	var snapshot models.WaitSnapshot
	err := s.waitTimes.FindOne(ctx, bson.D{}, options.FindOne().SetSort(bson.D{{Key: "observedAt", Value: -1}})).Decode(&snapshot)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &snapshot, nil
}

// Snapshots returns the snapshots taken in [from, to), oldest first, without
// the details of each ride.
func (s *Store) Snapshots(ctx context.Context, from time.Time, to time.Time) ([]models.WaitSnapshot, error) {
	cursor, err := s.waitTimes.Find(ctx,
		bson.D{{Key: "observedAt", Value: bson.D{{Key: "$gte", Value: from}, {Key: "$lt", Value: to}}}},
		options.Find().
			SetSort(bson.D{{Key: "observedAt", Value: 1}}).
			SetProjection(bson.D{{Key: "rides", Value: 0}}))
	if err != nil {
		return nil, err
	}
	snapshots := []models.WaitSnapshot{}
	if err = cursor.All(ctx, &snapshots); err != nil {
		return nil, err
	}
	return snapshots, nil
}
