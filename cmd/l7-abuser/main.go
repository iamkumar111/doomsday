package main

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"github.com/kjsst/sh-mvdos/internal/guard"
	"github.com/kjsst/sh-mvdos/internal/redisbus"
	"github.com/kjsst/sh-mvdos/internal/vector"
	"github.com/kjsst/sh-mvdos/internal/worker"
	"github.com/kjsst/sh-mvdos/internal/worker/busloop"
	"github.com/kjsst/sh-mvdos/internal/worker/proxy"
)

func main() {
	policyPath := env("POLICY_PATH", "data/lab-policy.yaml")
	redisAddr := env("REDIS_ADDR", "127.0.0.1:6379")
	vectorsPath := env("VECTORS_PATH", vector.DefaultVectorsPath)
	vectorName := "l7-abuser"

	ctx, err := guard.MustAuthorizeControlPlane(guard.Config{PolicyPath: policyPath, Vector: vectorName})
	if err != nil {
		os.Exit(guard.ExitCode(err))
	}
	reg, err := vector.LoadRegistry(vectorsPath)
	if err != nil {
		slog.Error("vectors registry", "err", err)
		os.Exit(1)
	}
	bus := redisbus.New(redisAddr)
	slog.Info("worker ready", "vector", vectorName)

	busloop.Serve(ctx, busloop.Options{
		Vector:     vectorName,
		PolicyPath: policyPath,
		Bus:        bus,
		Run: func(ctx context.Context, ev redisbus.PhaseEvent, prog *busloop.PhaseProgress) (uint64, uint64) {
			mode := "baseline"
			phaseProxy := ""
			wsPath := ""
			if ev.Params != nil {
				if ev.Params["mode"] != "" {
					mode = ev.Params["mode"]
				}
				phaseProxy = ev.Params["proxy_file"]
				wsPath = ev.Params["ws_path"]
			}
			proxyFile := proxy.ResolveFile(policyPath, phaseProxy, env("PROXY_FILE", ""))
			spec, err := reg.Resolve(mode)
			if err != nil {
				spec, _ = reg.Resolve("baseline")
			}
			if prog != nil {
				prog.ActualMode = string(spec.ID)
				prog.Protocol = spec.Protocol
			}
			scale := vector.Scale{
				Workers:   ev.Workers,
				Streams:   ev.Streams,
				Batch:     ev.BatchSize,
				ProxyFile: strings.TrimSpace(proxyFile),
				WSPath:    wsPath,
			}
			w := worker.L7Abuser{
				Target:    ev.TargetURL,
				Workers:   scale.Workers,
				Streams:   scale.Streams,
				BatchSize: scale.Batch,
				Mode:      mode,
				ProxyFile: scale.ProxyFile,
			}
			if scale.Batch <= 0 && spec.ID == vector.IDRUDY {
				w.BatchSize = 1
			}
			if prog != nil {
				w.Progress = &worker.ProgressSink{
					Attempts:        &prog.Attempts,
					Errors:          &prog.Errors,
					OpenConnections: &prog.OpenConnections,
					PeakOpen:        &prog.PeakOpen,
				}
			}
			reqs, errs, runErr := w.Run(ctx)
			if runErr != nil && runErr != context.Canceled && runErr != context.DeadlineExceeded {
				slog.Debug("vector run ended", "mode", mode, "err", runErr)
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