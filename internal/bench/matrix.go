package bench

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kjsst/sh-mvdos/internal/vector"
)

// MatrixOptions tunes Slayer-parity benchmark runs.
type MatrixOptions struct {
	Target      string
	Duration    time.Duration
	Workers     int
	Streams     int
	Batch       int
	ProxyFile   string
	VectorsPath string
}

// MatrixResult compares core vectors on the same target/scale.
type MatrixResult struct {
	VectorID        string  `json:"vector_id"`
	Protocol        string  `json:"protocol"`
	Attempts        uint64  `json:"attempts"`
	Errors          uint64  `json:"errors"`
	OpenConnections uint64  `json:"open_connections,omitempty"`
	RPS             float64 `json:"rps"`
	ElapsedSec      float64 `json:"elapsed_sec"`
	VsSlayerPct     float64 `json:"vs_slayer_pct,omitempty"`
	ParityPassed    bool    `json:"parity_passed,omitempty"`
}

// RunMatrix benchmarks Slayer-class vectors using the unified vector engine.
func RunMatrix(ctx context.Context, opt MatrixOptions) ([]MatrixResult, error) {
	reg, err := vector.LoadRegistry(opt.VectorsPath)
	if err != nil {
		return nil, err
	}
	if opt.Duration <= 0 {
		opt.Duration = 30 * time.Second
	}
	ids := []string{"httpget", "httppost", "rudy", "apiflood", "h2-rapid-reset", "ws-flood"}
	var out []MatrixResult
	for _, id := range ids {
		spec, err := reg.Resolve(id)
		if err != nil {
			continue
		}
		scale := vector.Scale{
			Workers:   opt.Workers,
			Streams:   opt.Streams,
			Batch:     opt.Batch,
			ProxyFile: opt.ProxyFile,
		}
		scale = spec.FillDefaults(spec.ApplyCapabilities(scale))
		runCtx, cancel := context.WithTimeout(ctx, opt.Duration)
		res, _ := vector.Run(runCtx, spec, opt.Target, scale)
		cancel()
		score := ScoreParity(id, scale.Workers, res)
		out = append(out, MatrixResult{
			VectorID:        string(res.VectorID),
			Protocol:        res.Protocol,
			Attempts:        res.Attempts,
			Errors:          res.Errors,
			OpenConnections: res.OpenConnections,
			RPS:             res.RPS,
			ElapsedSec:      res.Elapsed,
			VsSlayerPct:     score.VsSlayerPct,
			ParityPassed:    score.Passed,
		})
	}
	return out, nil
}

func FormatMatrixTable(results []MatrixResult) string {
	sort.Slice(results, func(i, j int) bool { return results[i].VectorID < results[j].VectorID })
	var b strings.Builder
	b.WriteString(fmt.Sprintf("\n%-18s %-10s %10s %8s %6s %8s %8s\n",
		"VECTOR", "PROTOCOL", "ATTEMPTS", "ERRORS", "OPEN", "RPS", "SLAYER%"))
	b.WriteString(strings.Repeat("-", 78) + "\n")
	for _, r := range results {
		b.WriteString(fmt.Sprintf("%-18s %-10s %10d %8d %6d %8.1f %7.1f%%\n",
			r.VectorID, r.Protocol, r.Attempts, r.Errors, r.OpenConnections, r.RPS, r.VsSlayerPct))
	}
	return b.String()
}