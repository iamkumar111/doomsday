package learnattack

import "github.com/kjsst/sh-mvdos/internal/redisbus"

// MetricDelta is the per-round change for one vector (not cumulative totals).
type MetricDelta struct {
	Vector   string  `json:"vector"`
	Requests uint64  `json:"requests"`
	Errors   uint64  `json:"errors"`
	RPS      float64 `json:"rps"`
	Dropped  bool    `json:"dropped,omitempty"` // vector was active last window but absent now
}

// Snapshot copies cumulative metrics for delta computation.
type Snapshot map[string]redisbus.MetricsEvent

func NewSnapshot(src map[string]redisbus.MetricsEvent) Snapshot {
	out := make(Snapshot, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// Delta returns per-vector changes since the previous snapshot.
func (prev Snapshot) Delta(cur map[string]redisbus.MetricsEvent) []MetricDelta {
	var out []MetricDelta
	seen := make(map[string]bool, len(cur))
	for name, now := range cur {
		seen[name] = true
		before, ok := prev[name]
		if !ok {
			out = append(out, MetricDelta{
				Vector: name, Requests: now.Requests, Errors: now.Errors, RPS: now.RPS,
			})
			continue
		}
		dr := int64(now.Requests) - int64(before.Requests)
		de := int64(now.Errors) - int64(before.Errors)
		if dr < 0 {
			dr = int64(now.Requests)
		}
		if de < 0 {
			de = int64(now.Errors)
		}
		dRPS := now.RPS - before.RPS
		if dRPS < 0 {
			dRPS = now.RPS
		}
		out = append(out, MetricDelta{
			Vector:   name,
			Requests: uint64(dr),
			Errors:   uint64(de),
			RPS:      dRPS,
		})
	}
	for name := range prev {
		if seen[name] {
			continue
		}
		out = append(out, MetricDelta{Vector: name, Dropped: true})
	}
	return out
}

func SummarizeDeltas(deltas []MetricDelta) (totalDeltaRPS float64, errRate float64) {
	var reqs, errs uint64
	for _, d := range deltas {
		totalDeltaRPS += d.RPS
		reqs += d.Requests
		errs += d.Errors
	}
	if reqs > 0 {
		errRate = float64(errs) / float64(reqs+errs)
	}
	return totalDeltaRPS, errRate
}

// BestVectorByDelta ranks vectors using only this round's per-vector deltas.
func BestVectorByDelta(deltas []MetricDelta) string {
	if len(deltas) == 0 {
		return ""
	}
	best := ""
	bestScore := -1.0
	for _, d := range deltas {
		if d.Dropped {
			continue
		}
		score := vectorDeltaScore(d)
		if score > bestScore {
			bestScore = score
			best = d.Vector
		}
	}
	return best
}

func vectorDeltaScore(d MetricDelta) float64 {
	if d.Requests == 0 && d.RPS <= 0 {
		return 0
	}
	success := 1.0
	total := d.Requests + d.Errors
	if total > 0 {
		success = 1.0 - float64(d.Errors)/float64(total)
	}
	// Round-local signal: delta RPS weighted by success rate and volume.
	vol := float64(d.Requests)
	if vol < 1 {
		vol = 1
	}
	return d.RPS * success * (1 + vol/100)
}