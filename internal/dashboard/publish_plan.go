package dashboard

import (
	"context"
	"log/slog"
	"time"

	"github.com/kjsst/sh-mvdos/internal/planner"
	"github.com/kjsst/sh-mvdos/internal/redisbus"
)

func (s *Server) publishRunPlan(runID string, plan planner.RunPlan, base time.Time) {
	if runID == "" {
		return
	}
	events := planner.ToPhaseEvents(plan, runID, base)
	phaseIDs := make([]string, len(events))
	for i, ev := range events {
		phaseIDs[i] = ev.PhaseID
	}
	s.resetPhaseBatch(phaseIDs)
	s.persistRunState()

	for _, ev := range events {
		delay := time.Until(ev.At)
		if delay < 0 {
			delay = 0
		}
		go func(ev redisbus.PhaseEvent, d time.Duration) {
			if d > 0 {
				select {
				case <-time.After(d):
				case <-s.phaseCtx.Done():
					return
				}
			}
			if s.controlCtx != nil && s.controlCtx.Err() != nil {
				return
			}
			pubCtx, cancel := context.WithTimeout(s.phaseCtx, 5*time.Second)
			if err := s.bus.PublishPhase(pubCtx, ev); err != nil {
				slog.Error("dashboard planned phase publish failed", "run_id", ev.RunID, "id", ev.PhaseID, "err", err)
			} else {
				slog.Info("dashboard planned phase published", "run_id", ev.RunID, "id", ev.PhaseID, "vector", ev.Vector, "workers", ev.Workers)
			}
			cancel()
		}(ev, delay)
	}
}