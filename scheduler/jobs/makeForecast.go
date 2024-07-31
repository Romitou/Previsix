package jobs

import (
	"context"
	"github.com/romitou/previsix/database"
	"github.com/romitou/previsix/database/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"log"
	"time"
)

func MakeForecast(dayForecast models.DayCalendar) {
	log.Println("Making forecast for day", dayForecast.StartDate.Time().Format("2006-01-02"))

	db := database.Get().Database("previsix")
	collection := db.Collection("calendar")

	_, err := collection.UpdateOne(
		context.Background(),
		bson.D{{"_id", dayForecast.ID}},
		bson.D{{"$set", bson.D{{"forecastPending", false}, {"lastForecast", primitive.NewDateTimeFromTime(time.Now())}}}},
	)
	if err != nil {
		log.Println(err)
		return
	}

	// Make forecast
	var forecast models.DayForecast
	forecast.CrowdScore = 0.5
	forecast.ConfidenceScore = 0.5
	forecast.WaitTimes = []float64{10, 20, 30, 40, 50}
	forecast.ForecastedAt = primitive.NewDateTimeFromTime(time.Now())

	_, err = collection.UpdateOne(
		context.Background(),
		bson.D{{"_id", dayForecast.ID}},
		bson.D{{"$push", bson.D{{"forecasts", forecast}}}},
	)
	if err != nil {
		log.Println(err)
		return
	}

	log.Println("Forecast for day", dayForecast.StartDate.Time().Format("2006-01-02"), "is made")
}
