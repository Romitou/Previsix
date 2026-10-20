// Package weather reads daily weather forecasts from Open-Meteo (https://open-meteo.com).
package weather

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/romitou/previsix/httputil"
)

// Day is the weather of a day at the park.
type Day struct {
	Date string `json:"date" bson:"date"`
	// Maximum temperature, in °C.
	TemperatureMax float64 `json:"temperatureMax" bson:"temperatureMax"`
	// Total precipitation, in mm.
	Precipitation float64 `json:"precipitation" bson:"precipitation"`
	// Maximum probability of precipitation, between 0 and 100.
	PrecipitationProbability float64 `json:"precipitationProbability" bson:"precipitationProbability"`
}

type Client struct {
	Endpoint  string
	Latitude  float64
	Longitude float64
}

type forecastResponse struct {
	Daily struct {
		Time                        []string   `json:"time"`
		TemperatureMax              []*float64 `json:"temperature_2m_max"`
		PrecipitationSum            []*float64 `json:"precipitation_sum"`
		PrecipitationProbabilityMax []*float64 `json:"precipitation_probability_max"`
	} `json:"daily"`
}

// MaxForecastDays is the furthest day Open-Meteo forecasts.
const MaxForecastDays = 16

// Forecast returns the weather of the last pastDays days and the next 16 days,
// indexed by YYYY-MM-DD.
func (c Client) Forecast(ctx context.Context, pastDays int) (map[string]Day, error) {
	query := url.Values{}
	query.Set("latitude", strconv.FormatFloat(c.Latitude, 'f', -1, 64))
	query.Set("longitude", strconv.FormatFloat(c.Longitude, 'f', -1, 64))
	query.Set("daily", "temperature_2m_max,precipitation_sum,precipitation_probability_max")
	query.Set("timezone", "Europe/Paris")
	query.Set("past_days", strconv.Itoa(pastDays))
	query.Set("forecast_days", strconv.Itoa(MaxForecastDays))

	var response forecastResponse
	if err := httputil.GetJSON(ctx, c.Endpoint+"?"+query.Encode(), &response); err != nil {
		return nil, err
	}

	daily := response.Daily
	if len(daily.TemperatureMax) != len(daily.Time) || len(daily.PrecipitationSum) != len(daily.Time) || len(daily.PrecipitationProbabilityMax) != len(daily.Time) {
		return nil, fmt.Errorf("inconsistent weather response: %d days", len(daily.Time))
	}

	days := make(map[string]Day, len(daily.Time))
	for i, date := range daily.Time {
		// Days without temperature are not forecast yet.
		if daily.TemperatureMax[i] == nil {
			continue
		}
		day := Day{Date: date, TemperatureMax: *daily.TemperatureMax[i]}
		if daily.PrecipitationSum[i] != nil {
			day.Precipitation = *daily.PrecipitationSum[i]
		}
		if daily.PrecipitationProbabilityMax[i] != nil {
			day.PrecipitationProbability = *daily.PrecipitationProbabilityMax[i]
		} else if day.Precipitation > 0.5 {
			// Past days have no probability, but we know whether it rained.
			day.PrecipitationProbability = 100
		}
		days[date] = day
	}
	return days, nil
}
