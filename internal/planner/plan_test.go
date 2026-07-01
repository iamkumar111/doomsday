package planner_test

import (
	"testing"

	"github.com/kjsst/sh-mvdos/internal/planner"
	"github.com/kjsst/sh-mvdos/internal/vector"
)

func TestWeightedScaleSplit(t *testing.T) {
	reg, err := vector.LoadRegistry("../../configs/vectors.yaml")
	if err != nil {
		t.Fatal(err)
	}
	paths, err := vector.LoadPathProfiles("../../configs/path-profiles.yaml")
	if err != nil {
		t.Fatal(err)
	}
	plans, err := planner.LoadPlans("../../configs/combo-plans.yaml")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := planner.FindPlan(plans, "slayer-l7-mix")
	if err != nil {
		t.Fatal(err)
	}
	rp, err := planner.BuildRunPlan(reg, paths, plan, "https://lab.example", planner.RunScale{
		Workers: 500, Streams: 100, Batch: 200, MaxDurationSec: 300,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rp.Phases) != 4 {
		t.Fatalf("expected 4 phases, got %d", len(rp.Phases))
	}
	totalWorkers := 0
	for _, ph := range rp.Phases {
		totalWorkers += ph.Scale.Workers
	}
	if totalWorkers > 500 {
		t.Fatalf("weighted workers should not exceed budget: %d", totalWorkers)
	}
}