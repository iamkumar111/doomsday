package bench

import (
	"testing"

	"github.com/kjsst/sh-mvdos/internal/vector"
)

func TestScoreParityRUDY(t *testing.T) {
	pass := ScoreParity("rudy", 500, vector.RunResult{OpenConnections: 460})
	if !pass.Passed {
		t.Fatalf("expected pass at 92%%, got %.1f", pass.VsSlayerPct)
	}
	fail := ScoreParity("rudy", 500, vector.RunResult{OpenConnections: 400})
	if fail.Passed {
		t.Fatalf("expected fail at 80%%")
	}
}

func TestScoreParityHTTPGet(t *testing.T) {
	s := ScoreParity("httpget", 100, vector.RunResult{RPS: 180})
	if s.VsSlayerPct < ParityGateHTTPGet {
		t.Fatalf("expected >= gate, got %.1f", s.VsSlayerPct)
	}
	if !s.Passed {
		t.Fatal("expected pass")
	}
}

func TestPctCapsAt100(t *testing.T) {
	if v := pct(200, 100); v != 100 {
		t.Fatalf("pct cap: %v", v)
	}
}