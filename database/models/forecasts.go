package models

import "go.mongodb.org/mongo-driver/bson/primitive"

type DayForecastType string

const (
	DayForecastTypeNormal DayForecastType = "NORMAL"
	DayForecastTypeEvent  DayForecastType = "EVENT"
)

type AttractionForecast struct {
	Slug      string    `json:"slug" bson:"slug"`
	Name      string    `json:"name" bson:"name"`
	WaitTimes []float64 `json:"waitTimes" bson:"waitTimes"`
}

type DayForecast struct {
	ID        primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	OpenDate  primitive.DateTime `json:"startDate" bson:"startDate"`
	CloseDate primitive.DateTime `json:"endDate" bson:"endDate"`
	DayType   DayForecastType    `json:"dayType" bson:"dayType"`

	CrowdScore      float64 `json:"crowdScore" bson:"crowdScore"`
	ConfidenceScore float64 `json:"confidence" bson:"confidence"`

	Attractions []AttractionForecast `json:"attractions" bson:"attractions"`

	CreatedAt    primitive.DateTime `json:"createdAt" bson:"createdAt"`
	ForecastedAt primitive.DateTime `json:"forecastedAt" bson:"forecastedAt"`
}
