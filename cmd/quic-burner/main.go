package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/kjsst/sh-mvdos/internal/guard"
	"github.com/kjsst/sh-mvdos/internal/redisbus"
	"github.com/kjsst/sh-mvdos/internal/worker"
	"github.com/kjsst/sh-mvdos/internal/worker/busloop"
)

func main() {
	policyPath := env("POLICY_PATH", "data/lab-policy.yaml")
	redisAddr := env("REDIS_ADDR", "127.0.0.1:6379")
	vector := "quic-burner"

	ctx, err := guard.MustAuthorizeControlPlane(guard.Config{PolicyPath: policyPath, Vector: vector})
	if err != nil {
		os.Exit(guard.ExitCode(err))
	}
	bus := redisbus.New(redisAddr)
	slog.Info("worker ready", "vector", vector)

	busloop.Serve(ctx, busloop.Options{
		Vector:     vector,
		PolicyPath: policyPath,
		Bus:        bus,
		Run: func(ctx context.Context, ev redisbus.PhaseEvent, prog *busloop.PhaseProgress) (uint64, uint64) {
			if prog != nil {
				prog.ActualMode = "quic-burner"
				prog.Protocol = "quic"
			}
			w := worker.QUICBurner{Target: ev.TargetURL, Workers: ev.Workers, BatchSize: ev.BatchSize}
			reqs, errs, _ := w.Run(ctx)
			if prog != nil {
				prog.Attempts.Store(reqs)
				prog.Errors.Store(errs)
			}
			return reqs, errs
		},
	})
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}