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
	vector := "ws-flood"

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
		Run: func(ctx context.Context, ev redisbus.PhaseEvent) (uint64, uint64) {
			wsPath := ""
			if ev.Params != nil {
				wsPath = ev.Params["ws_path"]
			}
			w := worker.WSFlood{
				Target:    ev.TargetURL,
				Workers:   ev.Workers,
				Streams:   ev.Streams,
				BatchSize: ev.BatchSize,
				WSPath:    wsPath,
			}
			reqs, errs, _ := w.Run(ctx)
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