package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kjsst/sh-mvdos/internal/labpolicy"
	"github.com/kjsst/sh-mvdos/internal/planner"
	"github.com/kjsst/sh-mvdos/internal/vector"
)

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
		return planner.RunPlan{}, false, nil
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
		return planner.RunPlan{}, false, nil
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
	var draft struct {
		Combo string `json:"combo"`
	}
	_ = json.NewDecoder(r.Body).Decode(&draft)
	if draft.Combo != "" {
		p.Combo = draft.Combo
	}
	rp, ok, err := s.resolveRunPlan(p, p.Combo, p.Workers, p.Streams, p.BatchSize, p.MaxDurationSec)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if !ok {
		http.Error(w, fmt.Sprintf("no combo plan for %q — add to configs/combo-plans.yaml or set attack mode", p.Combo), 404)
		return
	}
	writeJSON(w, rp)
}