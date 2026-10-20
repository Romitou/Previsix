package models

import (
	"time"

	"github.com/romitou/previsix/forecast"
	"github.com/romitou/previsix/themeparks"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type DayForecastType string

const (
	DayForecastTypeNormal DayForecastType = "NORMAL"
	DayForecastTypeEvent  DayForecastType = "EVENT"
)

// DayForecast is a forecast of an operating slot. Every forecast is kept in the
// forecasts collection, the latest one is also embedded in the slot.
type DayForecast struct {
	ID         bson.ObjectID `json:"id" bson:"_id,omitempty"`
	CalendarID bson.ObjectID `json:"calendarId" bson:"calendarId"`
	Date       string        `json:"date" bson:"date"`

	// Expected average standby wait time, in minutes.
	AvgWait         float64               `json:"avgWait" bson:"avgWait"`
	CrowdScore      float64               `json:"crowdScore" bson:"crowdScore"`
	ConfidenceScore float64               `json:"confidence" bson:"confidence"`
	WaitTimes       []forecast.HourlyWait `json:"waitTimes" bson:"waitTimes"`
	Features        forecast.Features     `json:"features" bson:"features"`
	// Amount of observed days the model was trained on (0 for the prior model).
	ModelSamples int       `json:"modelSamples" bson:"modelSamples"`
	ForecastedAt time.Time `json:"forecastedAt" bson:"forecastedAt"`
}

// Observation summarizes the wait times measured during a past slot.
type Observation struct {
	AvgWait    float64 `json:"avgWait" bson:"avgWait"`
	CrowdScore float64 `json:"crowdScore" bson:"crowdScore"`
	// Amount of wait times snapshots taken during the slot.
	Samples int `json:"samples" bson:"samples"`
	// Average wait time of each profile bin, -1 when not observed.
	Profile   []float64             `json:"profile" bson:"profile"`
	WaitTimes []forecast.HourlyWait `json:"waitTimes" bson:"waitTimes"`
	// Features of the slot, with the actual weather, used to train the model.
	Features forecast.Features `json:"features" bson:"features"`
	// Average wait time forecast before the opening, to measure the accuracy.
	ForecastAvgWait *float64  `json:"forecastAvgWait,omitempty" bson:"forecastAvgWait,omitempty"`
	ObservedAt      time.Time `json:"observedAt" bson:"observedAt"`
}

// DayCalendar is an operating slot of the park. There is at most one slot per
// day and type: the daytime opening, and the night event.
type DayCalendar struct {
	ID        bson.ObjectID   `json:"id" bson:"_id,omitempty"`
	Date      string          `json:"date" bson:"date"`
	DayType   DayForecastType `json:"dayType" bson:"dayType"`
	StartDate time.Time       `json:"startDate" bson:"startDate"`
	CloseDate time.Time       `json:"endDate" bson:"endDate"`
	// False when the slot disappeared from the official calendar.
	Active bool   `json:"active" bson:"active"`
	Source string `json:"source" bson:"source"`

	LatestForecast *DayForecast `json:"latestForecast,omitempty" bson:"latestForecast,omitempty"`
	Observed       *Observation `json:"observed,omitempty" bson:"observed,omitempty"`

	ForecastPending bool       `json:"forecastPending" bson:"forecastPending"`
	PendingSince    *time.Time `json:"-" bson:"pendingSince,omitempty"`
	LastForecast    *time.Time `json:"lastForecast,omitempty" bson:"lastForecast,omitempty"`
	CreatedAt       time.Time  `json:"createdAt" bson:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt" bson:"updatedAt"`
}

// WaitSnapshot is the state of every attraction at a given time.
type WaitSnapshot struct {
	ID         bson.ObjectID         `json:"-" bson:"_id,omitempty"`
	ObservedAt time.Time             `json:"observedAt" bson:"observedAt"`
	Date       string                `json:"date" bson:"date"`
	Rides      []themeparks.RideWait `json:"rides" bson:"rides"`
	// Amount of operating attractions.
	Operating int `json:"operating" bson:"operating"`
	// Average standby wait time of the operating attractions, nil when none reports one.
	AvgWait *float64 `json:"avgWait" bson:"avgWait"`
}
