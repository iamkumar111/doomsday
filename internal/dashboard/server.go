package dashboard

import (
	"context"
	"embed"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fmt"

	"github.com/kjsst/sh-mvdos/internal/fsutil"
	"github.com/kjsst/sh-mvdos/internal/guard"
	"github.com/kjsst/sh-mvdos/internal/learnattack"
	"github.com/kjsst/sh-mvdos/internal/labpolicy"
	"github.com/kjsst/sh-mvdos/internal/orchestrator"
	"github.com/kjsst/sh-mvdos/internal/recon"
	"github.com/kjsst/sh-mvdos/internal/redisbus"
)

//go:embed static/*
var staticFS embed.FS

type Server struct {
	PolicyPath  string
	PhasesPath  string
	CombosPath  string
	RedisAddr   string
	Addr        string
	APIToken    string
	mu          sync.RWMutex
	run         RunState
	bus         *redisbus.Client
	policyDirty bool
	controlCtx  context.Context

	// live state from redis (scoped to current run)
	metrics         map[string]redisbus.MetricsEvent
	lastPhases      []string // phase ids confirmed via redis subscriber
	runL7Mode       string   // L7 mode snapshot for the active run
	pendingPhases   map[string]struct{}
	confirmedPhases map[string]struct{}
	subCtxCancel    context.CancelFunc

	// for cancelling delayed phase publishes on stop
	phaseCtx    context.Context
	phaseCancel context.CancelFunc

	learnMu     sync.RWMutex
	learnState  learnattack.RoundResult
	learnActive bool
	learnCancel context.CancelFunc

	runScale RunScale
}

type RunScale struct {
	Workers   int    `json:"workers"`
	Streams   int    `json:"streams"`
	BatchSize int    `json:"batch_size"`
	ProxyFile string `json:"proxy_file,omitempty"`
}

type RunState struct {
	RunID           string    `json:"run_id,omitempty"`
	Active          bool      `json:"active"`
	StartedAt       time.Time `json:"started_at,omitempty"`
	ExpiresAt       time.Time `json:"expires_at,omitempty"`
	Combo           string    `json:"combo"`
	Mode            string    `json:"mode"`
	Target          string    `json:"target"`
	VectorsOK       bool      `json:"vectors_ok"`
	PhasesPlanned   int       `json:"phases_planned,omitempty"`
	PhasesConfirmed int       `json:"phases_confirmed,omitempty"`
}

type persistedSnapshot struct {
	Run         RunState                         `json:"run"`
	RunL7Mode   string                           `json:"run_l7_mode,omitempty"`
	RunScale    RunScale                         `json:"run_scale,omitempty"`
	LearnActive bool                             `json:"learn_active,omitempty"`
	LastPhases  []string                         `json:"last_phases,omitempty"`
	Metrics     map[string]redisbus.MetricsEvent `json:"metrics,omitempty"`
}

var weakDashboardTokens = map[string]bool{
	"": true, "change-me-lab-token": true, "changeme": true, "lab-token": true,
}

func New(policyPath, phasesPath, combosPath, redisAddr, addr, apiToken string) *Server {
	s := &Server{
		PolicyPath: policyPath,
		PhasesPath: phasesPath,
		CombosPath: combosPath,
		RedisAddr:  redisAddr,
		Addr:       addr,
		APIToken:   apiToken,
		bus:        redisbus.New(redisAddr),
		metrics:    make(map[string]redisbus.MetricsEvent),
		lastPhases:      make([]string, 0, 8),
		pendingPhases:   make(map[string]struct{}),
		confirmedPhases: make(map[string]struct{}),
	}
	if weakDashboardTokens[strings.TrimSpace(apiToken)] {
		slog.Warn("DASHBOARD_TOKEN is empty or uses a placeholder — set a strong secret in data/runtime.env")
	}
	s.phaseCtx, s.phaseCancel = context.WithCancel(context.Background())
	s.loadPersistedRunState()
	return s
}

func (s *Server) loadPersistedRunState() {
	path := s.runSnapshotPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var snap persistedSnapshot
	if json.Unmarshal(data, &snap) != nil {
		return
	}
	// only restore recent runs that were still active (e.g. dashboard restart mid-attack)
	if !snap.Run.Active || snap.Run.StartedAt.IsZero() || time.Since(snap.Run.StartedAt) >= time.Hour {
		return
	}
	s.mu.Lock()
	s.run = snap.Run
	s.runL7Mode = snap.RunL7Mode
	s.runScale = snap.RunScale
	if len(snap.LastPhases) > 0 {
		s.lastPhases = append([]string(nil), snap.LastPhases...)
	}
	if len(snap.Metrics) > 0 {
		s.metrics = make(map[string]redisbus.MetricsEvent, len(snap.Metrics))
		for k, v := range snap.Metrics {
			s.metrics[k] = v
		}
	}
	s.mu.Unlock()
	s.learnMu.Lock()
	s.learnActive = snap.LearnActive
	s.learnMu.Unlock()
}

func (s *Server) persistRunState() {
	s.mu.RLock()
	snap := persistedSnapshot{
		Run:        s.run,
		RunL7Mode:  s.runL7Mode,
		RunScale:   s.runScale,
		LastPhases: append([]string(nil), s.lastPhases...),
		Metrics:    make(map[string]redisbus.MetricsEvent, len(s.metrics)),
	}
	for k, v := range s.metrics {
		snap.Metrics[k] = v
	}
	s.mu.RUnlock()
	s.learnMu.RLock()
	snap.LearnActive = s.learnActive
	s.learnMu.RUnlock()
	path := s.runSnapshotPath()
	data, _ := json.Marshal(snap)
	_ = fsutil.WriteFile(path, data, 0o644)
}

func (s *Server) resetPhaseBatch(phaseIDs []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingPhases = make(map[string]struct{}, len(phaseIDs))
	s.confirmedPhases = make(map[string]struct{}, len(phaseIDs))
	for _, id := range phaseIDs {
		if id != "" {
			s.pendingPhases[id] = struct{}{}
		}
	}
	s.run.PhasesPlanned = len(s.pendingPhases)
	s.run.PhasesConfirmed = 0
}

func (s *Server) notePhaseConfirmed(phaseID string) {
	if phaseID == "" {
		return
	}
	if _, pending := s.pendingPhases[phaseID]; !pending {
		return
	}
	if _, seen := s.confirmedPhases[phaseID]; seen {
		return
	}
	s.confirmedPhases[phaseID] = struct{}{}
	s.run.PhasesConfirmed = len(s.confirmedPhases)
}

func (s *Server) runSnapshotPath() string {
	dir := filepath.Dir(s.PolicyPath)
	if dir == "" || dir == "." {
		dir = "data"
	}
	return filepath.Join(dir, ".dashboard-run-snapshot.json")
}

func (s *Server) statusPayload() map[string]any {
	s.mu.RLock()
	run := s.run
	metrics := make(map[string]redisbus.MetricsEvent, len(s.metrics))
	for k, v := range s.metrics {
		metrics[k] = v
	}
	phases := append([]string(nil), s.lastPhases...)
	s.mu.RUnlock()

	s.learnMu.RLock()
	learn := s.learnState
	learnOn := s.learnActive
	s.learnMu.RUnlock()

	redisUp := s.redisOK()
	checkCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	onlineWorkers, _ := s.bus.ListWorkers(checkCtx)
	cancel()
	warning := ""
	if run.Active && run.PhasesConfirmed == 0 && !run.StartedAt.IsZero() && time.Since(run.StartedAt) > 10*time.Second {
		warning = "attack started but no phases confirmed on Redis — check workers are up and subscribed"
	}
	if run.Active && len(metrics) == 0 && !run.StartedAt.IsZero() && time.Since(run.StartedAt) > 20*time.Second {
		if warning != "" {
			warning += "; "
		}
		warning += "no worker metrics yet"
	}

	return map[string]any{
		"run":         run,
		"redis":       redisUp,
		"server":      s.Addr,
		"metrics":     metrics,
		"last_phases": phases,
		"learn":       map[string]any{"active": learnOn, "round": learn},
		"workers":     onlineWorkers,
		"warning":     warning,
	}
}

func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	api := func(pattern string, h http.HandlerFunc) {
		mux.HandleFunc(pattern, s.requireAPIAuth(h))
	}
	api("/api/status", s.handleStatus)
	api("/api/policy", s.handlePolicy)
	api("/api/attack/start", s.handleAttackStart)
	api("/api/attack/stop", s.handleAttackStop)
	api("/api/validate", s.handleValidate)
	api("/api/combos", s.handleCombos)
	api("/api/recon", s.handleRecon)
	api("/api/recon/apply", s.handleReconApply)
	api("/api/recon/draft", s.handleReconDraft)
	api("/api/recon/promote", s.handleReconPromote)
	api("/api/policy/allowed", s.handlePolicyAllowed)
	mux.HandleFunc("/api/auth/config", s.handleAuthConfig)

	srv := &http.Server{Addr: s.Addr, Handler: mux}
	if s.APIToken != "" {
		slog.Info("dashboard listening (API token required)", "addr", s.Addr)
	} else {
		slog.Warn("dashboard listening without DASHBOARD_TOKEN — API is unauthenticated")
	}
	if s.controlCtx != nil {
		go func() {
			<-s.controlCtx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = srv.Shutdown(shutdownCtx)
		}()
	}
	return srv.ListenAndServe()
}

// Run starts the subscriber background worker (using provided ctx for lifetime)
// then serves HTTP. Use this instead of ListenAndServe when you have an authorized ctx.
func (s *Server) Run(ctx context.Context) error {
	s.controlCtx = ctx
	subCtx, cancel := context.WithCancel(ctx)
	s.subCtxCancel = cancel
	if s.phaseCancel != nil {
		s.phaseCancel()
	}
	s.phaseCtx, s.phaseCancel = context.WithCancel(ctx)
	go s.runSubscriber(subCtx)
	go s.resumeActiveRunIfNeeded(ctx)

	return s.ListenAndServe()
}

func (s *Server) handleAuthConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"auth_required": s.APIToken != ""})
}

func (s *Server) runSubscriber(ctx context.Context) {
	if s.bus == nil {
		return
	}
	sub := s.bus.Subscribe(ctx, redisbus.ChannelPhase, redisbus.ChannelStop, redisbus.ChannelMetrics)
	defer sub.Close()

	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			switch msg.Channel {
			case redisbus.ChannelStop:
				ev, err := redisbus.Decode[redisbus.StopEvent](msg.Payload)
				if err != nil {
					ev = redisbus.StopEvent{Reason: msg.Payload}
				}
				if ev.RunID == "" {
					continue
				}
				s.mu.Lock()
				if redisbus.MatchesRun(ev.RunID, s.run.RunID) {
					s.run.Active = false
					s.run.PhasesConfirmed = 0
				}
				s.mu.Unlock()
				s.persistRunState()
				slog.Info("dashboard: received stop event", "run_id", ev.RunID, "reason", ev.Reason)
			case redisbus.ChannelPhase:
				ev, err := redisbus.Decode[redisbus.PhaseEvent](msg.Payload)
				if err == nil && ev.RunID != "" {
					s.mu.Lock()
					if redisbus.MatchesRun(ev.RunID, s.run.RunID) {
						s.run.Active = true
						s.notePhaseConfirmed(ev.PhaseID)
						s.lastPhases = append(s.lastPhases, ev.PhaseID)
						if len(s.lastPhases) > 10 {
							s.lastPhases = s.lastPhases[1:]
						}
					}
					s.mu.Unlock()
					s.persistRunState()
				}
			case redisbus.ChannelMetrics:
				var m redisbus.MetricsEvent
				if err := json.Unmarshal([]byte(msg.Payload), &m); err == nil && m.RunID != "" {
					s.mu.Lock()
					if redisbus.MatchesRun(m.RunID, s.run.RunID) {
						s.metrics[m.Vector] = m
						s.run.Active = true
						s.run.VectorsOK = true
					}
					s.mu.Unlock()
					s.persistRunState()
				}
			}
		}
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	f, err := staticFS.Open("static/index.html")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.Copy(w, f)
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.statusPayload())
}

func (s *Server) handlePolicy(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		p, err := labpolicy.Load(s.PolicyPath)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, p)
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		var draft labpolicy.Policy
		if err := json.Unmarshal(body, &draft); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		// Load existing to protect critical fields (ethics, lab_mode, allowed_hosts, max_duration)
		existing, err := labpolicy.Load(s.PolicyPath)
		if err != nil {
			existing = &labpolicy.Policy{}
		}
		// Merge only safe editable fields from draft
		if draft.TargetURL != "" {
			existing.TargetURL = draft.TargetURL
		}
		if draft.Combo != "" {
			existing.Combo = draft.Combo
		}
		if draft.ConductorMode != "" {
			existing.ConductorMode = draft.ConductorMode
		}
		if draft.Workers > 0 {
			existing.Workers = draft.Workers
		}
		if draft.Streams > 0 {
			existing.Streams = draft.Streams
		}
		if draft.BatchSize > 0 {
			existing.BatchSize = draft.BatchSize
		}
		if draft.WatchdogCPUPercent >= 0 {
			existing.WatchdogCPUPercent = draft.WatchdogCPUPercent
		}
		if draft.L7Mode != "" {
			existing.L7Mode = draft.L7Mode
		}
		if strings.Contains(string(body), "proxy_file") {
			existing.ProxyFile = strings.TrimSpace(draft.ProxyFile)
		}
		// lab_mode, ethics_ack, allowed_hosts remain authoritative in the policy file
		if err := existing.Save(s.PolicyPath); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"status": "saved"})
	default:
		http.Error(w, "method not allowed", 405)
	}
}

func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var draft labpolicy.Policy
	if err := json.NewDecoder(r.Body).Decode(&draft); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	// Always validate against the on-disk policy's allowed_hosts (UI may not send them)
	policy, err := labpolicy.Load(s.PolicyPath)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	host, err := draft.TargetHost()
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	ok := policy.IsHostAllowed(host)
	var blockErr string
	// Also enforce scheme check for consistency
	if uerr := guard.ValidateTarget(draft.TargetURL, policy); uerr != nil {
		ok = false
		blockErr = uerr.Error()
	}
	writeJSON(w, map[string]any{
		"ok":            ok,
		"host":          host,
		"blocked":       !ok,
		"allowed_hosts": policy.AllowedHosts,
		"error":         blockErr,
	})
}

func (s *Server) handleAttackStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if !s.controlPlaneReady(w) {
		return
	}
	p, err := labpolicy.Load(s.PolicyPath)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	// Full target authorization (scheme + allowed_hosts + ethics + lab_mode)
	if err := guard.MustValidatePolicyTarget(s.PolicyPath, p.TargetURL); err != nil {
		http.Error(w, err.Error(), 403)
		return
	}
	switch p.ConductorMode {
	case "auto":
		http.Error(w, "conductor_mode=auto; manual start disabled (start with docker compose --profile auto)", 409)
		return
	case "hybrid":
		// dashboard-owned manual/hybrid/learn runs
	default:
		// manual, learn-and-attack, etc.
	}
	if !s.redisOK() {
		http.Error(w, "redis unavailable — start redis or lab stack first", 503)
		return
	}
	phases, err := orchestrator.LoadPhases(s.PhasesPath)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	combos, err := orchestrator.LoadCombos(s.CombosPath)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	var learner *learnattack.Learner
	startCombo := p.Combo
	workers, streams, batch := p.Workers, p.Streams, p.BatchSize
	if learnattack.IsLearnMode(p.ConductorMode) {
		learner = learnattack.NewLearner(learnattack.Scale{
			Workers: p.Workers, Streams: p.Streams, BatchSize: p.BatchSize, Combo: p.Combo,
		}, comboIDs(combos), p.Combo)
		learnPath := filepath.Join("data", ".learn-state.json")
		learner.SetStatePath(learnPath)
		learner.Restore()
		baseCtx, baseCancel := context.WithTimeout(r.Context(), 15*time.Second)
		baseline, baseNote := learner.EnsureBaseline(baseCtx, p.TargetURL)
		baseCancel()
		workers = learner.Best.Workers
		streams = learner.Best.Streams
		batch = learner.Best.BatchSize
		startCombo = learner.Best.Combo
		s.learnMu.Lock()
		s.learnActive = true
		s.learnState = learnattack.RoundResult{
			Round: 0, Scale: learner.Best, Baseline: baseline,
			Decision: baseNote + " — learn-and-attack started",
		}
		s.learnMu.Unlock()
	} else {
		s.learnMu.Lock()
		s.learnActive = false
		s.learnState = learnattack.RoundResult{}
		s.learnMu.Unlock()
	}

	combo, err := orchestrator.FindCombo(combos, startCombo)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	selected := orchestrator.SelectPhases(phases, combo)
	if len(selected) == 0 {
		http.Error(w, "no phases selected for combo", 400)
		return
	}
	required := orchestrator.RequiredVectors(selected)
	checkCtx, checkCancel := context.WithTimeout(r.Context(), 3*time.Second)
	missing, err := s.bus.WorkersReady(checkCtx, required)
	checkCancel()
	if err != nil {
		http.Error(w, "worker registry check failed: "+err.Error(), 503)
		return
	}
	if len(missing) > 0 {
		http.Error(w, fmt.Sprintf("required workers offline: %s — start vector containers first", strings.Join(missing, ", ")), 503)
		return
	}

	now := time.Now()
	maxDur := p.MaxDurationSec
	if maxDur <= 0 {
		maxDur = 300
	}
	expires := now.Add(time.Duration(maxDur) * time.Second)

	runID := newRunID()
	if s.phaseCancel != nil {
		s.phaseCancel()
	}
	parent := s.controlCtx
	if parent == nil {
		parent = context.Background()
	}
	s.phaseCtx, s.phaseCancel = context.WithCancel(parent)

	s.mu.Lock()
	s.lastPhases = nil
	s.metrics = make(map[string]redisbus.MetricsEvent)
	s.runL7Mode = p.L7Mode
	s.runScale = RunScale{Workers: workers, Streams: streams, BatchSize: batch, ProxyFile: p.ProxyFile}
	s.run = RunState{
		RunID:           runID,
		Active:          true,
		StartedAt:       now,
		ExpiresAt:       expires,
		Combo:           startCombo,
		Mode:            p.ConductorMode,
		Target:          p.TargetURL,
		VectorsOK:       false,
	}
	s.mu.Unlock()
	s.persistRunState()
	s.publishPhases(runID, selected, p.TargetURL, workers, streams, batch, p.ProxyFile, now, expires)

	if learnattack.IsLearnMode(p.ConductorMode) && learner != nil {
		s.startLearnLoop(learner, p, combos, phases, expires)
	}

	s.armRunDeadline(expires, runID)
	payload := s.statusPayload()
	payload["status"] = "started"
	payload["run_id"] = runID
	writeJSON(w, payload)
}

func (s *Server) handleAttackStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if !s.controlPlaneReady(w) {
		return
	}
	s.mu.RLock()
	runID := s.run.RunID
	s.mu.RUnlock()
	_ = s.bus.PublishStop(context.Background(), redisbus.StopEvent{
		RunID: runID, Reason: "dashboard",
	})
	if s.phaseCancel != nil {
		s.phaseCancel()
	}
	if s.learnCancel != nil {
		s.learnCancel()
	}
	parent := s.controlCtx
	if parent == nil {
		parent = context.Background()
	}
	s.phaseCtx, s.phaseCancel = context.WithCancel(parent)
	s.learnMu.Lock()
	s.learnActive = false
	s.learnState = learnattack.RoundResult{}
	s.learnMu.Unlock()
	_ = learnattack.ClearState(filepath.Join("data", ".learn-state.json"))
	s.mu.Lock()
	s.run.Active = false
	s.run.RunID = ""
	s.run.PhasesPlanned = 0
	s.run.PhasesConfirmed = 0
	s.runL7Mode = ""
	s.metrics = make(map[string]redisbus.MetricsEvent)
	s.mu.Unlock()
	s.persistRunState()
	payload := s.statusPayload()
	payload["status"] = "stopped"
	writeJSON(w, payload)
}

func (s *Server) redisOK() bool {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return s.bus.Ping(ctx) == nil
}

func pickInt(phaseVal, policyVal int) int {
	return orchestrator.PickScale(phaseVal, policyVal)
}

func comboIDs(combos []orchestrator.Combo) []string {
	ids := make([]string, len(combos))
	for i, c := range combos {
		ids[i] = c.ID
	}
	return ids
}

func (s *Server) metricsCopy() map[string]redisbus.MetricsEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]redisbus.MetricsEvent, len(s.metrics))
	for k, v := range s.metrics {
		out[k] = v
	}
	return out
}

func (s *Server) startLearnLoop(learner *learnattack.Learner, p *labpolicy.Policy, combos []orchestrator.Combo, allPhases []orchestrator.Phase, expires time.Time) {
	if s.learnCancel != nil {
		s.learnCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.learnCancel = cancel

	snap := learnattack.NewSnapshot(s.metricsCopy())
	activeCombo := learner.LastCombo

	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				return
			case <-s.phaseCtx.Done():
				return
			case <-ticker.C:
				if time.Now().After(expires) {
					return
				}
				s.mu.RLock()
				active := s.run.Active
				s.mu.RUnlock()
				if !active {
					learner.Persist()
					return
				}

				cur := s.metricsCopy()
				deltas := snap.Delta(cur)
				snap = learnattack.NewSnapshot(cur)

				probeCtx, probeCancel := context.WithTimeout(ctx, 10*time.Second)
				probe := learnattack.Probe(probeCtx, p.TargetURL)
				probeCancel()

				result := learner.Next(learnattack.RoundInput{
					Probe:       probe,
					Deltas:      deltas,
					ActiveCombo: activeCombo,
				})
				activeCombo = result.Scale.Combo

				combo, err := orchestrator.FindCombo(combos, result.Scale.Combo)
				if err != nil {
					continue
				}
				selected := orchestrator.SelectPhases(allPhases, combo)
				if len(selected) == 0 {
					continue
				}
				s.mu.RLock()
				runID := s.run.RunID
				s.mu.RUnlock()
				if runID == "" {
					continue
				}
				s.mu.RLock()
				exp := s.run.ExpiresAt
				s.mu.RUnlock()
				s.publishPhases(runID, selected, p.TargetURL, result.Scale.Workers, result.Scale.Streams, result.Scale.BatchSize, p.ProxyFile, time.Now(), exp)

				s.learnMu.Lock()
				s.learnState = result
				s.learnActive = true
				s.learnMu.Unlock()

				s.mu.Lock()
				s.run.Combo = result.Scale.Combo
				s.mu.Unlock()

				slog.Info("learn-and-attack",
					"round", result.Round,
					"workers", result.Scale.Workers,
					"combo", result.Scale.Combo,
					"degradation", result.Degradation,
					"top_vector", result.TopVector,
					"decision", result.Decision,
				)
			}
		}
	}()
}

func (s *Server) controlPlaneReady(w http.ResponseWriter) bool {
	if s.controlCtx != nil && s.controlCtx.Err() != nil {
		http.Error(w, "control plane unavailable", http.StatusServiceUnavailable)
		return false
	}
	return true
}

func (s *Server) armRunDeadline(expires time.Time, runID string) {
	go func(exp time.Time, rid string) {
		timer := time.NewTimer(time.Until(exp))
		defer timer.Stop()
		<-timer.C
		s.mu.Lock()
		if s.run.Active && s.run.RunID == rid {
			s.run.Active = false
			s.run.PhasesConfirmed = 0
		}
		s.mu.Unlock()
		s.persistRunState()
		_ = s.bus.PublishStop(context.Background(), redisbus.StopEvent{
			RunID: rid, Reason: "max_duration",
		})
	}(expires, runID)
}

func (s *Server) resumeActiveRunIfNeeded(ctx context.Context) {
	time.Sleep(500 * time.Millisecond)
	s.mu.RLock()
	run := s.run
	scale := s.runScale
	l7Mode := s.runL7Mode
	confirmed := make(map[string]struct{}, len(s.confirmedPhases))
	for id := range s.confirmedPhases {
		confirmed[id] = struct{}{}
	}
	for _, id := range s.lastPhases {
		confirmed[id] = struct{}{}
	}
	s.mu.RUnlock()
	s.learnMu.RLock()
	learnOn := s.learnActive
	s.learnMu.RUnlock()

	if !run.Active || run.RunID == "" {
		return
	}
	if time.Now().After(run.ExpiresAt) {
		_ = s.bus.PublishStop(context.Background(), redisbus.StopEvent{RunID: run.RunID, Reason: "expired_on_resume"})
		return
	}

	phases, err := orchestrator.LoadPhases(s.PhasesPath)
	if err != nil {
		return
	}
	combos, err := orchestrator.LoadCombos(s.CombosPath)
	if err != nil {
		return
	}
	combo, err := orchestrator.FindCombo(combos, run.Combo)
	if err != nil {
		return
	}
	selected := orchestrator.SelectPhases(phases, combo)
	if len(selected) == 0 {
		return
	}

	slog.Info("dashboard resuming active run after restart", "run_id", run.RunID, "combo", run.Combo)
	for _, ph := range selected {
		if _, ok := confirmed[ph.ID]; ok {
			continue
		}
		publishAt := run.StartedAt.Add(time.Duration(ph.AtSec) * time.Second)
		ev := orchestrator.BuildPhaseEvent(ph, run.RunID, run.Target, scale.Workers, scale.Streams, scale.BatchSize, run.StartedAt, run.ExpiresAt, l7Mode, scale.ProxyFile)
		ev.At = publishAt
		delay := time.Until(publishAt)
		if delay < 0 {
			delay = 0
		}
		go func(ev redisbus.PhaseEvent, d time.Duration) {
			if d > 0 {
				select {
				case <-time.After(d):
				case <-s.phaseCtx.Done():
					return
				case <-ctx.Done():
					return
				}
			}
			pubCtx, cancel := context.WithTimeout(s.phaseCtx, 5*time.Second)
			if err := s.bus.PublishPhase(pubCtx, ev); err != nil {
				slog.Error("dashboard resume phase publish failed", "run_id", ev.RunID, "id", ev.PhaseID, "err", err)
			} else {
				slog.Info("dashboard resumed phase", "run_id", ev.RunID, "id", ev.PhaseID, "vector", ev.Vector)
			}
			cancel()
		}(ev, delay)
	}

	s.armRunDeadline(run.ExpiresAt, run.RunID)

	if learnOn {
		p, err := labpolicy.Load(s.PolicyPath)
		if err == nil {
			learner := learnattack.NewLearner(learnattack.Scale{
				Workers: scale.Workers, Streams: scale.Streams, BatchSize: scale.BatchSize, Combo: run.Combo,
			}, comboIDs(combos), run.Combo)
			learner.SetStatePath(filepath.Join("data", ".learn-state.json"))
			learner.Restore()
			s.startLearnLoop(learner, p, combos, phases, run.ExpiresAt)
		}
	}
}

func (s *Server) publishPhases(runID string, selected []orchestrator.Phase, target string, workers, streams, batch int, proxyFile string, base, expires time.Time) {
	if runID == "" {
		return
	}
	phaseIDs := make([]string, len(selected))
	for i, ph := range selected {
		phaseIDs[i] = ph.ID
	}
	s.resetPhaseBatch(phaseIDs)
	s.persistRunState()

	s.mu.RLock()
	l7Mode := s.runL7Mode
	s.mu.RUnlock()
	for _, ph := range selected {
		ev := orchestrator.BuildPhaseEvent(ph, runID, target, workers, streams, batch, base, expires, l7Mode, proxyFile)
		delay := time.Until(ev.At)
		if delay < 0 {
			delay = 0
		}
		go func(ev redisbus.PhaseEvent, d time.Duration) {
			if d > 0 {
				select {
				case <-time.After(d):
				case <-s.phaseCtx.Done():
					return
				}
			}
			if s.controlCtx != nil && s.controlCtx.Err() != nil {
				return
			}
			pubCtx, cancel := context.WithTimeout(s.phaseCtx, 5*time.Second)
			if err := s.bus.PublishPhase(pubCtx, ev); err != nil {
				slog.Error("dashboard phase publish failed", "run_id", ev.RunID, "id", ev.PhaseID, "err", err)
			} else {
				slog.Info("dashboard phase published", "run_id", ev.RunID, "id", ev.PhaseID, "vector", ev.Vector, "workers", ev.Workers)
			}
			cancel()
		}(ev, delay)
	}
}

func (s *Server) reconDraftPath() string {
	return labpolicy.ReconDraftPath(filepath.Dir(s.PolicyPath))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) handleCombos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	combos, err := orchestrator.LoadCombos(s.CombosPath)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"combos": combos})
}

// handlePolicyAllowed adds or removes hosts from allowed_hosts in lab-policy.yaml.
func (s *Server) handlePolicyAllowed(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost, http.MethodDelete:
	default:
		http.Error(w, "method not allowed", 405)
		return
	}

	var req struct {
		Host  string   `json:"host"`
		Hosts []string `json:"hosts"`
		URL   string   `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	candidates := append([]string{}, req.Hosts...)
	if req.Host != "" {
		candidates = append(candidates, req.Host)
	}
	if req.URL != "" {
		if u, err := url.Parse(req.URL); err == nil && u.Hostname() != "" {
			candidates = append(candidates, u.Hostname())
		}
	}
	if len(candidates) == 0 {
		http.Error(w, "host, hosts, or url required", 400)
		return
	}

	p, err := labpolicy.Load(s.PolicyPath)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	var changed []string
	if r.Method == http.MethodPost {
		for _, h := range candidates {
			if p.AddAllowedHost(h) {
				changed = append(changed, labpolicy.NormalizeHost(h))
			}
		}
	} else {
		for _, h := range candidates {
			if p.RemoveAllowedHost(h) {
				changed = append(changed, labpolicy.NormalizeHost(h))
			}
		}
	}

	if len(changed) == 0 {
		writeJSON(w, map[string]any{
			"status":        "unchanged",
			"allowed_hosts": p.AllowedHosts,
		})
		return
	}

	if err := p.Save(s.PolicyPath); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	action := "added"
	if r.Method == http.MethodDelete {
		action = "removed"
	}
	promoted, promoteNote := s.tryPromoteReconDraft(p)
	writeJSON(w, map[string]any{
		"status":        action,
		"changed":       changed,
		"allowed_hosts": p.AllowedHosts,
		"draft_promoted": promoted,
		"promote_note":  promoteNote,
	})
}

// handleRecon analyzes a target URL and returns technology fingerprint + recommended attacks.
func (s *Server) handleRecon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}

	target := r.URL.Query().Get("url")
	if target == "" && r.Method == http.MethodPost {
		var body struct {
			URL string `json:"url"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		target = body.URL
	}
	if target == "" {
		// fallback to current policy target
		if p, err := labpolicy.Load(s.PolicyPath); err == nil {
			target = p.TargetURL
		}
	}
	if target == "" {
		http.Error(w, "url required", 400)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()

	profile, err := recon.Analyze(ctx, target)
	if err != nil {
		// return partial profile even on partial failure
		if profile == nil {
			profile = &recon.TargetProfile{URL: target, Notes: err.Error()}
		}
	}

	writeJSON(w, map[string]any{
		"profile": profile,
		"redis":   s.redisOK(),
	})
}

func reconTuning(profile *recon.TargetProfile, intensity string) (combo string, workers, streams, batch int, l7Mode string) {
	combo = "baseline"
	if profile.RecommendedCombo != "" {
		combo = profile.RecommendedCombo
	}
	l7Mode = profile.RecommendedL7Mode

	intensity = strings.ToLower(intensity)
	if intensity == "" {
		intensity = profile.AttackIntensity
	}
	switch intensity {
	case "extreme":
		workers, streams, batch = 64, 500, 200
	case "high":
		workers, streams, batch = 32, 250, 120
	case "medium":
		workers, streams, batch = 12, 150, 80
	default:
		workers, streams, batch = 6, 80, 50
	}
	return combo, workers, streams, batch, l7Mode
}

// handleReconApply stores candidate intel in a recon draft; policy is updated only for allowlisted targets.
func (s *Server) handleReconApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}

	var req struct {
		URL       string `json:"url"`
		Intensity string `json:"intensity"` // low|medium|high|extreme
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", 400)
		return
	}

	target := req.URL
	if target == "" {
		http.Error(w, "url required", 400)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	profile, err := recon.Analyze(ctx, target)
	if err != nil && profile == nil {
		http.Error(w, err.Error(), 500)
		return
	}

	finalURL := profile.FinalURL
	if finalURL == "" {
		finalURL = target
	}

	policy, _ := labpolicy.Load(s.PolicyPath)
	if policy == nil {
		policy = &labpolicy.Policy{}
	}
	scratch := &labpolicy.Policy{TargetURL: finalURL}
	host, hostErr := scratch.TargetHost()
	targetAllowed := hostErr == nil && policy.IsHostAllowed(host)

	intensity := strings.ToLower(req.Intensity)
	if intensity == "" {
		intensity = profile.AttackIntensity
	}
	combo, workers, streams, batch, l7Mode := reconTuning(profile, intensity)

	draft := &labpolicy.ReconDraft{
		TargetURL:     finalURL,
		Host:          host,
		TargetAllowed: targetAllowed,
		Combo:         combo,
		L7Mode:        l7Mode,
		Workers:       workers,
		Streams:       streams,
		BatchSize:     batch,
		Intensity:     intensity,
		Profile:       profile,
	}
	if err := labpolicy.SaveReconDraft(s.reconDraftPath(), draft); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	status := "draft"
	draftOnly := !targetAllowed
	var saved *labpolicy.Policy
	if targetAllowed {
		saved, err = labpolicy.Load(s.PolicyPath)
		if err != nil {
			saved = &labpolicy.Policy{}
		}
		saved.TargetURL = finalURL
		saved.Combo = combo
		saved.Workers = workers
		saved.Streams = streams
		saved.BatchSize = batch
		if l7Mode != "" {
			saved.L7Mode = l7Mode
		}
		if err := saved.Save(s.PolicyPath); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		status = "applied"
		draft.PersistedPolicy = true
		_ = labpolicy.SaveReconDraft(s.reconDraftPath(), draft)
	}

	writeJSON(w, map[string]any{
		"status":          status,
		"draft_only":      draftOnly,
		"profile":         profile,
		"draft":           draft,
		"policy":          saved,
		"host":            host,
		"target_allowed":  targetAllowed,
		"needs_allowlist": !targetAllowed,
		"scale_advice":    "For extreme load run: docker compose --profile vectors up --scale l7-abuser=10 --scale h2-thrasher=5 --scale quic-burner=3",
		"intensity":       intensity,
	})
}

func (s *Server) handleReconDraft(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	draft, err := labpolicy.LoadReconDraft(s.reconDraftPath())
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "no recon draft", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, draft)
}

func (s *Server) handleReconPromote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	policy, err := labpolicy.Load(s.PolicyPath)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	promoted, note, saved, draft, err := s.promoteReconDraft(policy)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "no recon draft to promote", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), 400)
		return
	}
	if !promoted {
		http.Error(w, note, http.StatusConflict)
		return
	}
	writeJSON(w, map[string]any{
		"status":  "promoted",
		"note":    note,
		"policy":  saved,
		"draft":   draft,
	})
}

func (s *Server) tryPromoteReconDraft(policy *labpolicy.Policy) (bool, string) {
	promoted, note, _, _, err := s.promoteReconDraft(policy)
	if err != nil {
		return false, ""
	}
	return promoted, note
}

func (s *Server) promoteReconDraft(policy *labpolicy.Policy) (bool, string, *labpolicy.Policy, *labpolicy.ReconDraft, error) {
	draft, err := labpolicy.LoadReconDraft(s.reconDraftPath())
	if err != nil {
		return false, "", policy, nil, err
	}
	if draft.PersistedPolicy {
		return false, "recon draft already promoted to policy", policy, draft, nil
	}
	if !labpolicy.DraftHostAllowlisted(policy, draft) {
		host := draft.Host
		if host == "" {
			host = draft.TargetURL
		}
		return false, "target host not allowlisted: " + host, policy, draft, nil
	}
	saved, err := labpolicy.Load(s.PolicyPath)
	if err != nil {
		saved = policy
	}
	labpolicy.PromoteDraftToPolicy(saved, draft)
	if err := saved.Save(s.PolicyPath); err != nil {
		return false, "", saved, draft, err
	}
	draft.PersistedPolicy = true
	draft.TargetAllowed = true
	_ = labpolicy.SaveReconDraft(s.reconDraftPath(), draft)
	return true, "recon draft promoted to runnable policy", saved, draft, nil
}
