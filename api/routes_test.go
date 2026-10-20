package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/romitou/previsix/database/models"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type fakeStore struct {
	days     []models.DayCalendar
	snapshot *models.WaitSnapshot
	pingErr  error
	from, to string
}

func (f *fakeStore) Ping(context.Context) error { return f.pingErr }

func (f *fakeStore) Days(_ context.Context, from string, to string) ([]models.DayCalendar, error) {
	f.from, f.to = from, to
	return f.days, nil
}

func (f *fakeStore) Day(_ context.Context, date string) ([]models.DayCalendar, error) {
	var result []models.DayCalendar
	for _, day := range f.days {
		if day.Date == date {
			result = append(result, day)
		}
	}
	return result, nil
}

func (f *fakeStore) Forecasts(_ context.Context, id bson.ObjectID, _ int64) ([]models.DayForecast, error) {
	return []models.DayForecast{{CalendarID: id, AvgWait: 20}}, nil
}

func (f *fakeStore) LatestSnapshot(context.Context) (*models.WaitSnapshot, error) {
	return f.snapshot, nil
}

func (f *fakeStore) ObservedSlots(context.Context) ([]models.DayCalendar, error) {
	forecastWait := 30.0
	return []models.DayCalendar{
		{Observed: &models.Observation{AvgWait: 20, ForecastAvgWait: &forecastWait}},
		{Observed: &models.Observation{AvgWait: 25}},
	}, nil
}

func request(t *testing.T, router *gin.Engine, path string, out any) int {
	t.Helper()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	if out != nil && recorder.Code == http.StatusOK {
		if err := json.Unmarshal(recorder.Body.Bytes(), out); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	return recorder.Code
}

func newRouter(store Store) *gin.Engine {
	gin.SetMode(gin.TestMode)
	now := func() time.Time { return time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC) }
	static := fstest.MapFS{"index.html": {Data: []byte("<html>Previsix</html>")}}
	return NewRouter(store, static, now)
}

func TestDays(t *testing.T) {
	store := &fakeStore{days: []models.DayCalendar{{ID: bson.NewObjectID(), Date: "2026-10-20"}}}
	router := newRouter(store)

	var response struct {
		Days []models.DayCalendar `json:"days"`
	}
	if code := request(t, router, "/api/days", &response); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if len(response.Days) != 1 || store.from != "2026-10-19" || store.to != "2027-02-16" {
		t.Errorf("unexpected response %+v, range %s - %s", response, store.from, store.to)
	}

	for path, want := range map[string]int{
		"/api/days?from=2026-10-01&to=2026-10-31": http.StatusOK,
		"/api/days?from=yesterday":                http.StatusBadRequest,
		"/api/days?from=2026-10-01&to=2026-09-01": http.StatusBadRequest,
		"/api/days?from=2026-01-01&to=2028-01-01": http.StatusBadRequest,
	} {
		if code := request(t, router, path, nil); code != want {
			t.Errorf("%s: status %d, want %d", path, code, want)
		}
	}
}

func TestDay(t *testing.T) {
	store := &fakeStore{days: []models.DayCalendar{
		{ID: bson.NewObjectID(), Date: "2026-10-31", DayType: models.DayForecastTypeNormal},
		{ID: bson.NewObjectID(), Date: "2026-10-31", DayType: models.DayForecastTypeEvent},
	}}
	router := newRouter(store)

	var response struct {
		Slots []slotDetail `json:"slots"`
	}
	if code := request(t, router, "/api/days/2026-10-31", &response); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if len(response.Slots) != 2 || len(response.Slots[1].History) != 1 || response.Slots[1].DayType != models.DayForecastTypeEvent {
		t.Errorf("unexpected response %+v", response)
	}
	if code := request(t, router, "/api/days/2026-11-01", nil); code != http.StatusNotFound {
		t.Errorf("closed day: status %d", code)
	}
	if code := request(t, router, "/api/days/tomorrow", nil); code != http.StatusBadRequest {
		t.Errorf("invalid day: status %d", code)
	}
}

func TestLiveAndHealth(t *testing.T) {
	store := &fakeStore{}
	router := newRouter(store)
	if code := request(t, router, "/api/live", nil); code != http.StatusNotFound {
		t.Errorf("live without data: status %d", code)
	}
	store.snapshot = &models.WaitSnapshot{Operating: 3}
	if code := request(t, router, "/api/live", nil); code != http.StatusOK {
		t.Errorf("live: status %d", code)
	}

	if code := request(t, router, "/api/health", nil); code != http.StatusOK {
		t.Errorf("health: status %d", code)
	}
	store.pingErr = errors.New("down")
	if code := request(t, router, "/api/health", nil); code != http.StatusServiceUnavailable {
		t.Errorf("unhealthy: status %d", code)
	}
}

func TestModel(t *testing.T) {
	router := newRouter(&fakeStore{})
	var response struct {
		ObservedSlots int      `json:"observedSlots"`
		Accuracy      accuracy `json:"accuracy"`
		Features      []string `json:"features"`
	}
	if code := request(t, router, "/api/model", &response); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if response.ObservedSlots != 2 || response.Accuracy.Slots != 1 || response.Accuracy.MAE != 10 || response.Accuracy.Bias != 10 || len(response.Features) == 0 {
		t.Errorf("unexpected response %+v", response)
	}
}

func TestStatic(t *testing.T) {
	router := newRouter(&fakeStore{})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "<html>Previsix</html>" {
		t.Errorf("status %d, body %q", recorder.Code, recorder.Body.String())
	}
}
