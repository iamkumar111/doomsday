package registry

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kjsst/sh-mvdos/internal/redisbus"
)

const heartbeatStaleSec = 30

// PreflightRequest describes a planned run before publish.
type PreflightRequest struct {
	RequiredWorkers []string
	Workers         int
	Streams         int
}

// PreflightResult is the outcome of worker readiness checks.
type PreflightResult struct {
	OK         bool     `json:"ok"`
	Missing    []string `json:"missing,omitempty"`
	Saturated  []string `json:"saturated,omitempty"`
	Warnings   []string `json:"warnings,omitempty"`
	Workers    []redisbus.WorkerHeartbeat `json:"workers,omitempty"`
}

// Preflight verifies required workers are online and not over capacity.
func Preflight(ctx context.Context, bus *redisbus.Client, req PreflightRequest) (PreflightResult, error) {
	if bus == nil {
		return PreflightResult{}, fmt.Errorf("registry: nil bus")
	}
	online, err := bus.ListWorkers(ctx)
	if err != nil {
		return PreflightResult{}, err
	}
	byVector := make(map[string]redisbus.WorkerHeartbeat, len(online))
	now := time.Now().Unix()
	for _, hb := range online {
		if hb.Vector == "" {
			continue
		}
		if now-hb.LastSeen > heartbeatStaleSec {
			continue
		}
		byVector[hb.Vector] = hb
	}

	var missing, saturated, warnings []string
	for _, v := range req.RequiredWorkers {
		if v == "" {
			continue
		}
		hb, ok := byVector[v]
		if !ok {
			missing = append(missing, v)
			continue
		}
		cap := hb.Capacity
		if cap.MaxWorkers == 0 {
			def := LookupCapacity(v)
			cap = redisbus.WorkerCapacity{MaxWorkers: def.MaxWorkers, MaxStreams: def.MaxStreams}
		}
		if req.Workers > 0 && cap.MaxWorkers > 0 && req.Workers > cap.MaxWorkers {
			saturated = append(saturated, fmt.Sprintf("%s: plan needs %d workers, capacity %d", v, req.Workers, cap.MaxWorkers))
		}
		if req.Streams > 0 && cap.MaxStreams > 0 && req.Streams > cap.MaxStreams {
			warnings = append(warnings, fmt.Sprintf("%s: plan streams %d exceeds capacity %d", v, req.Streams, cap.MaxStreams))
		}
		if hb.ActiveRuns >= 3 {
			warnings = append(warnings, fmt.Sprintf("%s: %d active runs (may be saturated)", v, hb.ActiveRuns))
		}
	}
	return PreflightResult{
		OK:        len(missing) == 0,
		Missing:   missing,
		Saturated: saturated,
		Warnings:  warnings,
		Workers:   online,
	}, nil
}

// PreflightError formats a 503 response body.
func PreflightError(res PreflightResult) string {
	var parts []string
	if len(res.Missing) > 0 {
		parts = append(parts, "required workers offline: "+strings.Join(res.Missing, ", "))
	}
	if len(res.Saturated) > 0 {
		parts = append(parts, strings.Join(res.Saturated, "; "))
	}
	if len(parts) == 0 {
		return "preflight failed"
	}
	return strings.Join(parts, " — ")
}