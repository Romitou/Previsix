package models

import "go.mongodb.org/mongo-driver/bson/primitive"

type DayForecastType string

const (
	DayForecastTypeNormal DayForecastType = "NORMAL"
	DayForecastTypeEvent  DayForecastType = "EVENT"
)

type DayForecast struct {
	CrowdScore      float64            `json:"crowdScore" bson:"crowdScore"`
	ConfidenceScore float64            `json:"confidence" bson:"confidence"`
	WaitTimes       []float64          `json:"waitTimes" bson:"waitTimes"`
	ForecastedAt    primitive.DateTime `json:"forecastedAt" bson:"forecastedAt"`
}

type DayCalendar struct {
	ID        primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	StartDate primitive.DateTime `json:"startDate" bson:"startDate"`
	CloseDate primitive.DateTime `json:"endDate" bson:"endDate"`
	DayType   DayForecastType    `json:"dayType" bson:"dayType"`

	Forecasts []DayForecast `json:"forecasts" bson:"forecasts"`

	ForecastPending bool               `json:"forecastPending" bson:"forecastPending"`
	CreatedAt       primitive.DateTime `json:"createdAt" bson:"createdAt"`
	LastForecast    primitive.DateTime `json:"lastForecast" bson:"lastForecast"`
}
