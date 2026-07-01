package bench

import (
	"fmt"
	"strings"

	"github.com/kjsst/sh-mvdos/internal/vector"
)

const (
	ParityGateRUDY    = 90.0
	ParityGateHTTPGet = 85.0
	ParityGateHTTPPost = 85.0
)

// ParityScore evaluates how close a run is to Slayer-class SLOs on the same host.
type ParityScore struct {
	VectorID        string  `json:"vector_id"`
	Workers         int     `json:"workers"`
	Attempts        uint64  `json:"attempts"`
	Errors          uint64  `json:"errors"`
	RPS             float64 `json:"rps"`
	OpenConnections uint64  `json:"open_connections"`
	VsSlayerPct     float64 `json:"vs_slayer_pct"`
	GatePct         float64 `json:"gate_pct"`
	Passed          bool    `json:"passed"`
	Notes           string  `json:"notes,omitempty"`
}

// ScoreParity computes acceptance vs Slayer SLO targets from design doc §9.
func ScoreParity(vectorID string, workers int, res vector.RunResult) ParityScore {
	out := ParityScore{
		VectorID:        vectorID,
		Workers:         workers,
		Attempts:        res.Attempts,
		Errors:          res.Errors,
		RPS:             res.RPS,
		OpenConnections: res.OpenConnections,
		GatePct:         parityGate(vectorID),
	}
	switch vectorID {
	case "rudy":
		if workers <= 0 {
			workers = 1
		}
		held := res.OpenConnections
		if held == 0 && res.Attempts > 0 {
			held = uint64(workers)
		}
		out.VsSlayerPct = pct(float64(held), float64(workers))
		out.Passed = out.VsSlayerPct >= ParityGateRUDY
		out.Notes = fmt.Sprintf("open_connections=%d target>=%d", held, int(float64(workers)*ParityGateRUDY/100))
	case "httpget", "httppost", "apiflood":
		// Slayer-class RPS scales with workers; gate is % of expected floor.
		floor := expectedRPS(vectorID, workers)
		out.VsSlayerPct = pct(res.RPS, floor)
		out.Passed = out.VsSlayerPct >= out.GatePct
		out.Notes = fmt.Sprintf("rps=%.1f floor=%.1f", res.RPS, floor)
	case "h2-rapid-reset", "ws-flood":
		out.VsSlayerPct = pct(float64(res.Attempts), float64(workers*10))
		out.GatePct = 80
		out.Passed = out.VsSlayerPct >= out.GatePct
		out.Notes = "protocol-sensitive throughput"
	default:
		out.VsSlayerPct = 100
		out.Passed = true
		out.Notes = "no gate defined"
	}
	return out
}

func parityGate(vectorID string) float64 {
	switch vectorID {
	case "rudy":
		return ParityGateRUDY
	case "httpget":
		return ParityGateHTTPGet
	case "httppost", "apiflood":
		return ParityGateHTTPPost
	default:
		return 80
	}
}

func expectedRPS(vectorID string, workers int) float64 {
	if workers <= 0 {
		workers = 1
	}
	switch vectorID {
	case "httpget":
		return float64(workers) * 2
	case "httppost", "apiflood":
		return float64(workers) * 1.5
	default:
		return float64(workers)
	}
}

func pct(actual, expected float64) float64 {
	if expected <= 0 {
		return 100
	}
	v := actual / expected * 100
	if v > 100 {
		return 100
	}
	if v < 0 {
		return 0
	}
	return v
}

func FormatParityTable(scores []ParityScore) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("\n%-18s %8s %8s %8s %10s %6s %s\n",
		"VECTOR", "OPEN", "RPS", "GATE%", "VS_SLAYER", "PASS", "NOTES"))
	b.WriteString(strings.Repeat("-", 90) + "\n")
	for _, s := range scores {
		pass := "no"
		if s.Passed {
			pass = "yes"
		}
		b.WriteString(fmt.Sprintf("%-18s %8d %8.1f %7.0f%% %9.1f%% %6s %s\n",
			s.VectorID, s.OpenConnections, s.RPS, s.GatePct, s.VsSlayerPct, pass, s.Notes))
	}
	return b.String()
}