package database

import (
	"context"
	"errors"
	"time"

	"github.com/romitou/previsix/calendar"
	"github.com/romitou/previsix/database/models"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func slotType(slot calendar.Slot) models.DayForecastType {
	if slot.Event {
		return models.DayForecastTypeEvent
	}
	return models.DayForecastTypeNormal
}

// SyncResult reports what a calendar synchronization changed.
type SyncResult struct {
	Created     int
	Updated     int
	Deactivated int
}

// SyncCalendar upserts the slots, keyed by day and type. When deactivateFrom
// is not empty, the active slots from that day onwards and up to the last
// synchronized day that are not part of slots are deactivated: they were
// removed from the official calendar.
func (s *Store) SyncCalendar(ctx context.Context, slots []calendar.Slot, source string, deactivateFrom string, now time.Time) (SyncResult, error) {
	var result SyncResult
	var lastDate string
	var seen []bson.ObjectID

	for _, slot := range slots {
		if slot.Date > lastDate {
			lastDate = slot.Date
		}
		filter := bson.D{{Key: "date", Value: slot.Date}, {Key: "dayType", Value: slotType(slot)}}

		var existing models.DayCalendar
		err := s.calendar.FindOne(ctx, filter).Decode(&existing)
		switch {
		case errors.Is(err, mongo.ErrNoDocuments):
			inserted, err := s.calendar.InsertOne(ctx, models.DayCalendar{
				Date:      slot.Date,
				DayType:   slotType(slot),
				StartDate: slot.Open,
				CloseDate: slot.Close,
				Active:    true,
				Source:    source,
				CreatedAt: now,
				UpdatedAt: now,
			})
			if err != nil {
				return result, err
			}
			seen = append(seen, inserted.InsertedID.(bson.ObjectID))
			result.Created++
		case err != nil:
			return result, err
		default:
			seen = append(seen, existing.ID)
			if existing.Active && existing.StartDate.Equal(slot.Open) && existing.CloseDate.Equal(slot.Close) {
				continue
			}
			// New opening hours: the previous forecast is outdated.
			_, err = s.calendar.UpdateByID(ctx, existing.ID, bson.D{
				{Key: "$set", Value: bson.D{
					{Key: "startDate", Value: slot.Open},
					{Key: "endDate", Value: slot.Close},
					{Key: "active", Value: true},
					{Key: "source", Value: source},
					{Key: "updatedAt", Value: now},
				}},
				{Key: "$unset", Value: bson.D{{Key: "lastForecast", Value: ""}}},
			})
			if err != nil {
				return result, err
			}
			result.Updated++
		}
	}

	if deactivateFrom != "" && lastDate >= deactivateFrom {
		update, err := s.calendar.UpdateMany(ctx, bson.D{
			{Key: "active", Value: true},
			{Key: "date", Value: bson.D{{Key: "$gte", Value: deactivateFrom}, {Key: "$lte", Value: lastDate}}},
			{Key: "_id", Value: bson.D{{Key: "$nin", Value: seen}}},
		}, bson.D{{Key: "$set", Value: bson.D{{Key: "active", Value: false}, {Key: "updatedAt", Value: now}}}})
		if err != nil {
			return result, err
		}
		result.Deactivated = int(update.ModifiedCount)
	}
	return result, nil
}

// Days returns the active slots between two days (inclusive), sorted by opening.
func (s *Store) Days(ctx context.Context, from string, to string) ([]models.DayCalendar, error) {
	return s.findCalendar(ctx, bson.D{
		{Key: "active", Value: true},
		{Key: "date", Value: bson.D{{Key: "$gte", Value: from}, {Key: "$lte", Value: to}}},
	})
}

// Day returns the slots of a day, including the deactivated ones.
func (s *Store) Day(ctx context.Context, date string) ([]models.DayCalendar, error) {
	return s.findCalendar(ctx, bson.D{{Key: "date", Value: date}})
}

// Slot returns a slot by its identifier.
func (s *Store) Slot(ctx context.Context, id bson.ObjectID) (*models.DayCalendar, error) {
	var day models.DayCalendar
	if err := s.calendar.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&day); err != nil {
		return nil, err
	}
	return &day, nil
}

func (s *Store) findCalendar(ctx context.Context, filter bson.D) ([]models.DayCalendar, error) {
	cursor, err := s.calendar.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "startDate", Value: 1}}))
	if err != nil {
		return nil, err
	}
	days := []models.DayCalendar{}
	if err = cursor.All(ctx, &days); err != nil {
		return nil, err
	}
	return days, nil
}

// EventDates returns the days having an active night event between two days.
func (s *Store) EventDates(ctx context.Context, from string, to string) (map[string]bool, error) {
	days, err := s.findCalendar(ctx, bson.D{
		{Key: "dayType", Value: models.DayForecastTypeEvent},
		{Key: "active", Value: true},
		{Key: "date", Value: bson.D{{Key: "$gte", Value: from}, {Key: "$lte", Value: to}}},
	})
	if err != nil {
		return nil, err
	}
	events := make(map[string]bool, len(days))
	for _, day := range days {
		events[day.Date] = true
	}
	return events, nil
}

// ForecastCandidates returns the active slots that are not over yet and that
// are not being forecast, or whose forecast has been pending for too long.
func (s *Store) ForecastCandidates(ctx context.Context, now time.Time, pendingTimeout time.Duration) ([]models.DayCalendar, error) {
	filter := bson.D{
		{Key: "active", Value: true},
		{Key: "endDate", Value: bson.D{{Key: "$gt", Value: now}}},
		{Key: "$or", Value: bson.A{
			bson.D{{Key: "forecastPending", Value: false}},
			bson.D{{Key: "pendingSince", Value: bson.D{{Key: "$lt", Value: now.Add(-pendingTimeout)}}}},
		}},
	}
	return s.findCalendar(ctx, filter)
}

// ClaimForecast atomically marks a slot as pending. It returns false when the
// slot is already being forecast by someone else.
func (s *Store) ClaimForecast(ctx context.Context, id bson.ObjectID, now time.Time, pendingTimeout time.Duration) (bool, error) {
	result, err := s.calendar.UpdateOne(ctx, bson.D{
		{Key: "_id", Value: id},
		{Key: "$or", Value: bson.A{
			bson.D{{Key: "forecastPending", Value: false}},
			bson.D{{Key: "pendingSince", Value: bson.D{{Key: "$lt", Value: now.Add(-pendingTimeout)}}}},
		}},
	}, bson.D{{Key: "$set", Value: bson.D{{Key: "forecastPending", Value: true}, {Key: "pendingSince", Value: now}}}})
	if err != nil {
		return false, err
	}
	return result.ModifiedCount == 1, nil
}

// ReleaseForecast clears the pending flag of a slot without saving a forecast.
func (s *Store) ReleaseForecast(ctx context.Context, id bson.ObjectID) error {
	_, err := s.calendar.UpdateByID(ctx, id, bson.D{
		{Key: "$set", Value: bson.D{{Key: "forecastPending", Value: false}}},
		{Key: "$unset", Value: bson.D{{Key: "pendingSince", Value: ""}}},
	})
	return err
}

// ResetPending clears every pending flag. Called at startup, since the forecast
// jobs of a previous process are lost.
func (s *Store) ResetPending(ctx context.Context) (int, error) {
	result, err := s.calendar.UpdateMany(ctx, bson.D{{Key: "forecastPending", Value: true}}, bson.D{
		{Key: "$set", Value: bson.D{{Key: "forecastPending", Value: false}}},
		{Key: "$unset", Value: bson.D{{Key: "pendingSince", Value: ""}}},
	})
	if err != nil {
		return 0, err
	}
	return int(result.ModifiedCount), nil
}

// SlotsToObserve returns the slots that ended between since and now and whose
// wait times have not been summarized yet.
func (s *Store) SlotsToObserve(ctx context.Context, since time.Time, now time.Time) ([]models.DayCalendar, error) {
	return s.findCalendar(ctx, bson.D{
		{Key: "endDate", Value: bson.D{{Key: "$gte", Value: since}, {Key: "$lte", Value: now}}},
		{Key: "observed", Value: bson.D{{Key: "$exists", Value: false}}},
	})
}

// SetObservation stores the wait times summary of a past slot.
func (s *Store) SetObservation(ctx context.Context, id bson.ObjectID, observation models.Observation) error {
	_, err := s.calendar.UpdateByID(ctx, id, bson.D{{Key: "$set", Value: bson.D{{Key: "observed", Value: observation}}}})
	return err
}

// ObservedSlots returns every slot with observed wait times, the model training set.
func (s *Store) ObservedSlots(ctx context.Context) ([]models.DayCalendar, error) {
	return s.findCalendar(ctx, bson.D{{Key: "observed.samples", Value: bson.D{{Key: "$gt", Value: 0}}}})
}
