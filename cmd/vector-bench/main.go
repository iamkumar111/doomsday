package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/kjsst/sh-mvdos/internal/bench"
	"github.com/kjsst/sh-mvdos/internal/guard"
	"github.com/kjsst/sh-mvdos/internal/labpolicy"
	vec "github.com/kjsst/sh-mvdos/internal/vector"
	"github.com/kjsst/sh-mvdos/internal/worker"
)

func main() {
	var (
		list       = flag.Bool("list", false, "list all vectors")
		all        = flag.Bool("all", false, "bench every vector sequentially")
		vector     = flag.String("vector", "", "vector id to bench (see -list)")
		target     = flag.String("target", "", "target URL (overrides policy)")
		duration   = flag.Duration("duration", 10*time.Second, "bench duration per vector")
		workers    = flag.Int("workers", 4, "worker goroutines")
		streams    = flag.Int("streams", 100, "H2 streams per burst")
		batch      = flag.Int("batch", 50, "L7 batch size")
		jsonOut    = flag.Bool("json", false, "JSON output")
		policyPath = flag.String("policy", "data/lab-policy.yaml", "lab policy path")
		skipProbe  = flag.Bool("skip-probe", false, "skip victim reachability check")
		matrix     = flag.Bool("matrix", false, "Slayer-class vector matrix (httpget, httppost, rudy, apiflood, h2, ws)")
		parity     = flag.Bool("parity", false, "with -matrix: print Slayer parity pass/fail gates")
		proxyFile  = flag.String("proxy", "", "proxy file (optional, Slayer -p parity)")
	)
	flag.Parse()

	if *list {
		for _, s := range worker.Catalog {
			fmt.Printf("%-18s %-10s %s\n  victim: %s\n  redis: %s\n  %s\n\n",
				s.ID, s.Layer, s.Label, s.VictimNeed, s.RedisVector, s.Description)
		}
		return
	}

	ctx, err := guard.MustAuthorize(guard.Config{
		PolicyPath: *policyPath,
		TargetURL:  *target,
		Vector:     "vector-bench",
	})
	if err != nil {
		slog.Error("refused", "err", err)
		os.Exit(guard.ExitCode(err))
	}

	policy, _ := labpolicy.Load(*policyPath)
	tgt := *target
	if tgt == "" {
		tgt = policy.TargetURL
	}

	opt := bench.Options{
		Target:   tgt,
		Duration: *duration,
		Workers:  *workers,
		Streams:  *streams,
		Batch:    *batch,
	}

	if !*skipProbe {
		probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if err := bench.ProbeTarget(probeCtx, tgt); err != nil {
			slog.Error("victim probe failed", "target", tgt, "err", err)
			os.Exit(1)
		}
		cancel()
		slog.Info("victim ok", "target", tgt)
	}

	if *matrix {
		mopt := bench.MatrixOptions{
			Target:    tgt,
			Duration:  *duration,
			Workers:   *workers,
			Streams:   *streams,
			Batch:     *batch,
			ProxyFile: strings.TrimSpace(*proxyFile),
		}
		mresults, err := bench.RunMatrix(ctx, mopt)
		if err != nil {
			slog.Error("matrix bench failed", "err", err)
			os.Exit(1)
		}
		if *jsonOut {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(mresults)
		} else {
			fmt.Print(bench.FormatMatrixTable(mresults))
			if *parity {
				var scores []bench.ParityScore
				for _, r := range mresults {
					scores = append(scores, bench.ScoreParity(r.VectorID, *workers, vec.RunResult{
						Attempts:        r.Attempts,
						Errors:          r.Errors,
						RPS:             r.RPS,
						OpenConnections: r.OpenConnections,
						Elapsed:         r.ElapsedSec,
					}))
				}
				fmt.Print(bench.FormatParityTable(scores))
				for _, s := range scores {
					if s.GatePct > 0 && !s.Passed {
						os.Exit(2)
					}
				}
			}
		}
		return
	}

	var results []bench.Result
	switch {
	case *all:
		results, err = bench.RunAll(ctx, opt)
	case *vector != "":
		var r bench.Result
		r, err = bench.RunVector(ctx, *vector, opt)
		if err == nil {
			results = []bench.Result{r}
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: vector-bench -list | -vector <id> | -all")
		flag.PrintDefaults()
		os.Exit(guard.ExitUsage)
	}
	if err != nil {
		slog.Error("bench failed", "err", err)
		os.Exit(1)
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(results)
	} else {
		fmt.Print(bench.FormatTable(results))
		ready := filterRedisReady(results)
		fmt.Printf("\nRedis integration candidates (%d): %s\n", len(ready), strings.Join(ready, ", "))
		fmt.Println("\nPhase 2: wire redis-ready vectors into configs/phases.yaml + worker binaries.")
	}
}

func filterRedisReady(results []bench.Result) []string {
	var ids []string
	for _, r := range results {
		if r.RedisReady {
			ids = append(ids, r.VectorID)
		}
	}
	return ids
}
