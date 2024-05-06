package jobs

import (
	"context"
	"github.com/go-co-op/gocron/v2"
	"github.com/romitou/previsix/config"
	"github.com/romitou/previsix/database"
	"github.com/romitou/previsix/database/models"
	"go.mongodb.org/mongo-driver/bson"
	"log"
	"math"
	"sort"
)

func QueueForecastJobs(scheduler gocron.Scheduler) {
	db := database.Get().Database("previsix")
	collection := db.Collection("calendar")
	result, err := collection.Find(context.Background(), bson.M{
		"forecastedAt": bson.M{"$exists": false},
	})
	if err != nil {
		log.Println(err)
		return
	}

	var dayForecasts []models.DayForecast
	err = result.Decode(&dayForecasts)
	if err != nil {
		log.Println(err)
		return
	}

	sort.Slice(dayForecasts, func(i, j int) bool {
		iPriority := math.Pow(float64(dayForecasts[i].OpenDate.Time().Unix()-dayForecasts[i].ForecastedAt.Time().Unix()), config.Get().Forecasts.PriorityExponent)
		jPriority := math.Pow(float64(dayForecasts[j].OpenDate.Time().Unix()-dayForecasts[j].ForecastedAt.Time().Unix()), config.Get().Forecasts.PriorityExponent)
		return iPriority > jPriority
	})

	forecastJobs := dayForecasts[:int(math.Min(float64(len(dayForecasts)), float64(config.Get().Forecasts.Concurrent)))]
	for _, dayForecast := range forecastJobs {
		_, err = scheduler.NewJob(
			gocron.OneTimeJob(gocron.OneTimeJobStartImmediately()),
			gocron.NewTask(MakeForecast, dayForecast),
			gocron.WithName("forecast-"+dayForecast.ID.Hex()),
		)
		if err != nil {
			log.Println(err)
			continue
		}
		log.Println("Forecast job queued for", dayForecast.OpenDate.Time())
	}
}
