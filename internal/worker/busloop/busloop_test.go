package busloop

import (
	"testing"

	"github.com/kjsst/sh-mvdos/internal/redisbus"
)

func TestStopRunIsolationUsesMatchesRun(t *testing.T) {
	// Workers must only cancel runs whose run_id matches the stop event.
	if redisbus.MatchesRun("run-a", "run-b") {
		t.Fatal("stop for run-a must not match active run-b")
	}
	if !redisbus.MatchesRun("run-a", "run-a") {
		t.Fatal("stop for run-a must match active run-a")
	}
	if redisbus.MatchesRun("", "run-a") {
		t.Fatal("empty stop run_id must not cancel active run")
	}
	if redisbus.MatchesRun("run-a", "") {
		t.Fatal("stop must not match empty active run")
	}
}