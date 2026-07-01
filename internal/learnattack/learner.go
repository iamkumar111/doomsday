package learnattack

import (
	"context"
	"fmt"
	"math"
)

// Scale is the tuned attack parameters for one learn round.
type Scale struct {
	Workers   int    `json:"workers"`
	Streams   int    `json:"streams"`
	BatchSize int    `json:"batch_size"`
	Combo     string `json:"combo"`
}

// RoundInput is everything needed to score one experiment window.
type RoundInput struct {
	Probe       ProbeSample
	Deltas      []MetricDelta
	ActiveCombo string
}

// RoundResult is exposed to the dashboard live panel.
type RoundResult struct {
	Round          int           `json:"round"`
	Scale          Scale         `json:"scale"`
	Probe          ProbeSample   `json:"probe"`
	Baseline       ProbeSample   `json:"baseline,omitempty"`
	Degradation    float64       `json:"degradation"`
	ModelPredict   float64       `json:"model_predict"`
	BestDegradation float64      `json:"best_degradation"`
	TopVector      string        `json:"top_vector"` // highest round-local delta score (not victim attribution)
	ActiveCombo    string        `json:"active_combo"`
	Decision       string        `json:"decision"`
	Features       FeatureVector `json:"features,omitempty"`
}

// Learner uses online linear regression + combo UCB on victim degradation.
type Learner struct {
	Max        Scale
	Combos     []string
	Baseline   ProbeSample
	Model      OnlineModel
	ComboStats map[string]ComboStat
	Best       Scale
	BestScore  float64
	Round      int
	LastCombo  string
	History    []RoundResult
	statePath  string
	targetURL  string
}

func NewLearner(max Scale, combos []string, startCombo string) *Learner {
	if len(combos) == 0 {
		combos = []string{"baseline", "protocol-storm", "api-abuse"}
	}
	combo := startCombo
	if combo == "" {
		combo = combos[0]
	}
	w := maxInt(4, max.Workers/4)
	st := maxInt(50, max.Streams/4)
	b := maxInt(20, max.BatchSize/4)
	return &Learner{
		Max:        max,
		Combos:     combos,
		Model:      NewOnlineModel(),
		ComboStats: make(map[string]ComboStat),
		Best:       Scale{Workers: w, Streams: st, BatchSize: b, Combo: combo},
		LastCombo:  combo,
		statePath:  DefaultStatePath,
	}
}

func (l *Learner) SetBaseline(b ProbeSample) {
	l.Baseline = b
}

func (l *Learner) SetStatePath(path string) {
	l.statePath = path
}

func (l *Learner) Restore() bool {
	st, err := LoadState(l.statePath)
	if err != nil {
		return false
	}
	l.targetURL = st.TargetURL
	if st.Baseline.Reachable {
		l.Baseline = st.Baseline
	}
	l.Model = st.Model
	if st.ComboStats != nil {
		l.ComboStats = st.ComboStats
	}
	if st.Best.Workers > 0 {
		l.Best = clampScale(st.Best, l.Max)
	}
	l.BestScore = st.BestScore
	l.Round = st.Round
	if st.LastCombo != "" {
		l.LastCombo = st.LastCombo
	}
	if len(st.History) > 0 {
		l.History = st.History
	}
	return true
}

// EnsureBaseline keeps a restored baseline when target is unchanged; probes only when needed.
func (l *Learner) EnsureBaseline(ctx context.Context, target string) (ProbeSample, string) {
	if l.Baseline.Reachable && l.targetURL == target {
		return l.Baseline, "restored baseline for same target"
	}
	b := BaselineProbe(ctx, target, 3)
	l.SetBaseline(b)
	l.targetURL = target
	return b, "fresh baseline recorded for target"
}

func (l *Learner) Persist() {
	st := &PersistedState{
		TargetURL:  l.targetURL,
		Baseline:   l.Baseline,
		Model:      l.Model,
		ComboStats: l.ComboStats,
		Best:       l.Best,
		BestScore:  l.BestScore,
		Round:      l.Round,
		LastCombo:  l.LastCombo,
		History:    l.History,
	}
	_ = SaveState(l.statePath, st.export())
}

func (l *Learner) Next(in RoundInput) RoundResult {
	l.Round++
	degradation := DegradationScore(l.Baseline, in.Probe)
	deltaRPS, errRate := SummarizeDeltas(in.Deltas)
	features := BuildFeatures(l.Baseline, in.Probe, deltaRPS, errRate)
	predict := l.Model.Predict(features)

	// Attribute reward to the combo that was active this window.
	if in.ActiveCombo != "" {
		cs := l.ComboStats[in.ActiveCombo]
		cs.Pulls++
		cs.TotalReward += degradation
		l.ComboStats[in.ActiveCombo] = cs
	}

	l.Model.Update(features, degradation, 0.05)

	topVec := BestVectorByDelta(in.Deltas)
	next := l.Best
	decision := "hold scale"

	switch {
	case !in.Probe.Reachable:
		// Do not max-scale on probe failure — hold and reduce slightly.
		next.Workers = maxInt(2, next.Workers*9/10)
		next.Streams = maxInt(20, next.Streams*9/10)
		decision = "probe unreachable — hold scale (check target/network)"
	case errRate > 0.55:
		next = rotateCombo(l, next)
		next.BatchSize = maxInt(10, next.BatchSize*7/10)
		decision = fmt.Sprintf("attacker errors %.0f%% — rotate to %s", errRate*100, next.Combo)
	case degradation > l.BestScore:
		l.BestScore = degradation
		next = clampScale(next, l.Max)
		next.Workers = minInt(l.Max.Workers, maxInt(next.Workers+1, next.Workers*6/5))
		next.Streams = minInt(l.Max.Streams, next.Streams+15)
		decision = fmt.Sprintf("degradation improved (%.0f) — keep combo %s, scale up", degradation, next.Combo)
	case predict < l.BestScore*0.5 && l.Round%2 == 0:
		next = pickComboUCB(l, next)
		decision = "model under-predicts — UCB try combo " + next.Combo
	case l.Round%4 == 0:
		next = pickComboUCB(l, next)
		decision = "explore combo " + next.Combo
	default:
		next.Workers = minInt(l.Max.Workers, next.Workers+1)
		decision = "incremental workers +1"
	}

	next = clampScale(next, l.Max)
	l.Best = next
	l.LastCombo = next.Combo

	result := RoundResult{
		Round:           l.Round,
		Scale:           next,
		Probe:           in.Probe,
		Baseline:        l.Baseline,
		Degradation:     degradation,
		ModelPredict:    predict,
		BestDegradation: l.BestScore,
		TopVector:       topVec,
		ActiveCombo:     in.ActiveCombo,
		Decision:        decision,
		Features:        features,
	}
	l.History = append(l.History, result)
	l.Persist()
	return result
}

func pickComboUCB(l *Learner, current Scale) Scale {
	if len(l.Combos) <= 1 {
		return current
	}
	total := 0
	for _, c := range l.Combos {
		total += l.ComboStats[c].Pulls
	}
	if total == 0 {
		current.Combo = l.Combos[0]
		return current
	}
	bestCombo := current.Combo
	bestUCB := -1.0
	for _, c := range l.Combos {
		st := l.ComboStats[c]
		mean := 0.0
		if st.Pulls > 0 {
			mean = st.TotalReward / float64(st.Pulls)
		}
		ucb := mean + math.Sqrt(2*math.Log(float64(total+1))/float64(st.Pulls+1))
		if ucb > bestUCB {
			bestUCB = ucb
			bestCombo = c
		}
	}
	current.Combo = bestCombo
	return current
}

func rotateCombo(l *Learner, s Scale) Scale {
	if len(l.Combos) <= 1 {
		return s
	}
	idx := 0
	for i, c := range l.Combos {
		if c == s.Combo {
			idx = (i + 1) % len(l.Combos)
			break
		}
	}
	s.Combo = l.Combos[idx]
	return s
}

func clampScale(s, max Scale) Scale {
	s.Workers = minInt(max.Workers, maxInt(1, s.Workers))
	s.Streams = minInt(max.Streams, maxInt(10, s.Streams))
	s.BatchSize = minInt(max.BatchSize, maxInt(5, s.BatchSize))
	if s.Combo == "" {
		s.Combo = max.Combo
	}
	return s
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func IsLearnMode(mode string) bool {
	return mode == "learn-and-attack"
}