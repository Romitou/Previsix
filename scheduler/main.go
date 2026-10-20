package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/romitou/previsix/config"
	"github.com/romitou/previsix/dates"
	"github.com/romitou/previsix/scheduler/jobs"
)

// Scheduler runs the background jobs of Previsix.
type Scheduler struct {
	general   gocron.Scheduler
	forecasts gocron.Scheduler
}

// Start prepares the database state and schedules every job.
func Start(ctx context.Context, c *config.Config, runner *jobs.Runner) (*Scheduler, error) {
	// Forecasts pending in a previous process will never complete.
	if reset, err := runner.Store.ResetPending(ctx); err != nil {
		return nil, err
	} else if reset > 0 {
		slog.Info("pending forecasts reset", "count", reset)
	}
	// Use the observations gathered so far right away.
	runner.Train(ctx)

	general, err := gocron.NewScheduler(gocron.WithLocation(dates.Paris))
	if err != nil {
		return nil, err
	}
	forecasts, err := gocron.NewScheduler(
		gocron.WithLocation(dates.Paris),
		gocron.WithLimitConcurrentJobs(uint(c.Forecasts.Concurrent), gocron.LimitModeWait),
	)
	if err != nil {
		return nil, err
	}
	s := &Scheduler{general: general, forecasts: forecasts}

	startNow := gocron.WithStartAt(gocron.WithStartImmediately())
	// Let the calendar and the holidays synchronize before the first forecasts.
	startSoon := gocron.WithStartAt(gocron.WithStartDateTime(time.Now().Add(time.Minute)))
	singleton := gocron.WithSingletonMode(gocron.LimitModeReschedule)

	definitions := []struct {
		name       string
		definition gocron.JobDefinition
		task       gocron.Task
		options    []gocron.JobOption
	}{
		{
			name:       "syncCalendar",
			definition: gocron.DurationJob(6 * time.Hour),
			task:       gocron.NewTask(runner.SyncCalendar),
			options:    []gocron.JobOption{startNow},
		},
		{
			name:       "syncHolidays",
			definition: gocron.DailyJob(1, gocron.NewAtTimes(gocron.NewAtTime(3, 17, 0))),
			task:       gocron.NewTask(runner.SyncHolidays),
			options:    []gocron.JobOption{startNow},
		},
		{
			name:       "collectWaitTimes",
			definition: gocron.DurationJob(time.Duration(c.Collector.Interval) * time.Minute),
			task:       gocron.NewTask(runner.CollectWaitTimes),
			options:    []gocron.JobOption{startNow},
		},
		{
			name:       "learn",
			definition: gocron.DurationJob(time.Hour),
			task:       gocron.NewTask(runner.Learn),
			options:    []gocron.JobOption{startSoon},
		},
		{
			name:       "queueForecasts",
			definition: queueDefinition(c),
			task:       gocron.NewTask(runner.QueueForecasts, forecasts),
			options:    []gocron.JobOption{startSoon},
		},
	}

	for _, job := range definitions {
		options := append([]gocron.JobOption{gocron.WithName(job.name), singleton}, job.options...)
		if _, err = general.NewJob(job.definition, job.task, options...); err != nil {
			return nil, errors.Join(err, s.Shutdown())
		}
	}

	general.Start()
	forecasts.Start()
	return s, nil
}

// queueDefinition runs the forecast queuing at a random interval between the
// configured bounds, gocron rejecting random intervals with equal bounds.
func queueDefinition(c *config.Config) gocron.JobDefinition {
	minimum := time.Duration(c.Forecasts.Interval.Min) * time.Minute
	maximum := time.Duration(c.Forecasts.Interval.Max) * time.Minute
	if minimum == maximum {
		return gocron.DurationJob(minimum)
	}
	return gocron.DurationRandomJob(minimum, maximum)
}

// Shutdown stops the schedulers and waits for the running jobs.
func (s *Scheduler) Shutdown() error {
	return errors.Join(s.general.Shutdown(), s.forecasts.Shutdown())
}
