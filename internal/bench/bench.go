package bench

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/kjsst/sh-mvdos/internal/worker"
)

type Options struct {
	Target   string
	Duration time.Duration
	Workers  int
	Streams  int
	Batch    int
}

type Result struct {
	VectorID      string  `json:"vector_id"`
	Label         string  `json:"label"`
	Layer         string  `json:"layer"`
	VictimNeed    string  `json:"victim_need"`
	DurationSec   float64 `json:"duration_sec"`
	Requests      uint64  `json:"requests"`
	Errors        uint64  `json:"errors"`
	RPS           float64 `json:"rps"`
	SuccessPct    float64 `json:"success_pct"`
	Effectiveness string  `json:"effectiveness"` // low | medium | high
	Effort        string  `json:"effort"`        // low | medium | high (attacker cost)
	RedisReady    bool    `json:"redis_ready"`
	Notes         string  `json:"notes,omitempty"`
}

func ProbeTarget(ctx context.Context, target string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("victim unreachable: %w", err)
	}
	resp.Body.Close()
	return nil
}

func RunVector(ctx context.Context, vectorID string, opt Options) (Result, error) {
	spec, err := worker.FindSpec(vectorID)
	if err != nil {
		return Result{}, err
	}
	if opt.Duration <= 0 {
		opt.Duration = 10 * time.Second
	}
	runner, err := worker.NewRunner(vectorID, worker.Config{
		Target:    opt.Target,
		Workers:   opt.Workers,
		Streams:   opt.Streams,
		BatchSize: opt.Batch,
	})
	if err != nil {
		return Result{}, err
	}
	runCtx, cancel := context.WithTimeout(ctx, opt.Duration)
	defer cancel()
	start := time.Now()
	reqs, errs, runErr := runner.Run(runCtx)
	elapsed := time.Since(start)
	if elapsed < time.Millisecond {
		elapsed = opt.Duration
	}

	total := reqs + errs
	successPct := 100.0
	if total > 0 {
		successPct = float64(reqs) / float64(total) * 100
	}
	rps := float64(reqs) / elapsed.Seconds()

	res := Result{
		VectorID:    spec.ID,
		Label:       spec.Label,
		Layer:       spec.Layer,
		VictimNeed:  spec.VictimNeed,
		DurationSec: elapsed.Seconds(),
		Requests:    reqs,
		Errors:      errs,
		RPS:         rps,
		SuccessPct:  successPct,
	}
	res.Effectiveness, res.Effort, res.RedisReady, res.Notes = score(spec, reqs, rps, successPct, runErr)
	return res, nil
}

func RunAll(ctx context.Context, opt Options) ([]Result, error) {
	var out []Result
	for _, spec := range worker.Catalog {
		r, err := RunVector(ctx, spec.ID, opt)
		if err != nil {
			return out, err
		}
		out = append(out, r)
	}
	return out, nil
}

func score(spec worker.Spec, reqs uint64, rps, successPct float64, runErr error) (effectiveness, effort string, redisReady bool, notes string) {
	var notesParts []string

	// Effectiveness: throughput + success on victim
	switch {
	case rps >= 500 && successPct >= 90:
		effectiveness = "high"
	case rps >= 50 && successPct >= 70:
		effectiveness = "medium"
	default:
		effectiveness = "low"
	}

	// Effort: attacker-side waste (errors) + victim requirements
	errRate := 100 - successPct
	switch {
	case errRate <= 10 && !strings.Contains(spec.VictimNeed, "HTTP/2") && !strings.Contains(spec.VictimNeed, "HTTP/3"):
		effort = "low"
	case errRate <= 40:
		effort = "medium"
	default:
		effort = "high"
	}

	if spec.ID == "slowloris" {
		notesParts = append(notesParts, "scores open connections held (not RPS)")
		if reqs >= 20 && successPct >= 60 {
			effectiveness = "medium"
			redisReady = true
		}
	}
	if spec.ID == "h2-rapid-reset" && successPct < 50 {
		notesParts = append(notesParts, "likely HTTP/1.1 victim — use Nginx HTTP/2")
	}
	if spec.ID == "quic-burner" && rps < 10 {
		notesParts = append(notesParts, "HTTP/3 victim required for full QUIC stress")
	}
	if runErr != nil && runErr != context.DeadlineExceeded && runErr != context.Canceled {
		notesParts = append(notesParts, runErr.Error())
	}

	redisReady = effectiveness != "low" || (spec.ID == "slowloris" && successPct >= 60)
	notes = strings.Join(notesParts, "; ")
	return effectiveness, effort, redisReady, notes
}

func FormatTable(results []Result) string {
	sort.Slice(results, func(i, j int) bool {
		return results[i].VectorID < results[j].VectorID
	})
	var b strings.Builder
	b.WriteString(fmt.Sprintf("\n%-18s %-8s %-8s %8s %7s %6s %-12s %-6s %s\n",
		"VECTOR", "EFFECT", "EFFORT", "RPS", "SUCC%", "REQS", "REDIS?", "LAYER", "NOTES"))
	b.WriteString(strings.Repeat("-", 100) + "\n")
	for _, r := range results {
		ready := "no"
		if r.RedisReady {
			ready = "yes"
		}
		b.WriteString(fmt.Sprintf("%-18s %-8s %-8s %8.1f %6.1f%% %6d %-12s %-6s %s\n",
			r.VectorID, r.Effectiveness, r.Effort, r.RPS, r.SuccessPct, r.Requests, ready, r.Layer, r.Notes))
	}
	return b.String()
}
