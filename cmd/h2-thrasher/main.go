package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/kjsst/sh-mvdos/internal/guard"
	"github.com/kjsst/sh-mvdos/internal/redisbus"
	"github.com/kjsst/sh-mvdos/internal/vector"
	"github.com/kjsst/sh-mvdos/internal/worker/busloop"
)

func main() {
	policyPath := env("POLICY_PATH", "data/lab-policy.yaml")
	redisAddr := env("REDIS_ADDR", "127.0.0.1:6379")
	vectorsPath := env("VECTORS_PATH", vector.DefaultVectorsPath)
	vectorName := "h2-thrasher"

	ctx, err := guard.MustAuthorizeControlPlane(guard.Config{PolicyPath: policyPath, Vector: vectorName})
	if err != nil {
		os.Exit(guard.ExitCode(err))
	}
	reg, err := vector.LoadRegistry(vectorsPath)
	if err != nil {
		slog.Error("vectors registry", "err", err)
		os.Exit(1)
	}
	spec, _ := reg.Resolve("h2-rapid-reset")
	bus := redisbus.New(redisAddr)
	slog.Info("worker ready", "vector", vectorName)

	busloop.Serve(ctx, busloop.Options{
		Vector:     vectorName,
		Vectors:    []string{"h2-rapid-reset"},
		PolicyPath: policyPath,
		Bus:        bus,
		Run: func(ctx context.Context, ev redisbus.PhaseEvent) (uint64, uint64) {
			scale := vector.Scale{
				Workers: ev.Workers,
				Streams: ev.Streams,
				Batch:   ev.BatchSize,
			}
			res, _ := vector.Run(ctx, spec, ev.TargetURL, scale)
			return res.Attempts, res.Errors
		},
	})
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}