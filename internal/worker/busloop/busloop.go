package busloop

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kjsst/sh-mvdos/internal/guard"
	"github.com/kjsst/sh-mvdos/internal/labpolicy"
	"github.com/kjsst/sh-mvdos/internal/registry"
	"github.com/kjsst/sh-mvdos/internal/redisbus"
	"github.com/kjsst/sh-mvdos/internal/watchdog"
)

// PhaseProgress holds live counters published every 2s during a phase.
type PhaseProgress struct {
	Attempts        atomic.Uint64
	Errors          atomic.Uint64
	OpenConnections atomic.Uint64
	PeakOpen        atomic.Uint64
	ActualMode      string
	Protocol        string
}

// RunFunc executes one phase until ctx is canceled.
type RunFunc func(ctx context.Context, ev redisbus.PhaseEvent, prog *PhaseProgress) (reqs, errs uint64)

// Options configures a Redis phase/stop worker loop.
type Options struct {
	Vector     string
	Vectors    []string // optional aliases; defaults to Vector
	Version    string
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

type serveState struct {
	ctx    context.Context
	opt    Options
	policy *labpolicy.Policy
	mu     sync.Mutex
	active map[string]activeRun
	gen    uint64
}

// Serve blocks until ctx is canceled, handling run-scoped phase and stop events.
func Serve(ctx context.Context, opt Options) {
	sub := opt.Bus.Subscribe(ctx, redisbus.ChannelPhase, redisbus.ChannelStop)
	defer sub.Close()

	policy, _ := labpolicy.Load(opt.PolicyPath)
	if policy != nil && policy.WatchdogCPUPercent > 0 {
		wctx, wcancel := context.WithCancel(ctx)
		defer wcancel()
		go watchdog.Monitor(wctx, policy.WatchdogCPUPercent, wcancel)
		ctx = wctx
	}

	st := &serveState{
		ctx:    ctx,
		opt:    opt,
		policy: policy,
		active: make(map[string]activeRun),
	}

	go heartbeatLoop(ctx, st)
	go replayLoop(ctx, opt, st.replayPhase)
	st.replayOnce()

	for {
		select {
		case <-ctx.Done():
			st.cancelAll(nil)
			return
		case msg, ok := <-sub.Channel():
			if !ok {
				st.cancelAll(nil)
				return
			}
			if msg.Channel == redisbus.ChannelStop {
				ev, _ := redisbus.DecodeStop(msg.Payload)
				if ev.RunID == "" {
					slog.Warn("worker ignored stop without run_id", "vector", opt.Vector, "reason", ev.Reason)
					continue
				}
				st.mu.Lock()
				before := len(st.active)
				st.mu.Unlock()
				st.cancelAll(func(ar activeRun) bool {
					return redisbus.MatchesRun(ev.RunID, ar.runID)
				})
				st.mu.Lock()
				after := len(st.active)
				st.mu.Unlock()
				if before > 0 && after == before {
					slog.Debug("worker ignored stop for other run",
						"vector", opt.Vector, "event_run", ev.RunID, "reason", ev.Reason)
				} else if before > after {
					slog.Info("worker stop handled", "vector", opt.Vector, "run_id", ev.RunID, "reason", ev.Reason)
				}
				continue
			}

			ev, err := redisbus.Decode[redisbus.PhaseEvent](msg.Payload)
			if err != nil {
				continue
			}
			st.startPhase(ev)
		}
	}
}

func (st *serveState) replayOnce() {
	_ = st.opt.Bus.ReplayDuePhases(st.ctx, st.opt.Vector, st.replayPhase)
}

func (st *serveState) replayPhase(ev redisbus.PhaseEvent) {
	st.startPhaseInternal(ev, true)
}

func (st *serveState) startPhase(ev redisbus.PhaseEvent) {
	st.startPhaseInternal(ev, false)
}

func (st *serveState) startPhaseInternal(ev redisbus.PhaseEvent, replay bool) {
	if !st.opt.matchesVector(ev.Vector) {
		return
	}
	if ev.RunID == "" {
		slog.Warn("worker ignored phase without run_id", "vector", st.opt.Vector, "phase", ev.PhaseID)
		return
	}
	checkCtx, cancel := context.WithTimeout(st.ctx, 2*time.Second)
	stopped := st.opt.Bus.IsRunStopped(checkCtx, ev.RunID)
	cancel()
	if stopped {
		return
	}
	if verr := guard.MustValidatePolicyTarget(st.opt.PolicyPath, ev.TargetURL); verr != nil {
		slog.Error("worker refused target from redis", "vector", st.opt.Vector, "target", ev.TargetURL, "err", verr)
		return
	}

	key := ev.RunID + ":" + ev.PhaseID
	st.mu.Lock()
	if prev, ok := st.active[key]; ok {
		if replay {
			st.mu.Unlock()
			return
		}
		prev.cancel()
	}
	st.gen++
	gen := st.gen
	runCtx, runCancel := context.WithCancel(st.ctx)
	if !ev.ExpiresAt.IsZero() {
		dctx, dcancel := context.WithDeadline(runCtx, ev.ExpiresAt)
		runCtx = dctx
		go func() {
			<-dctx.Done()
			dcancel()
			runCancel()
		}()
	} else if st.policy != nil && st.policy.MaxDurationSec > 0 {
		dctx, dcancel := context.WithTimeout(runCtx, time.Duration(st.policy.MaxDurationSec)*time.Second)
		runCtx = dctx
		go func() {
			<-dctx.Done()
			dcancel()
			runCancel()
		}()
	}
	st.active[key] = activeRun{cancel: runCancel, gen: gen, runID: ev.RunID}
	st.mu.Unlock()

	go func(ev redisbus.PhaseEvent, key string, gen uint64) {
		defer func() {
			st.mu.Lock()
			if cur, ok := st.active[key]; ok && cur.gen == gen {
				delete(st.active, key)
			}
			st.mu.Unlock()
			runCancel()
		}()
		prog := &PhaseProgress{}
		if ev.Params != nil {
			prog.ActualMode = ev.Params["mode"]
			prog.Protocol = ev.Params["protocol"]
		}
		liveCtx, liveCancel := context.WithCancel(runCtx)
		defer liveCancel()
		go st.liveMetricsLoop(liveCtx, ev, prog)
		start := time.Now()
		reqs, errs := st.opt.Run(runCtx, ev, prog)
		elapsed := time.Since(start).Seconds()
		if elapsed < 0.001 {
			elapsed = 0.001
		}
		final := redisbus.MetricsFromPhase(ev, reqs, errs, float64(reqs)/elapsed)
		open := prog.PeakOpen.Load()
		if open == 0 {
			open = prog.OpenConnections.Load()
		}
		final.OpenConnections = open
		if prog.ActualMode != "" {
			final.ActualMode = prog.ActualMode
		}
		if prog.Protocol != "" {
			final.Protocol = prog.Protocol
		}
		_ = st.opt.Bus.Publish(context.Background(), redisbus.ChannelMetrics, final)
		_ = st.opt.Bus.WriteRunReceipt(context.Background(), ev.RunID, ev.PhaseID, final)
	}(ev, key, gen)
}

func (st *serveState) cancelAll(match func(activeRun) bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	for key, ar := range st.active {
		if match == nil || match(ar) {
			ar.cancel()
			delete(st.active, key)
		}
	}
}

func (st *serveState) activeRunCount() int {
	st.mu.Lock()
	defer st.mu.Unlock()
	return len(st.active)
}

func (st *serveState) liveMetricsLoop(ctx context.Context, ev redisbus.PhaseEvent, prog *PhaseProgress) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	publish := func() {
		reqs := prog.Attempts.Load()
		errs := prog.Errors.Load()
		m := redisbus.MetricsFromPhase(ev, reqs, errs, 0)
		m.OpenConnections = prog.OpenConnections.Load()
		if prog.ActualMode != "" {
			m.ActualMode = prog.ActualMode
		}
		if prog.Protocol != "" {
			m.Protocol = prog.Protocol
		}
		_ = st.opt.Bus.Publish(context.Background(), redisbus.ChannelMetrics, m)
	}
	publish()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			publish()
		}
	}
}

func heartbeatLoop(ctx context.Context, st *serveState) {
	host, _ := os.Hostname()
	vector := st.opt.Vector
	version := st.opt.Version
	if version == "" {
		version = os.Getenv("SHMV_VERSION")
	}
	if version == "" {
		version = "dev"
	}
	capSpec := registry.LookupCapacity(vector)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	touch := func() {
		tctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		_ = st.opt.Bus.TouchWorker(tctx, redisbus.WorkerHeartbeat{
			Vector:     vector,
			Version:    version,
			Host:       host,
			ActiveRuns: st.activeRunCount(),
			Capacity: redisbus.WorkerCapacity{
				MaxWorkers: capSpec.MaxWorkers,
				MaxStreams: capSpec.MaxStreams,
			},
		})
		cancel()
	}
	touch()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			touch()
		}
	}
}

func replayLoop(ctx context.Context, opt Options, handle func(redisbus.PhaseEvent)) {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = opt.Bus.ReplayDuePhases(ctx, opt.Vector, handle)
		}
	}
}