package main

import (
	"log/slog"
	"os"

	"github.com/kjsst/sh-mvdos/internal/dashboard"
	"github.com/kjsst/sh-mvdos/internal/guard"
	"github.com/kjsst/sh-mvdos/internal/planner"
	"github.com/kjsst/sh-mvdos/internal/vector"
)

func main() {
	ctx, err := guard.MustAuthorizeControlPlane(guard.Config{
		PolicyPath: env("POLICY_PATH", "data/lab-policy.yaml"),
		Vector:     "dashboard",
	})
	if err != nil {
		slog.Error("refused", "err", err)
		os.Exit(guard.ExitCode(err))
	}
	srv := dashboard.NewWithPaths(
		env("POLICY_PATH", "data/lab-policy.yaml"),
		env("PHASES_PATH", "configs/phases.yaml"),
		env("COMBOS_PATH", "configs/combos.yaml"),
		env("VECTORS_PATH", vector.DefaultVectorsPath),
		env("COMBO_PLANS_PATH", planner.DefaultComboPlansPath),
		env("PATH_PROFILES_PATH", vector.DefaultPathProfilesPath),
		env("REDIS_ADDR", "127.0.0.1:6379"),
		env("DASHBOARD_ADDR", "0.0.0.0:8089"),
		env("DASHBOARD_TOKEN", ""),
	)
	if err := srv.Run(ctx); err != nil {
		slog.Error("dashboard", "err", err)
		os.Exit(1)
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
