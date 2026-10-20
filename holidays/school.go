package holidays

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/romitou/previsix/dates"
	"github.com/romitou/previsix/httputil"
)

// Zones of the French school calendar. The park (Oise) is in zone B, but most
// of its visitors come from Île-de-France, in zone C.
var Zones = []string{"Zone A", "Zone B", "Zone C"}

// Period is a school holidays period for a zone.
type Period struct {
	Zone        string `json:"zone" bson:"zone"`
	Description string `json:"description" bson:"description"`
	// First day of holidays, YYYY-MM-DD.
	Start string `json:"start" bson:"start"`
	// First day back to school (exclusive), YYYY-MM-DD.
	End string `json:"end" bson:"end"`
}

// Contains reports whether the YYYY-MM-DD day is within the period.
func (p Period) Contains(day string) bool {
	return day >= p.Start && day < p.End
}

// ZonesOnHoliday returns the set of zones on holiday on a day.
func ZonesOnHoliday(periods []Period, day string) map[string]bool {
	zones := map[string]bool{}
	for _, period := range periods {
		if period.Contains(day) {
			zones[period.Zone] = true
		}
	}
	return zones
}

type schoolResponse struct {
	TotalCount int `json:"total_count"`
	Results    []struct {
		Description string    `json:"description"`
		Population  string    `json:"population"`
		StartDate   time.Time `json:"start_date"`
		EndDate     time.Time `json:"end_date"`
		Zones       string    `json:"zones"`
	} `json:"results"`
}

// SchoolClient reads the school holidays from the open data of the French
// Ministry of Education (data.education.gouv.fr).
type SchoolClient struct {
	Endpoint string
}

const pageSize = 100

// Periods returns the school holidays of metropolitan zones ending after since.
func (c SchoolClient) Periods(ctx context.Context, since time.Time) ([]Period, error) {
	seen := map[Period]bool{}
	var periods []Period

	for offset := 0; ; offset += pageSize {
		query := url.Values{}
		query.Set("where", fmt.Sprintf(`end_date >= "%s" and zones in ("Zone A", "Zone B", "Zone C")`, since.Format(dates.DayFormat)))
		query.Set("order_by", "start_date")
		query.Set("limit", strconv.Itoa(pageSize))
		query.Set("offset", strconv.Itoa(offset))

		var response schoolResponse
		if err := httputil.GetJSON(ctx, c.Endpoint+"?"+query.Encode(), &response); err != nil {
			return nil, err
		}

		for _, result := range response.Results {
			// Teachers-only periods (pre-rentrée) do not matter for attendance.
			if result.Population == "Enseignants" || !slices.Contains(Zones, result.Zones) {
				continue
			}
			period := Period{
				Zone:        result.Zones,
				Description: result.Description,
				// Dates are midnights in Paris expressed in UTC.
				Start: dates.DayKey(result.StartDate),
				End:   dates.DayKey(result.EndDate),
			}
			if period.End <= period.Start || seen[period] {
				continue
			}
			seen[period] = true
			periods = append(periods, period)
		}

		if len(response.Results) < pageSize || offset+pageSize >= response.TotalCount {
			break
		}
		// The API refuses offsets above 10000, we never need that much.
		if offset >= 9000 {
			break
		}
	}
	return periods, nil
}
