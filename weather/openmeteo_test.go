package weather

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestForecast(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("latitude") != "49.13" || query.Get("past_days") != "2" || query.Get("timezone") != "Europe/Paris" {
			t.Errorf("unexpected query %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"daily":{
			"time":["2026-10-18","2026-10-19","2026-10-20"],
			"temperature_2m_max":[14.2,16.5,null],
			"precipitation_sum":[3.1,0,null],
			"precipitation_probability_max":[null,20,null]
		}}`))
	}))
	defer server.Close()

	days, err := Client{Endpoint: server.URL, Latitude: 49.13, Longitude: 2.57}.Forecast(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 2 {
		t.Fatalf("got %d days, want 2", len(days))
	}
	if d := days["2026-10-18"]; d.TemperatureMax != 14.2 || d.Precipitation != 3.1 || d.PrecipitationProbability != 100 {
		t.Errorf("unexpected past day %+v", d)
	}
	if d := days["2026-10-19"]; d.TemperatureMax != 16.5 || d.PrecipitationProbability != 20 {
		t.Errorf("unexpected forecast day %+v", d)
	}
}

func TestForecastInconsistent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"daily":{"time":["2026-10-18"],"temperature_2m_max":[],"precipitation_sum":[],"precipitation_probability_max":[]}}`))
	}))
	defer server.Close()

	if _, err := (Client{Endpoint: server.URL}).Forecast(context.Background(), 0); err == nil {
		t.Fatal("expected an error")
	}
}
