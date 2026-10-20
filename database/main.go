package database

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/connstring"
)

const defaultDatabase = "previsix"

// Store gives access to the Previsix collections.
type Store struct {
	client    *mongo.Client
	calendar  *mongo.Collection
	forecasts *mongo.Collection
	waitTimes *mongo.Collection
	holidays  *mongo.Collection
}

// Connect connects to MongoDB and makes sure the indexes exist. When name is
// empty, the database of the URI is used, or "previsix".
func Connect(ctx context.Context, uri string, name string) (*Store, error) {
	if uri == "" {
		return nil, errors.New("mongodb uri is empty")
	}
	if name == "" {
		parsed, err := connstring.ParseAndValidate(uri)
		if err != nil {
			return nil, err
		}
		name = parsed.Database
	}
	if name == "" {
		name = defaultDatabase
	}

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err = client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}

	db := client.Database(name)
	store := &Store{
		client:    client,
		calendar:  db.Collection("calendar"),
		forecasts: db.Collection("forecasts"),
		waitTimes: db.Collection("waittimes"),
		holidays:  db.Collection("holidays"),
	}
	if err = store.ensureIndexes(ctx); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}
	return store, nil
}

func (s *Store) ensureIndexes(ctx context.Context) error {
	indexes := map[*mongo.Collection][]mongo.IndexModel{
		s.calendar: {
			{Keys: bson.D{{Key: "date", Value: 1}, {Key: "dayType", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "active", Value: 1}, {Key: "endDate", Value: 1}}},
		},
		s.forecasts: {
			{Keys: bson.D{{Key: "calendarId", Value: 1}, {Key: "forecastedAt", Value: -1}}},
		},
		s.waitTimes: {
			{Keys: bson.D{{Key: "observedAt", Value: 1}}},
		},
		s.holidays: {
			{Keys: bson.D{{Key: "zone", Value: 1}, {Key: "start", Value: 1}, {Key: "end", Value: 1}, {Key: "description", Value: 1}}, Options: options.Index().SetUnique(true)},
		},
	}
	for collection, models := range indexes {
		if _, err := collection.Indexes().CreateMany(ctx, models); err != nil {
			return err
		}
	}
	return nil
}

// Ping checks that the database is reachable.
func (s *Store) Ping(ctx context.Context) error {
	return s.client.Ping(ctx, nil)
}

func (s *Store) Close(ctx context.Context) error {
	return s.client.Disconnect(ctx)
}

// Drop removes every Previsix collection. Only meant for tests.
func (s *Store) Drop(ctx context.Context) error {
	return s.calendar.Database().Drop(ctx)
}
