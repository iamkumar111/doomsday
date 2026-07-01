package busloop

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/kjsst/sh-mvdos/internal/guard"
	"github.com/kjsst/sh-mvdos/internal/redisbus"
)

// RunFunc executes one phase until ctx is canceled.
type RunFunc func(ctx context.Context, ev redisbus.PhaseEvent) (reqs, errs uint64)

// Options configures a Redis phase/stop worker loop.
type Options struct {
	Vector     string
	Vectors    []string // optional aliases; defaults to Vector
	PolicyPath string
	Bus        *redisbus.Client
	Run        RunFunc
}

func (opt Options) matchesVector(name string) bool {
	if name == opt.Vector {
		return true
	}
	for _, v := range opt.Vectors {
		if name == v {
			return true
		}
	}
	return false
}

type activeRun struct {
	cancel context.CancelFunc
	gen    uint64
	runID  string
}

// Serve blocks until ctx is canceled, handling run-scoped phase and stop events.
// Distinct phase IDs for the same vector may run concurrently; republishing the same phase ID replaces that run only.
func Serve(ctx context.Context, opt Options) {
	sub := opt.Bus.Subscribe(ctx, redisbus.ChannelPhase, redisbus.ChannelStop)
	defer sub.Close()

	var mu sync.Mutex
	active := make(map[string]activeRun)
	var runGen uint64

	cancelAll := func(match func(activeRun) bool) {
		mu.Lock()
		defer mu.Unlock()
		for key, ar := range active {
			if match == nil || match(ar) {
				ar.cancel()
				delete(active, key)
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			cancelAll(nil)
			return
		case msg, ok := <-sub.Channel():
			if !ok {
				cancelAll(nil)
				return
			}
			if msg.Channel == redisbus.ChannelStop {
				ev, _ := redisbus.DecodeStop(msg.Payload)
				if ev.RunID == "" {
					slog.Warn("worker ignored stop without run_id", "vector", opt.Vector, "reason", ev.Reason)
					continue
				}
				mu.Lock()
				before := len(active)
				mu.Unlock()
				cancelAll(func(ar activeRun) bool {
					return redisbus.MatchesRun(ev.RunID, ar.runID)
				})
				mu.Lock()
				after := len(active)
				mu.Unlock()
				if before > 0 && after == before {
					slog.Debug("worker ignored stop for other run",
						"vector", opt.Vector, "event_run", ev.RunID, "reason", ev.Reason)
				} else if before > after {
					slog.Info("worker stop handled", "vector", opt.Vector, "run_id", ev.RunID, "reason", ev.Reason)
				}
				continue
			}

			ev, err := redisbus.Decode[redisbus.PhaseEvent](msg.Payload)
			if err != nil || !opt.matchesVector(ev.Vector) {
				continue
			}
			if ev.RunID == "" {
				slog.Warn("worker ignored phase without run_id", "vector", opt.Vector, "phase", ev.PhaseID)
				continue
			}
			if verr := guard.MustValidatePolicyTarget(opt.PolicyPath, ev.TargetURL); verr != nil {
				slog.Error("worker refused target from redis", "vector", opt.Vector, "target", ev.TargetURL, "err", verr)
				continue
			}

			key := ev.RunID + ":" + ev.PhaseID
			mu.Lock()
			if prev, ok := active[key]; ok {
				prev.cancel()
			}
			runGen++
			gen := runGen
			runCtx, cancel := context.WithCancel(ctx)
			active[key] = activeRun{cancel: cancel, gen: gen, runID: ev.RunID}
			mu.Unlock()

			go func(ev redisbus.PhaseEvent, key string, gen uint64) {
				defer func() {
					mu.Lock()
					if cur, ok := active[key]; ok && cur.gen == gen {
						delete(active, key)
					}
					mu.Unlock()
					cancel()
				}()
				start := time.Now()
				reqs, errs := opt.Run(runCtx, ev)
				_ = opt.Bus.Publish(context.Background(), redisbus.ChannelMetrics, redisbus.MetricsFromPhase(
					ev, reqs, errs, float64(reqs)/time.Since(start).Seconds(),
				))
			}(ev, key, gen)
		}
	}
}