package jobs

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/romitou/previsix/calendar"
	"github.com/romitou/previsix/config"
	"github.com/romitou/previsix/database"
	"github.com/romitou/previsix/database/models"
	"github.com/romitou/previsix/dates"
	"github.com/romitou/previsix/forecast"
)

type staticSource []calendar.Slot

func (s staticSource) Name() string { return "static" }

func (s staticSource) Slots(context.Context) ([]calendar.Slot, error) { return s, nil }

func testStore(t *testing.T) *database.Store {
	t.Helper()
	uri := os.Getenv("PREVISIX_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("PREVISIX_TEST_MONGO_URI is not set")
	}
	ctx := context.Background()
	store, err := database.Connect(ctx, uri, "previsix_test_"+strconv.FormatInt(time.Now().UnixNano(), 36))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = store.Drop(ctx)
		_ = store.Close(ctx)
	})
	return store
}

// fakeAPIs serves the weather, school holidays and live wait times. The live
// wait time is read from wait.
func fakeAPIs(t *testing.T, wait *atomic.Int64) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/weather":
			start := dates.StartOfDay(time.Now()).AddDate(0, 0, -7)
			var days, temperatures, rain, probability []string
			for i := 0; i < 23; i++ {
				days = append(days, `"`+start.AddDate(0, 0, i).Format(dates.DayFormat)+`"`)
				temperatures = append(temperatures, "21")
				rain = append(rain, "0")
				probability = append(probability, "10")
			}
			_, _ = fmt.Fprintf(w, `{"daily":{"time":[%s],"temperature_2m_max":[%s],"precipitation_sum":[%s],"precipitation_probability_max":[%s]}}`,
				strings.Join(days, ","), strings.Join(temperatures, ","), strings.Join(rain, ","), strings.Join(probability, ","))
		case r.URL.Path == "/holidays":
			_, _ = w.Write([]byte(`{"total_count":1,"results":[{"description":"Vacances","population":"-","start_date":"2020-01-01T00:00:00+00:00","end_date":"2040-01-01T00:00:00+00:00","zones":"Zone C"}]}`))
		case strings.HasSuffix(r.URL.Path, "/live"):
			_, _ = fmt.Fprintf(w, `{"liveData":[{"id":"a","name":"Toutatis","entityType":"ATTRACTION","status":"OPERATING","queue":{"STANDBY":{"waitTime":%d}}}]}`, wait.Load())
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func testConfig(t *testing.T, endpoint string) *config.Config {
	t.Helper()
	t.Setenv("PREVISIX_SOURCES_WEATHERENDPOINT", endpoint+"/weather")
	t.Setenv("PREVISIX_SOURCES_HOLIDAYSENDPOINT", endpoint+"/holidays")
	t.Setenv("PREVISIX_THEMEPARKS_ENDPOINT", endpoint)
	t.Setenv("PREVISIX_MODEL_MINTRAININGDAYS", "1")
	if err := config.Load(""); err != nil {
		t.Fatal(err)
	}
	return config.Get()
}

func TestPipeline(t *testing.T) {
	store := testStore(t)
	var wait atomic.Int64
	server := fakeAPIs(t, &wait)
	runner := NewRunner(testConfig(t, server.URL), store)
	t.Cleanup(func() { forecast.SetCurrent(forecast.PriorModel()) })

	today := dates.StartOfDay(time.Now())
	yesterday := today.AddDate(0, 0, -1)
	// Yesterday, then the next 10 days (today's slot may already be over).
	slots := staticSource{{Date: dates.DayKey(yesterday), Open: yesterday.Add(10 * time.Hour), Close: yesterday.Add(18 * time.Hour)}}
	for i := 1; i <= 10; i++ {
		day := today.AddDate(0, 0, i)
		slots = append(slots, calendar.Slot{Date: day.Format(dates.DayFormat), Open: day.Add(10 * time.Hour), Close: day.Add(18 * time.Hour)})
	}
	runner.Calendars = []calendar.Source{slots}

	// Yesterday's slot was synchronized at the time, the job only stores upcoming days.
	if _, err := store.SyncCalendar(context.Background(), slots[:1], "static", "", yesterday); err != nil {
		t.Fatal(err)
	}
	runner.SyncCalendar()
	runner.SyncHolidays()

	// Forecasts.
	forecastScheduler, err := gocron.NewScheduler(gocron.WithLimitConcurrentJobs(2, gocron.LimitModeWait))
	if err != nil {
		t.Fatal(err)
	}
	forecastScheduler.Start()
	runner.QueueForecasts(forecastScheduler)

	// Wait for the forecasts: the shutdown drops the jobs waiting for a slot.
	var days []models.DayCalendar
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if days, err = store.Days(context.Background(), dates.DayKey(today), dates.DayKey(today.AddDate(0, 0, 30))); err != nil {
			t.Fatal(err)
		}
		if !slices.ContainsFunc(days, func(day models.DayCalendar) bool { return day.ForecastPending }) {
			break
		}
	}
	if err = forecastScheduler.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if len(days) != 10 {
		t.Fatalf("got %d upcoming slots, want 10", len(days))
	}
	for _, day := range days {
		if day.LatestForecast == nil || day.ForecastPending {
			t.Fatalf("slot %s not forecast: %+v", day.Date, day)
		}
		if !day.LatestForecast.Features.ZoneC || day.LatestForecast.ModelSamples != 0 {
			t.Errorf("unexpected forecast features %+v", day.LatestForecast)
		}
	}
	if days[1].LatestForecast.Features.Weather == nil || days[9].LatestForecast.Features.Weather == nil {
		t.Error("the weather must be known for the next days")
	}

	// Wait times collected yesterday, then learning.
	for minute := 0; minute < 8*60; minute += 15 {
		at := yesterday.Add(10*time.Hour + time.Duration(minute)*time.Minute)
		runner.Now = func() time.Time { return at }
		wait.Store(int64(10 + minute/20))
		runner.CollectWaitTimes()
	}
	runner.Now = time.Now
	runner.Learn()

	observed, err := store.ObservedSlots(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(observed) != 1 || observed[0].Observed.Samples != 32 || observed[0].Observed.Features.Weather == nil {
		t.Fatalf("unexpected observations %+v", observed)
	}
	if model := forecast.Current(); model.Samples != 1 {
		t.Errorf("the model must be trained on the observed slot, got %d samples", model.Samples)
	}
}
