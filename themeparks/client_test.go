package themeparks

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/entity/park/schedule":
			_, _ = w.Write([]byte(`{"schedule":[
				{"date":"2026-10-21","type":"OPERATING","openingTime":"2026-10-21T09:00:00+02:00","closingTime":"2026-10-21T18:00:00+02:00"},
				{"date":"2026-10-21","type":"TICKETED_EVENT","openingTime":"2026-10-21T19:00:00+02:00","closingTime":"2026-10-22T01:00:00+02:00"},
				{"date":"2026-10-22","type":"INFO","openingTime":"2026-10-22T09:00:00+02:00","closingTime":"2026-10-22T18:00:00+02:00"}
			]}`))
		case "/entity/park/live":
			_, _ = w.Write([]byte(`{"liveData":[
				{"id":"b","name":"Toutatis","entityType":"ATTRACTION","status":"OPERATING","queue":{"STANDBY":{"waitTime":45}}},
				{"id":"a","name":"Oziris","entityType":"ATTRACTION","status":"CLOSED"},
				{"id":"c","name":"Show","entityType":"SHOW","status":"OPERATING"},
				{"id":"d","name":"Discobelix","entityType":"ATTRACTION","status":"OPERATING","queue":{"STANDBY":{"waitTime":null}}}
			]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestSlots(t *testing.T) {
	server := newServer(t)
	slots, err := Client{Endpoint: server.URL, ParkID: "park"}.Slots(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 2 {
		t.Fatalf("got %d slots, want 2", len(slots))
	}
	if slots[0].Date != "2026-10-21" || slots[0].Event || slots[0].Open.Hour() != 9 {
		t.Errorf("unexpected day slot %+v", slots[0])
	}
	if slots[1].Date != "2026-10-21" || !slots[1].Event || slots[1].Close.Hour() != 1 {
		t.Errorf("unexpected event slot %+v", slots[1])
	}
}

func TestLive(t *testing.T) {
	server := newServer(t)
	rides, err := Client{Endpoint: server.URL, ParkID: "park"}.Live(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rides) != 3 {
		t.Fatalf("got %d rides, want 3", len(rides))
	}
	if rides[0].Name != "Discobelix" || rides[0].WaitTime != nil {
		t.Errorf("unexpected ride %+v", rides[0])
	}
	if rides[2].Name != "Toutatis" || rides[2].WaitTime == nil || *rides[2].WaitTime != 45 || !rides[2].Operating() {
		t.Errorf("unexpected ride %+v", rides[2])
	}
}
