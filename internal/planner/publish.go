package planner

import (
	"time"

	"github.com/kjsst/sh-mvdos/internal/redisbus"
)

// ToPhaseEvents converts a run plan into Redis phase events.
func ToPhaseEvents(plan RunPlan, runID string, start time.Time) []redisbus.PhaseEvent {
	expires := start.Add(time.Duration(plan.MaxDurationSec) * time.Second)
	out := make([]redisbus.PhaseEvent, 0, len(plan.Phases))
	for _, ph := range plan.Phases {
		params := map[string]string{}
		if ph.Scale.ProxyFile != "" {
			params["proxy_file"] = ph.Scale.ProxyFile
		}
		if ph.Scale.WSPath != "" {
			params["ws_path"] = ph.Scale.WSPath
		}
		if ph.Worker == "l7-abuser" {
			params["mode"] = ph.Vector
		}
		if ph.Protocol != "" {
			params["protocol"] = ph.Protocol
		}
		ev := redisbus.PhaseEvent{
			RunID:     runID,
			PhaseID:   ph.PhaseID,
			Vector:    ph.RedisVector,
			TargetURL: plan.Target,
			Workers:   ph.Scale.Workers,
			Streams:   ph.Scale.Streams,
			BatchSize: ph.Scale.Batch,
			Params:    params,
			At:        start.Add(time.Duration(ph.StartAfterSec) * time.Second),
			ExpiresAt: expires,
		}
		out = append(out, ev)
	}
	return out
}