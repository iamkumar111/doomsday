package learnattack_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kjsst/sh-mvdos/internal/learnattack"
	"github.com/kjsst/sh-mvdos/internal/redisbus"
)

func baseline() learnattack.ProbeSample {
	return learnattack.ProbeSample{LatencyMs: 50, Reachable: true, StatusCode: 200}
}

func TestDegradationScoreUsesBaselineNotRPS(t *testing.T) {
	b := baseline()
	healthy := learnattack.ProbeSample{LatencyMs: 55, Reachable: true, StatusCode: 200}
	slow := learnattack.ProbeSample{LatencyMs: 300, Reachable: true, StatusCode: 200}
	if learnattack.DegradationScore(b, healthy) >= learnattack.DegradationScore(b, slow) {
		t.Fatal("slow target should score higher degradation than healthy")
	}
}

func TestUnreachableDoesNotMaxScale(t *testing.T) {
	l := learnattack.NewLearner(learnattack.Scale{Workers: 32, Streams: 200, BatchSize: 100, Combo: "baseline"}, []string{"baseline"}, "baseline")
	l.SetBaseline(baseline())
	l.Best = learnattack.Scale{Workers: 20, Streams: 100, BatchSize: 50, Combo: "baseline"}
	r := l.Next(learnattack.RoundInput{
		Probe:       learnattack.ProbeSample{Reachable: false, Error: "timeout"},
		ActiveCombo: "baseline",
	})
	if r.Scale.Workers >= 32 {
		t.Fatalf("unreachable probe must not max scale, got workers=%d", r.Scale.Workers)
	}
}

func TestMetricsDeltaWindow(t *testing.T) {
	prev := learnattack.NewSnapshot(map[string]redisbus.MetricsEvent{
		"l7-abuser": {Requests: 100, Errors: 5, RPS: 10},
	})
	cur := map[string]redisbus.MetricsEvent{
		"l7-abuser": {Requests: 250, Errors: 10, RPS: 25},
	}
	d := prev.Delta(cur)
	if len(d) != 1 || d[0].Requests != 150 || d[0].RPS != 15 {
		t.Fatalf("unexpected delta: %+v", d)
	}
}

func TestComboRotationOnHighErrors(t *testing.T) {
	l := learnattack.NewLearner(learnattack.Scale{Workers: 16, Streams: 100, BatchSize: 50}, []string{"baseline", "protocol-storm"}, "baseline")
	l.SetBaseline(baseline())
	l.Best = learnattack.Scale{Workers: 8, Streams: 50, BatchSize: 30, Combo: "baseline"}
	// Flat probe so degradation path does not mask error-rate rotation.
	r := l.Next(learnattack.RoundInput{
		Probe:       learnattack.ProbeSample{LatencyMs: 50, Reachable: true, StatusCode: 200},
		ActiveCombo: "baseline",
		Deltas: []learnattack.MetricDelta{
			{Vector: "l7-abuser", Requests: 10, Errors: 95, RPS: 5},
		},
	})
	if r.Scale.Combo != "protocol-storm" {
		t.Fatalf("expected combo rotation, got %s", r.Scale.Combo)
	}
}

func TestBestVectorByDeltaUsesRoundLocalSignal(t *testing.T) {
	deltas := []learnattack.MetricDelta{
		{Vector: "l7-abuser", RPS: 200, Requests: 200, Errors: 0},
		{Vector: "h2-thrasher", RPS: 50, Requests: 50, Errors: 0},
	}
	top := learnattack.BestVectorByDelta(deltas)
	if top != "l7-abuser" {
		t.Fatalf("expected top delta vector l7-abuser, got %s", top)
	}
}

func TestDeltaReportsDroppedVectors(t *testing.T) {
	prev := learnattack.NewSnapshot(map[string]redisbus.MetricsEvent{
		"l7-abuser": {Requests: 100, RPS: 10},
		"h2-thrasher": {Requests: 50, RPS: 5},
	})
	cur := map[string]redisbus.MetricsEvent{
		"l7-abuser": {Requests: 120, RPS: 12},
	}
	deltas := prev.Delta(cur)
	var dropped bool
	for _, d := range deltas {
		if d.Vector == "h2-thrasher" && d.Dropped {
			dropped = true
		}
	}
	if !dropped {
		t.Fatalf("expected dropped h2-thrasher in deltas: %+v", deltas)
	}
}

func TestEnsureBaselinePreservesRestored(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "learn.json")
	l := learnattack.NewLearner(learnattack.Scale{Workers: 16, Streams: 80, BatchSize: 40}, []string{"baseline"}, "baseline")
	l.SetBaseline(baseline())
	l.SetStatePath(path)
	// simulate persisted session
	_ = learnattack.SaveState(path, &learnattack.PersistedState{
		TargetURL: "http://127.0.0.1:8443",
		Baseline:  baseline(),
	})
	l2 := learnattack.NewLearner(learnattack.Scale{Workers: 16, Streams: 80, BatchSize: 40}, []string{"baseline"}, "baseline")
	l2.SetStatePath(path)
	l2.Restore()
	b, note := l2.EnsureBaseline(context.Background(), "http://127.0.0.1:8443")
	if note != "restored baseline for same target" {
		t.Fatalf("unexpected note: %s", note)
	}
	if b.LatencyMs != 50 {
		t.Fatalf("baseline overwritten: %+v", b)
	}
}

func TestClearStateRemovesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "learn.json")
	_ = os.WriteFile(path, []byte("{}"), 0o644)
	if err := learnattack.ClearState(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("expected file removed")
	}
}

func TestPersistedStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "learn.json")
	l := learnattack.NewLearner(learnattack.Scale{Workers: 16, Streams: 80, BatchSize: 40}, []string{"baseline"}, "baseline")
	l.SetBaseline(baseline())
	l.SetStatePath(path)
	l.Next(learnattack.RoundInput{
		Probe:       learnattack.ProbeSample{LatencyMs: 200, Reachable: true, StatusCode: 200},
		ActiveCombo: "baseline",
	})
	l2 := learnattack.NewLearner(learnattack.Scale{Workers: 16, Streams: 80, BatchSize: 40}, []string{"baseline"}, "baseline")
	l2.SetStatePath(path)
	l2.Restore()
	if l2.Round == 0 {
		t.Fatal("expected restored round > 0")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestScaleUpOnDegradationImprovement(t *testing.T) {
	l := learnattack.NewLearner(learnattack.Scale{Workers: 32, Streams: 200, BatchSize: 100}, []string{"baseline"}, "baseline")
	l.SetBaseline(baseline())
	startWorkers := l.Best.Workers
	r := l.Next(learnattack.RoundInput{
		Probe:       learnattack.ProbeSample{LatencyMs: 400, Reachable: true, StatusCode: 503},
		ActiveCombo: "baseline",
		Deltas:      []learnattack.MetricDelta{{Vector: "l7-abuser", RPS: 10, Requests: 50, Errors: 1}},
	})
	if r.Scale.Workers <= startWorkers {
		t.Fatalf("expected scale up on degradation, %d -> %d", startWorkers, r.Scale.Workers)
	}
}

func TestMaxCapsRespected(t *testing.T) {
	l := learnattack.NewLearner(learnattack.Scale{Workers: 8, Streams: 40, BatchSize: 20}, []string{"baseline"}, "baseline")
	l.SetBaseline(baseline())
	l.Best = learnattack.Scale{Workers: 8, Streams: 40, BatchSize: 20, Combo: "baseline"}
	for i := 0; i < 10; i++ {
		r := l.Next(learnattack.RoundInput{
			Probe:       learnattack.ProbeSample{LatencyMs: 500, Reachable: true, StatusCode: 500},
			ActiveCombo: "baseline",
		})
		if r.Scale.Workers > 8 || r.Scale.Streams > 40 || r.Scale.BatchSize > 20 {
			t.Fatalf("caps exceeded: %+v", r.Scale)
		}
	}
}