package jobs

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/romitou/previsix/calendar"
	"github.com/romitou/previsix/config"
	"github.com/romitou/previsix/database"
	"github.com/romitou/previsix/holidays"
	"github.com/romitou/previsix/themeparks"
	"github.com/romitou/previsix/weather"
)

// Runner holds the dependencies of the jobs.
type Runner struct {
	Config *config.Config
	Store  *database.Store
	// Calendar sources, by decreasing priority.
	Calendars  []calendar.Source
	ThemeParks themeparks.Client
	Holidays   holidays.SchoolClient
	Weather    *WeatherCache
	// Now is overridable for tests.
	Now func() time.Time
}

func NewRunner(c *config.Config, store *database.Store) *Runner {
	themeParks := themeparks.Client{Endpoint: c.ThemeParks.Endpoint, ParkID: c.ThemeParks.ParkID}

	var sources []calendar.Source
	if c.Asterix.Endpoint != "" {
		sources = append(sources, asterixSource(c))
	}
	if c.ThemeParks.ParkID != "" {
		sources = append(sources, themeParks)
	}

	return &Runner{
		Config:     c,
		Store:      store,
		Calendars:  sources,
		ThemeParks: themeParks,
		Holidays:   holidays.SchoolClient{Endpoint: c.Sources.HolidaysEndpoint},
		Weather: &WeatherCache{
			Client: weather.Client{Endpoint: c.Sources.WeatherEndpoint, Latitude: c.Park.Latitude, Longitude: c.Park.Longitude},
			TTL:    time.Hour,
		},
		Now: time.Now,
	}
}

// jobTimeout bounds the duration of every job.
const jobTimeout = 5 * time.Minute

func (r *Runner) context() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), jobTimeout)
}

// pastWeatherDays is how far back the weather is fetched, to describe the
// days being observed.
const pastWeatherDays = 7

// WeatherCache keeps the weather forecast for a while, and serves the
// previous one when Open-Meteo is unreachable.
type WeatherCache struct {
	Client weather.Client
	TTL    time.Duration

	mutex     sync.Mutex
	days      map[string]weather.Day
	fetchedAt time.Time
	failedAt  time.Time
}

// weatherRetryDelay avoids hammering Open-Meteo while it fails.
const weatherRetryDelay = 10 * time.Minute

func (w *WeatherCache) Days(ctx context.Context) map[string]weather.Day {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	if w.days != nil && time.Since(w.fetchedAt) < w.TTL {
		return w.days
	}
	if time.Since(w.failedAt) < weatherRetryDelay {
		return w.days
	}
	days, err := w.Client.Forecast(ctx, pastWeatherDays)
	if err != nil {
		slog.Warn("cannot fetch the weather forecast", "error", err)
		w.failedAt = time.Now()
		return w.days
	}
	w.days = days
	w.fetchedAt = time.Now()
	return days
}

// Day returns the weather of a day, or nil when unknown.
func (w *WeatherCache) Day(ctx context.Context, date string) *weather.Day {
	day, ok := w.Days(ctx)[date]
	if !ok {
		return nil
	}
	return &day
}
