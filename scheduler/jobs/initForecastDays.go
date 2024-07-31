package jobs

import (
	"context"
	"errors"
	"github.com/romitou/previsix/asterix"
	"github.com/romitou/previsix/database"
	"github.com/romitou/previsix/database/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"time"
)

func InitForecastDays() {
	calendar := asterix.GetCalendar()
	db := database.Get().Database("previsix")
	collection := db.Collection("calendar")
	for _, calendarDay := range calendar {
		var forecastDay models.DayCalendar
		result := collection.FindOne(context.Background(), bson.D{{"startDate", primitive.NewDateTimeFromTime(calendarDay.OpenDate)}})
		if result.Err() != nil {
			if errors.Is(result.Err(), mongo.ErrNoDocuments) {
				dayType := models.DayForecastTypeNormal
				if calendarDay.OpenDate.Day() != calendarDay.CloseDate.Day() {
					dayType = models.DayForecastTypeEvent
				}
				forecastDay = models.DayCalendar{
					StartDate: primitive.NewDateTimeFromTime(calendarDay.OpenDate),
					CloseDate: primitive.NewDateTimeFromTime(calendarDay.CloseDate),
					DayType:   dayType,
					Forecasts: []models.DayForecast{},
					CreatedAt: primitive.NewDateTimeFromTime(time.Now()),
				}
				_, err := collection.InsertOne(context.Background(), forecastDay)
				if err != nil {
					continue
				}
			}
		}
	}
}
