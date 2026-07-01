package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kjsst/sh-mvdos/internal/labpolicy"
	"github.com/kjsst/sh-mvdos/internal/orchestrator"
	"github.com/kjsst/sh-mvdos/internal/planner"
	"github.com/kjsst/sh-mvdos/internal/vector"
)

type planDraft struct {
	Combo          string `json:"combo"`
	L7Mode         string `json:"l7_mode"`
	TargetURL      string `json:"target_url"`
	Workers        int    `json:"workers"`
	Streams        int    `json:"streams"`
	BatchSize      int    `json:"batch_size"`
	MaxDurationSec int    `json:"max_duration_sec"`
	ProxyFile      string `json:"proxy_file"`
	WSPath         string `json:"ws_path"`
}

func (s *Server) loadPlannerDeps() (*vector.Registry, *vector.PathProfiles, error) {
	reg, err := vector.LoadRegistry(s.VectorsPath)
	if err != nil {
		return nil, nil, err
	}
	paths, err := vector.LoadPathProfiles(s.PathProfilesPath)
	if err != nil {
		return nil, nil, err
	}
	return reg, paths, nil
}

func (s *Server) resolveRunPlan(p *labpolicy.Policy, comboID string, workers, streams, batch, maxDur int) (planner.RunPlan, bool, error) {
	reg, paths, err := s.loadPlannerDeps()
	if err != nil {
		return planner.RunPlan{}, false, fmt.Errorf("planner deps: %w (vectors=%s profiles=%s)", err, s.VectorsPath, s.PathProfilesPath)
	}
	scale := planner.RunScale{
		Workers:        workers,
		Streams:        streams,
		Batch:          batch,
		MaxDurationSec: maxDur,
		ProxyFile:      p.ProxyFile,
		WSPath:         p.WSPath,
	}
	if mode := p.EffectiveAttackMode(); mode != "" {
		if _, err := reg.Resolve(mode); err == nil {
			rp, err := planner.PlanFromAttackMode(reg, paths, mode, p.TargetURL, scale)
			return rp, true, err
		}
	}
	plans, err := planner.LoadPlans(s.ComboPlansPath)
	if err != nil {
		return planner.RunPlan{}, false, fmt.Errorf("combo plans: %w (path=%s)", err, s.ComboPlansPath)
	}
	plan, err := planner.FindPlan(plans, comboID)
	if err != nil {
		return planner.RunPlan{}, false, nil
	}
	rp, err := planner.BuildRunPlan(reg, paths, plan, p.TargetURL, scale)
	return rp, true, err
}

func (s *Server) handleVectors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	reg, err := vector.LoadRegistry(s.VectorsPath)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"vectors": reg.List()})
}

func (s *Server) handlePlanPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	p, err := labpolicy.Load(s.PolicyPath)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	var draft planDraft
	_ = json.NewDecoder(r.Body).Decode(&draft)
	applyPlanDraft(p, draft)
	rp, ok, err := s.resolveRunPlan(p, p.Combo, p.Workers, p.Streams, p.BatchSize, p.MaxDurationSec)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if !ok {
		if legacy := s.legacyPlanPreview(p); legacy != nil {
			writeJSON(w, legacy)
			return
		}
		http.Error(w, fmt.Sprintf("no combo plan for %q — add to configs/combo-plans.yaml or set attack mode", p.Combo), 404)
		return
	}
	writeJSON(w, rp)
}

func applyPlanDraft(p *labpolicy.Policy, draft planDraft) {
	if draft.Combo != "" {
		p.Combo = draft.Combo
	}
	p.L7Mode = draft.L7Mode
	if draft.TargetURL != "" {
		p.TargetURL = draft.TargetURL
	}
	if draft.Workers > 0 {
		p.Workers = draft.Workers
	}
	if draft.Streams > 0 {
		p.Streams = draft.Streams
	}
	if draft.BatchSize > 0 {
		p.BatchSize = draft.BatchSize
	}
	if draft.MaxDurationSec > 0 {
		p.MaxDurationSec = draft.MaxDurationSec
	}
	p.ProxyFile = draft.ProxyFile
	p.WSPath = draft.WSPath
}

func (s *Server) legacyPlanPreview(p *labpolicy.Policy) map[string]any {
	combos, err := orchestrator.LoadCombos(s.CombosPath)
	if err != nil {
		return nil
	}
	phases, err := orchestrator.LoadPhases(s.PhasesPath)
	if err != nil {
		return nil
	}
	combo, err := orchestrator.FindCombo(combos, p.Combo)
	if err != nil {
		return nil
	}
	selected := orchestrator.SelectAttackPhases(phases, combo, p.EffectiveAttackMode())
	if len(selected) == 0 {
		return nil
	}
	lines := make([]map[string]any, 0, len(selected))
	workers := map[string]struct{}{}
	for _, ph := range selected {
		lines = append(lines, map[string]any{
			"phase_id":        ph.ID,
			"vector":          ph.Vector,
			"label":           ph.ID,
			"start_after_sec": ph.AtSec,
			"scale": map[string]int{
				"workers": p.Workers,
				"streams": p.Streams,
				"batch":   p.BatchSize,
			},
		})
		workers[ph.Vector] = struct{}{}
	}
	req := make([]string, 0, len(workers))
	for w := range workers {
		req = append(req, w)
	}
	return map[string]any{
		"plan_id":          p.Combo,
		"label":            combo.Label + " (legacy phases.yaml)",
		"target":           p.TargetURL,
		"max_duration_sec": p.MaxDurationSec,
		"phases":           lines,
		"required_workers": req,
		"warnings":         []string{"legacy plan — no weighted scale split; migrate to combo-plans.yaml"},
		"legacy":           true,
	}
}

func (s *Server) handlePlatform(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	plannerOK := true
	var plannerErr string
	if _, _, err := s.loadPlannerDeps(); err != nil {
		plannerOK = false
		plannerErr = err.Error()
	}
	if _, err := planner.LoadPlans(s.ComboPlansPath); err != nil {
		plannerOK = false
		if plannerErr != "" {
			plannerErr += "; "
		}
		plannerErr += err.Error()
	}
	writeJSON(w, map[string]any{
		"version":            "vector-platform-v2",
		"vectors_path":       s.VectorsPath,
		"combo_plans_path":   s.ComboPlansPath,
		"path_profiles_path": s.PathProfilesPath,
		"planner_ready":      plannerOK,
		"planner_error":      plannerErr,
	})
}