package learnattack

import (
	"context"
	"net/http"
	"time"
)

// ProbeSample is one victim health check during a learn-and-attack round.
type ProbeSample struct {
	LatencyMs  float64 `json:"latency_ms"`
	StatusCode int     `json:"status_code"`
	Reachable  bool    `json:"reachable"`
	Error      string  `json:"error,omitempty"`
}

func Probe(ctx context.Context, target string) ProbeSample {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return ProbeSample{Error: err.Error()}
	}
	client := &http.Client{
		Timeout:       8 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return nil },
	}
	resp, err := client.Do(req)
	elapsed := time.Since(start).Seconds() * 1000
	if err != nil {
		return ProbeSample{LatencyMs: elapsed, Reachable: false, Error: err.Error()}
	}
	defer resp.Body.Close()
	_, _ = ioDrain(resp.Body, 32*1024)
	return ProbeSample{
		LatencyMs:  elapsed,
		StatusCode: resp.StatusCode,
		Reachable:  true,
	}
}

// BaselineProbe averages N pre-attack probes so slow targets are not misread as impact.
func BaselineProbe(ctx context.Context, target string, samples int) ProbeSample {
	if samples <= 0 {
		samples = 3
	}
	var sum ProbeSample
	ok := 0
	for i := 0; i < samples; i++ {
		p := Probe(ctx, target)
		if !p.Reachable {
			continue
		}
		sum.LatencyMs += p.LatencyMs
		sum.StatusCode = p.StatusCode
		ok++
		if i < samples-1 {
			select {
			case <-ctx.Done():
				break
			case <-time.After(300 * time.Millisecond):
			}
		}
	}
	if ok == 0 {
		return ProbeSample{Reachable: false, Error: "baseline probes failed"}
	}
	sum.LatencyMs /= float64(ok)
	sum.Reachable = true
	return sum
}

func ioDrain(r interface{ Read([]byte) (int, error) }, limit int64) (int64, error) {
	buf := make([]byte, 4096)
	var n int64
	for n < limit {
		nr, err := r.Read(buf)
		n += int64(nr)
		if err != nil {
			return n, err
		}
	}
	return n, nil
}