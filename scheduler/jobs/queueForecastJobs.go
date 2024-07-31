package jobs

import (
	"context"
	"github.com/go-co-op/gocron/v2"
	"github.com/romitou/previsix/config"
	"github.com/romitou/previsix/database"
	"github.com/romitou/previsix/database/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
	"log"
	"math"
	"sort"
)

func QueueForecastJobs(scheduler gocron.Scheduler) {
	db := database.Get().Database("previsix")
	collection := db.Collection("calendar")
	result, err := collection.Find(
		context.Background(),
		bson.M{"$or": []bson.M{
			{"forecastPending": false},
			{"forecastPending": bson.M{"$exists": false}},
		}},
		&options.FindOptions{
			Sort: bson.M{"startDate": 1},
		})
	if err != nil {
		log.Println(err)
		return
	}

	var dayForecasts []models.DayCalendar
	err = result.All(context.Background(), &dayForecasts)
	if err != nil {
		log.Println(err)
		return
	}

	sort.Slice(dayForecasts, func(i, j int) bool {
		iPriority := math.Pow(float64(dayForecasts[i].StartDate.Time().Unix()-dayForecasts[i].LastForecast.Time().Unix()), config.Get().Forecasts.PriorityExponent)
		jPriority := math.Pow(float64(dayForecasts[j].StartDate.Time().Unix()-dayForecasts[j].LastForecast.Time().Unix()), config.Get().Forecasts.PriorityExponent)
		return iPriority > jPriority
	})

	forecastJobs := dayForecasts[:int(math.Min(float64(len(dayForecasts)), float64(config.Get().Forecasts.Concurrent)))]
	for _, dayForecast := range forecastJobs {

		_, err = collection.UpdateOne(
			context.Background(),
			bson.D{{"_id", dayForecast.ID}},
			bson.D{{"$set", bson.D{{"forecastPending", true}}}},
		)
		if err != nil {
			log.Println(err)
			continue
		}

		_, err = scheduler.NewJob(
			gocron.OneTimeJob(gocron.OneTimeJobStartImmediately()),
			gocron.NewTask(MakeForecast, dayForecast),
			gocron.WithName("forecast-"+dayForecast.ID.Hex()),
		)
		if err != nil {
			log.Println(err)
			continue
		}
		log.Println("Forecast job queued for", dayForecast.StartDate.Time())
	}
}
