package forecast

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/romitou/previsix/dates"
	"github.com/romitou/previsix/holidays"
	"github.com/romitou/previsix/weather"
)

func slotAt(t *testing.T, day string, openHour, closeHour int) SlotInfo {
	t.Helper()
	d, err := dates.ParseDay(day)
	if err != nil {
		t.Fatal(err)
	}
	return SlotInfo{
		Date:  day,
		Open:  d.Add(time.Duration(openHour) * time.Hour),
		Close: d.Add(time.Duration(closeHour) * time.Hour),
	}
}

func TestBuildFeatures(t *testing.T) {
	periods := []holidays.Period{{Zone: "Zone C", Start: "2026-05-09", End: "2026-05-26"}}
	features := BuildFeatures(slotAt(t, "2026-05-15", 10, 19), periods, &weather.Day{TemperatureMax: 30, PrecipitationProbability: 50})
	if features.Weekday != time.Friday || !features.Bridge || features.PublicHoliday || !features.ZoneC || features.ZoneB || features.OpeningHours != 9 {
		t.Errorf("unexpected features %+v", features)
	}
	vector := features.Vector()
	if len(vector) != len(FeatureNames) || len(PriorCoefficients) != len(FeatureNames) {
		t.Fatalf("vector length %d, coefficients %d, names %d", len(vector), len(PriorCoefficients), len(FeatureNames))
	}
	if vector[0] != 1 || vector[4] != 1 || vector[15] != 0.5 || vector[17] != 0.5 {
		t.Errorf("unexpected vector %v", vector)
	}
}

func TestPriorModelOrdering(t *testing.T) {
	model := PriorModel()
	now, _ := dates.ParseDay("2026-01-01")
	predict := func(slot SlotInfo, periods []holidays.Period) Prediction {
		return model.Predict(BuildFeatures(slot, periods, nil), slot.Open, slot.Close, now)
	}

	quietTuesday := predict(slotAt(t, "2026-06-02", 10, 18), nil)
	saturday := predict(slotAt(t, "2026-06-06", 10, 18), nil)
	holidaySaturday := predict(slotAt(t, "2026-06-06", 10, 19), []holidays.Period{{Zone: "Zone C", Start: "2026-06-01", End: "2026-06-30"}})

	if !(quietTuesday.AvgWait < saturday.AvgWait && saturday.AvgWait < holidaySaturday.AvgWait) {
		t.Errorf("unexpected ordering: %v < %v < %v", quietTuesday.AvgWait, saturday.AvgWait, holidaySaturday.AvgWait)
	}
	if len(saturday.WaitTimes) != 8 || saturday.WaitTimes[0].Time.Hour() != 10 {
		t.Errorf("unexpected hourly wait times %+v", saturday.WaitTimes)
	}
	// The afternoon is busier than the opening.
	if saturday.WaitTimes[4].Minutes <= saturday.WaitTimes[0].Minutes {
		t.Errorf("profile should peak in the afternoon: %+v", saturday.WaitTimes)
	}
	if saturday.Confidence <= 0 || saturday.Confidence > 1 {
		t.Errorf("confidence out of range: %v", saturday.Confidence)
	}
}

func TestPredictHalfHours(t *testing.T) {
	slot := slotAt(t, "2026-06-06", 10, 18)
	slot.Open = slot.Open.Add(30 * time.Minute)
	prediction := PriorModel().Predict(BuildFeatures(slot, nil, nil), slot.Open, slot.Close, slot.Open)
	// 10:30-18:00 covers the 10h to 17h hours.
	if len(prediction.WaitTimes) != 8 {
		t.Errorf("got %d hours, want 8", len(prediction.WaitTimes))
	}
}

func TestConfidenceDecreasesWithDistance(t *testing.T) {
	model := PriorModel()
	slot := slotAt(t, "2026-08-01", 10, 19)
	features := BuildFeatures(slot, nil, &weather.Day{TemperatureMax: 22})
	near := model.Predict(features, slot.Open, slot.Close, slot.Open.Add(-24*time.Hour))
	far := model.Predict(BuildFeatures(slot, nil, nil), slot.Open, slot.Close, slot.Open.Add(-100*24*time.Hour))
	if near.Confidence <= far.Confidence {
		t.Errorf("near confidence %v should be above far confidence %v", near.Confidence, far.Confidence)
	}
}

func TestTrainUsesPriorWithoutData(t *testing.T) {
	model, err := Train([]Sample{{AvgWait: 20}}, 5, 10)
	if err != nil {
		t.Fatal(err)
	}
	if model.Samples != 0 || model.Sigma != priorSigma {
		t.Errorf("expected the prior model, got %+v", model)
	}
}

func TestTrainRecoversEffects(t *testing.T) {
	// Synthetic park: a real Saturday effect much stronger than the prior guess.
	truth := append([]float64(nil), PriorCoefficients...)
	truth[0] = math.Log(15)
	truth[5] = 0.9  // saturday
	truth[11] = 0.1 // bridge

	random := rand.New(rand.NewSource(1))
	start, _ := dates.ParseDay("2025-03-01")
	var samples []Sample
	for i := 0; i < 400; i++ {
		day := start.AddDate(0, 0, i).Format(dates.DayFormat)
		slot := slotAt(t, day, 10, 18+random.Intn(3))
		features := BuildFeatures(slot, nil, &weather.Day{
			TemperatureMax:           5 + random.Float64()*30,
			PrecipitationProbability: random.Float64() * 100,
			Precipitation:            random.Float64() * 5,
		})
		var logWait float64
		for k, value := range features.Vector() {
			logWait += truth[k] * value
		}
		logWait += random.NormFloat64() * 0.1
		profile := make([]float64, ProfileBins)
		for k := range profile {
			profile[k] = math.Exp(logWait) * (0.5 + float64(k)/float64(ProfileBins))
		}
		samples = append(samples, Sample{Features: features, AvgWait: math.Exp(logWait), Profile: profile})
	}

	model, err := Train(samples, 1, 14)
	if err != nil {
		t.Fatal(err)
	}
	if model.Samples != 400 {
		t.Errorf("trained on %d samples", model.Samples)
	}
	if math.Abs(model.Coefficients[5]-0.9) > 0.1 {
		t.Errorf("saturday coefficient = %v, want ≈ 0.9", model.Coefficients[5])
	}
	if math.Abs(model.Coefficients[0]-math.Log(15)) > 0.15 {
		t.Errorf("intercept = %v, want ≈ %v", model.Coefficients[0], math.Log(15))
	}
	if model.Sigma > 0.2 {
		t.Errorf("sigma = %v, want ≈ 0.1", model.Sigma)
	}
	if model.Profile[ProfileBins-1] <= model.Profile[0] {
		t.Errorf("profile should increase like the samples: %v", model.Profile)
	}
}

func TestCrowdScore(t *testing.T) {
	if CrowdScore(0) != 0 || CrowdScore(2) != 0 || CrowdScore(500) != 1 {
		t.Error("crowd score must be clamped between 0 and 1")
	}
	if CrowdScore(10) >= CrowdScore(20) {
		t.Error("crowd score must increase with the wait time")
	}
}

func TestSolve(t *testing.T) {
	solution, err := solve([][]float64{{2, 1}, {1, 3}}, []float64{3, 5})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(solution[0]-0.8) > 1e-9 || math.Abs(solution[1]-1.4) > 1e-9 {
		t.Errorf("solution = %v", solution)
	}
	if _, err = solve([][]float64{{1, 1}, {1, 1}}, []float64{1, 1}); err == nil {
		t.Error("expected a singular matrix error")
	}
}
