// Package forecast holds the attendance model. It predicts the average standby
// wait time of the park for an operating slot from calendar and weather
// features, and learns from the wait times observed on past days.
package forecast

import (
	"math"
	"time"

	"github.com/romitou/previsix/dates"
	"github.com/romitou/previsix/holidays"
	"github.com/romitou/previsix/weather"
)

// Features describes an operating slot.
type Features struct {
	Date          string       `json:"date" bson:"date"`
	Weekday       time.Weekday `json:"weekday" bson:"weekday"`
	ZoneA         bool         `json:"zoneA" bson:"zoneA"`
	ZoneB         bool         `json:"zoneB" bson:"zoneB"`
	ZoneC         bool         `json:"zoneC" bson:"zoneC"`
	PublicHoliday bool         `json:"publicHoliday" bson:"publicHoliday"`
	Bridge        bool         `json:"bridge" bson:"bridge"`
	NightEvent    bool         `json:"nightEvent" bson:"nightEvent"`
	EventDay      bool         `json:"eventDay" bson:"eventDay"`
	OpeningHours  float64      `json:"openingHours" bson:"openingHours"`
	Weather       *weather.Day `json:"weather,omitempty" bson:"weather,omitempty"`
}

// SlotInfo is what the model needs to know about the slot being forecast.
type SlotInfo struct {
	Date  string
	Open  time.Time
	Close time.Time
	// The slot is a ticketed night event.
	NightEvent bool
	// A night event takes place on that day (the daytime is busier).
	EventDay bool
}

// BuildFeatures computes the features of a slot. weatherDay may be nil when the
// day is too far away to be forecast.
func BuildFeatures(slot SlotInfo, periods []holidays.Period, weatherDay *weather.Day) Features {
	day, err := dates.ParseDay(slot.Date)
	if err != nil {
		day = dates.StartOfDay(slot.Open)
	}
	_, publicHoliday := holidays.PublicHoliday(day)
	zones := holidays.ZonesOnHoliday(periods, slot.Date)
	return Features{
		Date:          slot.Date,
		Weekday:       day.Weekday(),
		ZoneA:         zones["Zone A"],
		ZoneB:         zones["Zone B"],
		ZoneC:         zones["Zone C"],
		PublicHoliday: publicHoliday,
		Bridge:        holidays.IsBridge(day),
		NightEvent:    slot.NightEvent,
		EventDay:      slot.EventDay,
		OpeningHours:  slot.Close.Sub(slot.Open).Hours(),
		Weather:       weatherDay,
	}
}

// FeatureNames lists the model inputs, in the order of Vector.
var FeatureNames = []string{
	"intercept",
	"monday", "wednesday", "thursday", "friday", "saturday", "sunday",
	"zoneA", "zoneB", "zoneC",
	"publicHoliday", "bridge",
	"nightEvent", "eventDay",
	"openingHours",
	"rainProbability", "rainAmount", "heat", "cold",
}

func boolFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// Vector converts the features into the model inputs. Tuesday is the reference day.
func (f Features) Vector() []float64 {
	v := make([]float64, 0, len(FeatureNames))
	v = append(v, 1)
	for _, weekday := range []time.Weekday{time.Monday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday} {
		v = append(v, boolFloat(f.Weekday == weekday))
	}
	v = append(v,
		boolFloat(f.ZoneA), boolFloat(f.ZoneB), boolFloat(f.ZoneC),
		boolFloat(f.PublicHoliday), boolFloat(f.Bridge),
		boolFloat(f.NightEvent), boolFloat(f.EventDay),
		// Centered on a usual 9 hours day.
		f.OpeningHours-9,
	)

	var rainProbability, rainAmount, heat, cold float64
	if f.Weather != nil {
		rainProbability = f.Weather.PrecipitationProbability / 100
		rainAmount = math.Log1p(math.Max(0, f.Weather.Precipitation))
		heat = math.Max(0, f.Weather.TemperatureMax-25) / 10
		cold = math.Max(0, 15-f.Weather.TemperatureMax) / 10
	}
	return append(v, rainProbability, rainAmount, heat, cold)
}

// PriorCoefficients is the expert guess used until enough wait times have been
// observed, and towards which the fitted model is pulled. The model predicts
// the logarithm of the average wait time, in minutes, so each coefficient is
// roughly a relative effect (0.3 ≈ +35%).
var PriorCoefficients = []float64{
	math.Log(12),                 // a quiet Tuesday out of holidays
	0, 0.15, 0, 0.05, 0.45, 0.35, // weekdays (French pupils are off on Wednesday afternoons)
	0.10, 0.15, 0.35, // school holidays, zone C being the Paris area
	0.45, 0.35, // public holiday, bridge
	0.20, 0.15, // night event slot, day with a night event
	0.06,                       // per extra opening hour: the park opens longer on busy days
	-0.25, -0.05, -0.15, -0.20, // rain probability, rain amount, heat, cold
}
