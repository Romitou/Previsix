package forecast

import (
	"errors"
	"math"
	"time"
)

// ProfileBins is the resolution of the daily wait times profile: the opening
// period is split into this many equal parts.
const ProfileBins = 12

// Wait times references used to turn a wait time into a 0-1 crowd score.
const (
	quietWait   = 5.0
	crowdedWait = 60.0
)

// Default spread of the prediction errors (on the log scale) before any training.
const priorSigma = 0.45

// Sample is a past slot whose wait times have been observed.
type Sample struct {
	Features Features
	// Average standby wait time over the slot, in minutes.
	AvgWait float64
	// Average wait time of each profile bin, NaN when not observed.
	Profile []float64
}

// Model is a trained (or prior) attendance model.
type Model struct {
	Coefficients []float64 `json:"coefficients"`
	// Average share of the daily wait time over the opening period (mean 1).
	Profile []float64 `json:"profile"`
	// Standard deviation of the residuals, on the log scale.
	Sigma float64 `json:"sigma"`
	// Mean absolute error on the training samples, in minutes.
	TrainingMAE float64   `json:"trainingMae"`
	Samples     int       `json:"samples"`
	TrainedAt   time.Time `json:"trainedAt"`
}

// PriorProfile is the usual shape of a day: quiet at the opening, a peak in
// the early afternoon and a decrease before the closing.
func PriorProfile() []float64 {
	profile := make([]float64, ProfileBins)
	for i := range profile {
		x := (float64(i) + 0.5) / ProfileBins
		profile[i] = 0.35 + math.Sin(math.Pi*math.Min(1, x*1.1))
	}
	return normalize(profile)
}

func normalize(profile []float64) []float64 {
	var sum float64
	for _, value := range profile {
		sum += value
	}
	if sum <= 0 {
		return PriorProfile()
	}
	mean := sum / float64(len(profile))
	normalized := make([]float64, len(profile))
	for i, value := range profile {
		normalized[i] = value / mean
	}
	return normalized
}

// PriorModel returns the model used when no wait time has been observed yet.
func PriorModel() *Model {
	return &Model{
		Coefficients: append([]float64(nil), PriorCoefficients...),
		Profile:      PriorProfile(),
		Sigma:        priorSigma,
	}
}

// Train fits a ridge regression of the log wait times, pulled towards the prior
// coefficients by lambda: with few samples the model stays close to the expert
// guess, and drifts away as evidence accumulates. Below minSamples, the prior
// model is returned.
func Train(samples []Sample, lambda float64, minSamples int) (*Model, error) {
	var valid []Sample
	for _, sample := range samples {
		if sample.AvgWait > 0 && !math.IsNaN(sample.AvgWait) {
			valid = append(valid, sample)
		}
	}
	if len(valid) < minSamples || len(valid) == 0 {
		model := PriorModel()
		model.TrainedAt = time.Now()
		return model, nil
	}

	size := len(FeatureNames)
	// Normal equations: (XᵀX + λI) β = Xᵀy + λβ₀
	matrix := make([][]float64, size)
	for i := range matrix {
		matrix[i] = make([]float64, size)
		matrix[i][i] = lambda
	}
	vector := make([]float64, size)
	for i := range vector {
		vector[i] = lambda * PriorCoefficients[i]
	}
	for _, sample := range valid {
		x := sample.Features.Vector()
		y := math.Log(sample.AvgWait)
		for i := 0; i < size; i++ {
			vector[i] += x[i] * y
			for j := 0; j < size; j++ {
				matrix[i][j] += x[i] * x[j]
			}
		}
	}
	coefficients, err := solve(matrix, vector)
	if err != nil {
		return nil, err
	}

	model := &Model{
		Coefficients: coefficients,
		Profile:      trainProfile(valid),
		Samples:      len(valid),
		TrainedAt:    time.Now(),
	}

	var squares, absolute float64
	for _, sample := range valid {
		predicted := model.predictLog(sample.Features)
		residual := math.Log(sample.AvgWait) - predicted
		squares += residual * residual
		absolute += math.Abs(sample.AvgWait - math.Exp(predicted))
	}
	model.TrainingMAE = absolute / float64(len(valid))
	// Blend with the prior spread so that a handful of samples cannot make the
	// model overconfident.
	n := float64(len(valid))
	model.Sigma = math.Sqrt((squares + 10*priorSigma*priorSigma) / (n + 10))
	return model, nil
}

// trainProfile averages the observed profiles, starting from the prior one.
func trainProfile(samples []Sample) []float64 {
	prior := PriorProfile()
	sums := make([]float64, ProfileBins)
	counts := make([]float64, ProfileBins)
	// The prior counts as a few days of observations.
	for i := range prior {
		sums[i] = 3 * prior[i]
		counts[i] = 3
	}
	for _, sample := range samples {
		if len(sample.Profile) != ProfileBins {
			continue
		}
		for i, value := range sample.Profile {
			if math.IsNaN(value) || value < 0 {
				continue
			}
			sums[i] += value / sample.AvgWait
			counts[i]++
		}
	}
	profile := make([]float64, ProfileBins)
	for i := range profile {
		profile[i] = sums[i] / counts[i]
	}
	return normalize(profile)
}

// solve solves a linear system with Gaussian elimination and partial pivoting.
func solve(matrix [][]float64, vector []float64) ([]float64, error) {
	n := len(vector)
	a := make([][]float64, n)
	for i := range matrix {
		a[i] = append(append([]float64(nil), matrix[i]...), vector[i])
	}
	for column := 0; column < n; column++ {
		pivot := column
		for row := column + 1; row < n; row++ {
			if math.Abs(a[row][column]) > math.Abs(a[pivot][column]) {
				pivot = row
			}
		}
		if math.Abs(a[pivot][column]) < 1e-12 {
			return nil, errors.New("singular matrix")
		}
		a[column], a[pivot] = a[pivot], a[column]
		for row := column + 1; row < n; row++ {
			factor := a[row][column] / a[column][column]
			for k := column; k <= n; k++ {
				a[row][k] -= factor * a[column][k]
			}
		}
	}
	solution := make([]float64, n)
	for row := n - 1; row >= 0; row-- {
		sum := a[row][n]
		for k := row + 1; k < n; k++ {
			sum -= a[row][k] * solution[k]
		}
		solution[row] = sum / a[row][row]
	}
	return solution, nil
}

func (m *Model) predictLog(features Features) float64 {
	var sum float64
	for i, value := range features.Vector() {
		sum += m.Coefficients[i] * value
	}
	return sum
}

// HourlyWait is the expected average wait time at a given time.
type HourlyWait struct {
	Time    time.Time `json:"time" bson:"time"`
	Minutes float64   `json:"minutes" bson:"minutes"`
}

// Prediction is the forecast of an operating slot.
type Prediction struct {
	// Expected average standby wait time over the slot, in minutes.
	AvgWait float64
	// Crowd score between 0 (empty park) and 1 (packed park).
	CrowdScore float64
	// Confidence between 0 and 1.
	Confidence float64
	// Expected wait time for each hour of the slot.
	WaitTimes []HourlyWait
}

// CrowdScore maps an average wait time to a 0-1 score, on a logarithmic scale.
func CrowdScore(avgWait float64) float64 {
	if avgWait <= 0 {
		return 0
	}
	score := math.Log(avgWait/quietWait) / math.Log(crowdedWait/quietWait)
	return math.Max(0, math.Min(1, score))
}

// Predict forecasts a slot. now is used to evaluate how far the slot is.
func (m *Model) Predict(features Features, open, close time.Time, now time.Time) Prediction {
	avgWait := math.Exp(m.predictLog(features))
	prediction := Prediction{
		AvgWait:    round(avgWait),
		CrowdScore: round(CrowdScore(avgWait)),
		Confidence: round(m.confidence(features, open.Sub(now))),
	}

	duration := close.Sub(open)
	for at := open.Truncate(time.Hour); at.Before(close); at = at.Add(time.Hour) {
		// Middle of the hour, clamped within the opening period.
		middle := at.Add(30 * time.Minute)
		if middle.Before(open) {
			middle = open
		}
		if middle.After(close) {
			middle = close
		}
		bin := int(float64(ProfileBins) * float64(middle.Sub(open)) / float64(duration))
		bin = max(0, min(ProfileBins-1, bin))
		prediction.WaitTimes = append(prediction.WaitTimes, HourlyWait{Time: at, Minutes: round(avgWait * m.Profile[bin])})
	}
	return prediction
}

// confidence decreases with the uncertainty of the model and with the
// distance of the slot, and is lower when the weather is not known yet.
func (m *Model) confidence(features Features, horizon time.Duration) float64 {
	// A sigma of 0.2 (±20%) is a very good model, 0.6 a poor one.
	quality := math.Max(0.2, math.Min(1, 1.3-1.5*m.Sigma))
	days := math.Max(0, horizon.Hours()/24)
	distance := 1 - 0.25*math.Min(1, days/120)
	if features.Weather == nil {
		distance *= 0.9
	}
	return math.Max(0, math.Min(1, quality*distance))
}

func round(value float64) float64 {
	return math.Round(value*100) / 100
}
