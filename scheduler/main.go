package scheduler

import (
	"github.com/go-co-op/gocron/v2"
	"github.com/romitou/previsix/config"
	"github.com/romitou/previsix/scheduler/jobs"
	"time"
)

func Start() error {
	generalScheduler, err := gocron.NewScheduler()
	if err != nil {
		return err
	}

	forecastScheduler, err := gocron.NewScheduler(gocron.WithLimitConcurrentJobs(uint(config.Get().Forecasts.Concurrent), gocron.LimitModeWait))
	if err != nil {
		return err
	}

	_, err = generalScheduler.NewJob(
		gocron.DailyJob(1, gocron.NewAtTimes(gocron.NewAtTime(0, 0, 0))),
		gocron.NewTask(jobs.InitForecastDays),
		gocron.WithName("initForecastDays"),
		gocron.WithStartAt(gocron.WithStartImmediately()),
	)
	if err != nil {
		return err
	}

	_, err = forecastScheduler.NewJob(
		gocron.DurationRandomJob(
			time.Duration(config.Get().Forecasts.Interval.Min)*time.Minute,
			time.Duration(config.Get().Forecasts.Interval.Max)*time.Minute),
		gocron.NewTask(jobs.QueueForecastJobs, forecastScheduler),
	)

	go generalScheduler.Start()
	go forecastScheduler.Start()

	return err
}
