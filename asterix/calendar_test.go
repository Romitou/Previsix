package asterix

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/romitou/previsix/dates"
)

func TestParseTimes(t *testing.T) {
	at := func(day string, hour, minute int) time.Time {
		d, _ := dates.ParseDay(day)
		return time.Date(d.Year(), d.Month(), d.Day(), hour, minute, 0, 0, dates.Paris)
	}

	type slot struct {
		open, close time.Time
		event       bool
	}
	tests := []struct {
		name  string
		day   string
		times string
		want  []slot
		err   bool
	}{
		{"simple", "2024-07-14", "10h-19h", []slot{{at("2024-07-14", 10, 0), at("2024-07-14", 19, 0), false}}, false},
		{"spaces and uppercase", "2024-07-14", "10 H - 19 H", []slot{{at("2024-07-14", 10, 0), at("2024-07-14", 19, 0), false}}, false},
		{"minutes", "2024-07-14", "9h30 - 18h45", []slot{{at("2024-07-14", 9, 30), at("2024-07-14", 18, 45), false}}, false},
		{"night event", "2024-10-26", "10h-18h et 19h-1h", []slot{
			{at("2024-10-26", 10, 0), at("2024-10-26", 18, 0), false},
			{at("2024-10-26", 19, 0), at("2024-10-27", 1, 0), true},
		}, false},
		{"until midnight is not an event", "2024-07-14", "10h-24h", []slot{{at("2024-07-14", 10, 0), at("2024-07-15", 0, 0), false}}, false},
		{"en dash", "2024-07-14", "10h–19h", []slot{{at("2024-07-14", 10, 0), at("2024-07-14", 19, 0), false}}, false},
		{"closed", "2024-01-10", "Fermé", nil, false},
		{"empty", "2024-01-10", "", nil, false},
		{"garbage", "2024-07-14", "toute la journée", nil, true},
		{"invalid hour", "2024-07-14", "10h-27h", nil, true},
		{"invalid day", "14/07/2024", "10h-19h", nil, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			slots, err := ParseTimes(test.day, test.times)
			if (err != nil) != test.err {
				t.Fatalf("error = %v, want error %v", err, test.err)
			}
			if len(slots) != len(test.want) {
				t.Fatalf("got %d slots, want %d", len(slots), len(test.want))
			}
			for i, want := range test.want {
				got := slots[i]
				if !got.Open.Equal(want.open) || !got.Close.Equal(want.close) || got.Event != want.event || got.Date != test.day {
					t.Errorf("slot %d = %+v, want %+v", i, got, want)
				}
			}
		})
	}
}

func TestClientSlots(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var query GraphQlQuery
		if err := json.NewDecoder(r.Body).Decode(&query); err != nil || query.Query != "{ calendar }" {
			t.Errorf("unexpected query %+v (%v)", query, err)
		}
		_, _ = w.Write([]byte(`{"data":{"calendar":[
			{"day":"2024-10-26","times":"10h-18h et 19h-1h"},
			{"day":"2024-10-27","times":"n'importe quoi"},
			{"day":"2024-10-28","times":"10h-18h"}
		]}}`))
	}))
	defer server.Close()

	slots, err := Client{Endpoint: server.URL, Query: "{ calendar }"}.Slots(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 3 {
		t.Fatalf("got %d slots, want 3 (invalid entries are skipped)", len(slots))
	}
}

func TestClientGraphQLError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"errors":[{"message":"boom"}]}`))
	}))
	defer server.Close()

	if _, err := (Client{Endpoint: server.URL, Query: "{}"}).Slots(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
}

func TestClientHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusBadGateway)
	}))
	defer server.Close()

	if _, err := (Client{Endpoint: server.URL, Query: "{}"}).Slots(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
}
