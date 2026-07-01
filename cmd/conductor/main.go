package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/kjsst/sh-mvdos/internal/guard"
	"github.com/kjsst/sh-mvdos/internal/labpolicy"
	"github.com/kjsst/sh-mvdos/internal/orchestrator"
	"github.com/kjsst/sh-mvdos/internal/redisbus"
)

func main() {
	policyPath := env("POLICY_PATH", "data/lab-policy.yaml")
	phasesPath := env("PHASES_PATH", "configs/phases.yaml")
	combosPath := env("COMBOS_PATH", "configs/combos.yaml")
	redisAddr := env("REDIS_ADDR", "127.0.0.1:6379")

	ctx, err := guard.MustAuthorizeControlPlane(guard.Config{PolicyPath: policyPath, Vector: "conductor"})
	if err != nil {
		slog.Error("refused", "err", err)
		os.Exit(guard.ExitCode(err))
	}

	policy, err := labpolicy.Load(policyPath)
	if err != nil {
		slog.Error("policy", "err", err)
		os.Exit(1)
	}
	if policy.ConductorMode != "auto" {
		slog.Info("conductor idle — only runs when conductor_mode=auto",
			"mode", policy.ConductorMode,
			"hint", "use dashboard for manual/hybrid/learn runs, or set conductor_mode: auto",
		)
		<-ctx.Done()
		return
	}

	if err := guard.MustValidatePolicyTarget(policyPath, policy.TargetURL); err != nil {
		slog.Error("target not authorized for auto run", "err", err)
		os.Exit(guard.ExitCode(err))
	}

	phases, err := orchestrator.LoadPhases(phasesPath)
	if err != nil {
		slog.Error("phases", "err", err)
		os.Exit(1)
	}
	combos, err := orchestrator.LoadCombos(combosPath)
	if err != nil {
		slog.Error("combos", "err", err)
		os.Exit(1)
	}
	combo, err := orchestrator.FindCombo(combos, policy.Combo)
	if err != nil {
		slog.Error("combo", "err", err)
		os.Exit(1)
	}
	selected := orchestrator.SelectPhases(phases, combo)
	bus := redisbus.New(redisAddr)
	runID := newRunID()
	start := time.Now()
	schedule := orchestrator.ScheduleStart(start, selected)

	slog.Info("conductor auto run starting", "run_id", runID, "combo", policy.Combo, "phases", len(selected))

	for _, ph := range selected {
		targetTime := schedule[ph.ID]
		wait := time.Until(targetTime)
		if wait > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
		}
		if err := guard.MustValidatePolicyTarget(policyPath, policy.TargetURL); err != nil {
			slog.Error("refusing phase publish — target no longer authorized", "phase", ph.ID, "err", err)
			continue
		}
		ev := orchestrator.BuildPhaseEvent(ph, runID, policy.TargetURL, policy.Workers, policy.Streams, policy.BatchSize, start, policy.L7Mode, policy.ProxyFile)
		ev.At = targetTime
		if err := bus.Publish(ctx, redisbus.ChannelPhase, ev); err != nil {
			slog.Error("publish", "phase", ph.ID, "err", err)
		} else {
			slog.Info("phase published", "run_id", runID, "id", ph.ID, "vector", ph.Vector)
		}
	}

	maxDur := policy.MaxDurationSec
	if maxDur <= 0 {
		maxDur = 300
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Duration(maxDur) * time.Second):
		_ = bus.Publish(context.Background(), redisbus.ChannelStop, redisbus.StopEvent{
			RunID: runID, Reason: "conductor_max_duration",
		})
	}
}

func newRunID() string {
	return "run-auto-" + time.Now().Format("20060102-150405")
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}