package holidays

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/romitou/previsix/dates"
)

func day(t *testing.T, value string) time.Time {
	t.Helper()
	d, err := dates.ParseDay(value)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestPublicHolidays(t *testing.T) {
	// Reference: https://calendrier.api.gouv.fr/jours-feries/metropole/2026.json
	want := []string{"2026-01-01", "2026-04-06", "2026-05-01", "2026-05-08", "2026-05-14", "2026-05-25", "2026-07-14", "2026-08-15", "2026-11-01", "2026-11-11", "2026-12-25"}
	got := PublicHolidays(2026)
	if len(got) != len(want) {
		t.Fatalf("got %d holidays, want %d: %v", len(got), len(want), got)
	}
	for _, d := range want {
		if _, ok := got[d]; !ok {
			t.Errorf("missing %s", d)
		}
	}

	easterMondays := map[int]string{2024: "2024-04-01", 2025: "2025-04-21", 2027: "2027-03-29", 2038: "2038-04-26"}
	for year, d := range easterMondays {
		if PublicHolidays(year)[d] != "Lundi de Pâques" {
			t.Errorf("Easter Monday %d should be %s", year, d)
		}
	}
}

func TestIsBridge(t *testing.T) {
	tests := map[string]bool{
		"2026-05-15": true,  // Friday after Ascension
		"2026-05-14": false, // Ascension itself
		"2026-05-22": false, // ordinary Friday
		"2026-11-10": false, // Tuesday before 11 November (Wednesday)
		"2027-12-24": false, // Friday, Christmas is Saturday
		"2025-05-02": true,  // Friday after 1 May (Thursday)
		"2027-07-13": false, // Tuesday
		"2028-08-14": true,  // Monday before 15 August (Tuesday)
	}
	for d, want := range tests {
		if got := IsBridge(day(t, d)); got != want {
			t.Errorf("IsBridge(%s) = %v, want %v", d, got, want)
		}
	}
}

func TestZonesOnHoliday(t *testing.T) {
	periods := []Period{
		{Zone: "Zone C", Start: "2026-02-21", End: "2026-03-09"},
		{Zone: "Zone B", Start: "2026-02-14", End: "2026-03-02"},
	}
	if z := ZonesOnHoliday(periods, "2026-02-14"); !z["Zone B"] || z["Zone C"] {
		t.Errorf("unexpected zones %v", z)
	}
	if z := ZonesOnHoliday(periods, "2026-02-28"); !z["Zone B"] || !z["Zone C"] {
		t.Errorf("unexpected zones %v", z)
	}
	if z := ZonesOnHoliday(periods, "2026-03-09"); len(z) != 0 {
		t.Errorf("end day is back to school, got %v", z)
	}
}

func TestSchoolClient(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		if r.URL.Query().Get("where") == "" {
			t.Error("missing where clause")
		}
		var results string
		if offset == 0 {
			// Full first page, with duplicates for each académie.
			for i := 0; i < pageSize; i++ {
				results += `{"description":"Vacances de la Toussaint","population":"-","start_date":"2025-10-17T22:00:00+00:00","end_date":"2025-11-02T23:00:00+00:00","zones":"Zone C","location":"Paris"},`
			}
			results = results[:len(results)-1]
		} else {
			results = `{"description":"Vacances de Noël","population":"-","start_date":"2025-12-19T23:00:00+00:00","end_date":"2026-01-04T23:00:00+00:00","zones":"Zone B"},
				{"description":"Pré-rentrée","population":"Enseignants","start_date":"2026-08-30T22:00:00+00:00","end_date":"2026-08-31T22:00:00+00:00","zones":"Zone B"},
				{"description":"Vacances","population":"-","start_date":"2025-12-19T23:00:00+00:00","end_date":"2026-01-04T23:00:00+00:00","zones":"Corse"}`
		}
		_, _ = fmt.Fprintf(w, `{"total_count":%d,"results":[%s]}`, pageSize+3, results)
	}))
	defer server.Close()

	periods, err := SchoolClient{Endpoint: server.URL}.Periods(context.Background(), day(t, "2025-09-01"))
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Errorf("got %d requests, want 2", requests)
	}
	want := []Period{
		{Zone: "Zone C", Description: "Vacances de la Toussaint", Start: "2025-10-18", End: "2025-11-03"},
		{Zone: "Zone B", Description: "Vacances de Noël", Start: "2025-12-20", End: "2026-01-05"},
	}
	if len(periods) != len(want) {
		t.Fatalf("got %+v, want %+v", periods, want)
	}
	for i := range want {
		if periods[i] != want[i] {
			t.Errorf("period %d = %+v, want %+v", i, periods[i], want[i])
		}
	}
}
