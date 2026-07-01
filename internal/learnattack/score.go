package learnattack

import "math"

// DegradationScore measures target harm relative to baseline (not attacker throughput).
func DegradationScore(baseline, probe ProbeSample) float64 {
	if baseline.LatencyMs <= 0 {
		baseline.LatencyMs = 1
	}
	if !probe.Reachable {
		// Unreachable may be network blip — moderate score, not treated as success.
		return math.Max(0, baseline.LatencyMs*0.5)
	}
	delta := probe.LatencyMs - baseline.LatencyMs
	score := math.Max(0, delta)
	if probe.StatusCode >= 500 {
		score += 150
	} else if probe.StatusCode >= 400 && baseline.StatusCode < 400 {
		score += 40
	}
	// Ratio bonus: 2x slower than baseline is meaningful lab signal.
	ratio := probe.LatencyMs / baseline.LatencyMs
	if ratio >= 2 {
		score += 50 * (ratio - 1)
	}
	return score
}

// FeatureVector is the input to the online linear model.
type FeatureVector struct {
	LatencyDeltaNorm float64 `json:"latency_delta_norm"`
	Status5xx        float64 `json:"status_5xx"`
	Unreachable      float64 `json:"unreachable"`
	LogDeltaRPS      float64 `json:"log_delta_rps"`
	AttackerErrRate  float64 `json:"attacker_err_rate"`
}

func BuildFeatures(baseline, probe ProbeSample, deltaRPS, errRate float64) FeatureVector {
	base := baseline.LatencyMs
	if base <= 0 {
		base = 1
	}
	latNorm := 0.0
	if probe.Reachable {
		latNorm = (probe.LatencyMs - base) / base
	}
	s5 := 0.0
	if probe.Reachable && probe.StatusCode >= 500 {
		s5 = 1
	}
	unr := 0.0
	if !probe.Reachable {
		unr = 1
	}
	return FeatureVector{
		LatencyDeltaNorm: latNorm,
		Status5xx:        s5,
		Unreachable:        unr,
		LogDeltaRPS:        math.Log1p(math.Max(0, deltaRPS)),
		AttackerErrRate:    errRate,
	}
}

func (f FeatureVector) AsSlice() [5]float64 {
	return [5]float64{f.LatencyDeltaNorm, f.Status5xx, f.Unreachable, f.LogDeltaRPS, f.AttackerErrRate}
}

// OnlineModel is a lightweight online linear regressor (no pre-trained weights).
type OnlineModel struct {
	Weights [5]float64 `json:"weights"`
	Bias    float64    `json:"bias"`
}

func NewOnlineModel() OnlineModel {
	return OnlineModel{}
}

func (m *OnlineModel) Predict(f FeatureVector) float64 {
	x := f.AsSlice()
	p := m.Bias
	for i := range x {
		p += m.Weights[i] * x[i]
	}
	return p
}

// Update applies one SGD step toward observed degradation reward.
func (m *OnlineModel) Update(f FeatureVector, reward float64, lr float64) {
	x := f.AsSlice()
	pred := m.Predict(f)
	err := reward - pred
	m.Bias += lr * err
	for i := range x {
		m.Weights[i] += lr * err * x[i]
	}
}