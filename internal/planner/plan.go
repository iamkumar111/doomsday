package planner

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kjsst/sh-mvdos/internal/vector"
	"gopkg.in/yaml.v3"
)

const DefaultComboPlansPath = "configs/combo-plans.yaml"

type PhaseParams struct {
	PathProfile string   `yaml:"path_profile" json:"path_profile,omitempty"`
	Paths       []string `yaml:"paths" json:"paths,omitempty"`
	WSPath      string   `yaml:"ws_path" json:"ws_path,omitempty"`
}

type PlanPhase struct {
	Vector        string      `yaml:"vector" json:"vector"`
	StartAfterSec int         `yaml:"start_after_sec" json:"start_after_sec"`
	DurationSec   int         `yaml:"duration_sec" json:"duration_sec,omitempty"`
	Weight        float64     `yaml:"weight" json:"weight"`
	Params        PhaseParams `yaml:"params" json:"params,omitempty"`
}

type ComboPlan struct {
	ID     string      `yaml:"id" json:"id"`
	Label  string      `yaml:"label" json:"label"`
	Phases []PlanPhase `yaml:"phases" json:"phases"`
}

type planFile struct {
	Plans []ComboPlan `yaml:"plans"`
}

type RunScale struct {
	Workers        int
	Streams        int
	Batch          int
	MaxDurationSec int
	ProxyFile      string
	WSPath         string
}

// PlannedPhase is a resolved phase ready for preview or publish.
type PlannedPhase struct {
	PhaseID       string         `json:"phase_id"`
	Vector        string         `json:"vector"`
	Label         string         `json:"label"`
	Worker        string         `json:"worker"`
	RedisVector   string         `json:"redis_vector"`
	Protocol      string         `json:"protocol"`
	StartAfterSec int            `json:"start_after_sec"`
	DurationSec   int            `json:"duration_sec"`
	Weight        float64        `json:"weight"`
	Scale         vector.Scale   `json:"scale"`
	Capabilities  vector.Capability `json:"capabilities"`
	Paths         []string       `json:"paths,omitempty"`
	Warnings      []string       `json:"warnings,omitempty"`
}

// RunPlan is the full preview of an attack run.
type RunPlan struct {
	PlanID          string         `json:"plan_id"`
	Label           string         `json:"label"`
	Target          string         `json:"target"`
	MaxDurationSec  int            `json:"max_duration_sec"`
	Phases          []PlannedPhase `json:"phases"`
	RequiredWorkers []string       `json:"required_workers"`
	Warnings        []string       `json:"warnings"`
}

func LoadPlans(path string) ([]ComboPlan, error) {
	if path == "" {
		path = DefaultComboPlansPath
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f planFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	return f.Plans, nil
}

func FindPlan(plans []ComboPlan, id string) (ComboPlan, error) {
	for _, p := range plans {
		if p.ID == id {
			return p, nil
		}
	}
	return ComboPlan{}, fmt.Errorf("planner: plan %q not found", id)
}

// BuildRunPlan resolves combo plan with weighted scale split.
func BuildRunPlan(reg *vector.Registry, paths *vector.PathProfiles, plan ComboPlan, target string, scale RunScale) (RunPlan, error) {
	if reg == nil {
		return RunPlan{}, fmt.Errorf("planner: nil registry")
	}
	if len(plan.Phases) == 0 {
		return RunPlan{}, fmt.Errorf("planner: empty plan")
	}
	var totalWeight float64
	for _, ph := range plan.Phases {
		w := ph.Weight
		if w <= 0 {
			w = 1
		}
		totalWeight += w
	}
	if totalWeight <= 0 {
		totalWeight = float64(len(plan.Phases))
	}

	out := RunPlan{
		PlanID:         plan.ID,
		Label:          plan.Label,
		Target:         target,
		MaxDurationSec: scale.MaxDurationSec,
	}
	workerSet := map[string]struct{}{}

	for i, ph := range plan.Phases {
		spec, err := reg.Resolve(ph.Vector)
		if err != nil {
			return RunPlan{}, err
		}
		w := ph.Weight
		if w <= 0 {
			w = 1
		}
		fraction := w / totalWeight
		phaseScale := vector.Scale{
			Workers:   max(1, int(float64(scale.Workers)*fraction)),
			Streams:   max(1, int(float64(scale.Streams)*fraction)),
			Batch:     max(1, int(float64(scale.Batch)*fraction)),
			ProxyFile: scale.ProxyFile,
			WSPath:    firstNonEmpty(ph.Params.WSPath, scale.WSPath),
		}
		phaseScale = spec.FillDefaults(spec.ApplyCapabilities(phaseScale))

		resolvedPaths := paths.ResolveForVector(spec, ph.Params.PathProfile, ph.Params.Paths)
		if spec.Capabilities.Paths && len(resolvedPaths) == 0 && spec.ID == vector.IDWSFlood {
			out.Warnings = append(out.Warnings, fmt.Sprintf("ws-flood phase %d: no explicit path — set ws_path or path_profile", i))
		}
		if len(resolvedPaths) > 0 {
			phaseScale.Paths = resolvedPaths
		}

		dur := ph.DurationSec
		if dur <= 0 {
			dur = scale.MaxDurationSec
		}

		pp := PlannedPhase{
			PhaseID:       fmt.Sprintf("%s-%d", spec.ID, i),
			Vector:        string(spec.ID),
			Label:         spec.Label,
			Worker:        spec.Worker,
			RedisVector:   spec.RedisVector,
			Protocol:      spec.Protocol,
			StartAfterSec: ph.StartAfterSec,
			DurationSec:   dur,
			Weight:        w,
			Scale:         phaseScale,
			Capabilities:  spec.Capabilities,
			Paths:         resolvedPaths,
		}
		if spec.ID == vector.IDRUDY {
			pp.Warnings = append(pp.Warnings, "rudy uses workers only — one slow POST per worker")
		}
		out.Phases = append(out.Phases, pp)
		workerSet[spec.Worker] = struct{}{}
	}
	for w := range workerSet {
		out.RequiredWorkers = append(out.RequiredWorkers, w)
	}
	return out, nil
}

// PlanFromAttackMode builds a single-vector plan when dashboard attack mode overrides combo.
func PlanFromAttackMode(reg *vector.Registry, paths *vector.PathProfiles, mode, target string, scale RunScale) (RunPlan, error) {
	spec, err := reg.Resolve(mode)
	if err != nil {
		return RunPlan{}, err
	}
	plan := ComboPlan{
		ID:    string(spec.ID) + "-direct",
		Label: spec.Label + " (direct)",
		Phases: []PlanPhase{{
			Vector:        string(spec.ID),
			StartAfterSec: 0,
			Weight:        1,
			Params: PhaseParams{
				WSPath: scale.WSPath,
			},
		}},
	}
	if spec.Capabilities.Paths && paths != nil {
		for _, key := range spec.PathProfiles {
			plan.Phases[0].Params.PathProfile = key
			break
		}
	}
	return BuildRunPlan(reg, paths, plan, target, scale)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ExpiresAt returns run deadline from start.
// RedisHeartbeatVectors maps worker binary names to registry heartbeat keys.
func RedisHeartbeatVectors(workerBinaries []string) []string {
	mapping := map[string]string{
		"h2-thrasher": "h2-rapid-reset",
	}
	var out []string
	seen := map[string]struct{}{}
	for _, w := range workerBinaries {
		v := w
		if m, ok := mapping[w]; ok {
			v = m
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func ExpiresAt(start time.Time, maxSec int) time.Time {
	if maxSec <= 0 {
		maxSec = 300
	}
	return start.Add(time.Duration(maxSec) * time.Second)
}