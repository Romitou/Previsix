package jobs

import (
	"context"
	"errors"
	"github.com/romitou/previsix/asterix"
	"github.com/romitou/previsix/database"
	"github.com/romitou/previsix/database/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func InitForecastDays() {
	calendar := asterix.GetCalendar()
	db := database.Get().Database("previsix")
	collection := db.Collection("calendar")
	for _, calendarDay := range calendar {
		var forecastDay models.DayForecast
		result := collection.FindOne(context.Background(), calendarDay)
		if errors.Is(result.Err(), mongo.ErrNoDocuments) {
			dayType := models.DayForecastTypeNormal
			if calendarDay.OpenDate.Day() != calendarDay.CloseDate.Day() {
				dayType = models.DayForecastTypeEvent
			}
			forecastDay = models.DayForecast{
				OpenDate:        primitive.NewDateTimeFromTime(calendarDay.OpenDate),
				CloseDate:       primitive.NewDateTimeFromTime(calendarDay.CloseDate),
				DayType:         dayType,
				CrowdScore:      0,
				ConfidenceScore: 0,
				Attractions:     []models.AttractionForecast{},
			}
			_, err := collection.InsertOne(context.Background(), forecastDay)
			if err != nil {
				continue
			}
		}
	}
}
