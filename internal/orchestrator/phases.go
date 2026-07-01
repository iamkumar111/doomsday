package orchestrator

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Phase struct {
	ID      string            `yaml:"id"`
	Vector  string            `yaml:"vector"`
	AtSec   int               `yaml:"at_sec"`
	Workers int               `yaml:"workers"`
	Streams int               `yaml:"streams"`
	Batch   int               `yaml:"batch_size"`
	Params  map[string]string `yaml:"params"`
}

type PhaseFile struct {
	Phases []Phase `yaml:"phases"`
}

type Combo struct {
	ID      string   `yaml:"id"`
	Label   string   `yaml:"label"`
	Phases  []string `yaml:"phases"`
	Vectors []string `yaml:"vectors"`
}

type ComboFile struct {
	Combos []Combo `yaml:"combos"`
}

func LoadPhases(path string) ([]Phase, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f PhaseFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	return f.Phases, nil
}

func LoadCombos(path string) ([]Combo, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f ComboFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	return f.Combos, nil
}

func SelectPhases(all []Phase, combo Combo) []Phase {
	if len(combo.Phases) > 0 {
		byID := make(map[string]Phase, len(all))
		for _, p := range all {
			byID[p.ID] = p
		}
		out := make([]Phase, 0, len(combo.Phases))
		for _, id := range combo.Phases {
			if p, ok := byID[id]; ok {
				out = append(out, p)
			}
		}
		return out
	}
	// Fallback: if combo specifies vectors instead of phase ids (legacy/alt usage)
	if len(combo.Vectors) > 0 {
		vectorSet := make(map[string]bool, len(combo.Vectors))
		for _, v := range combo.Vectors {
			vectorSet[v] = true
		}
		var out []Phase
		for _, p := range all {
			if vectorSet[p.Vector] {
				out = append(out, p)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return all
}

func ScheduleStart(base time.Time, phases []Phase) map[string]time.Time {
	out := make(map[string]time.Time, len(phases))
	for _, p := range phases {
		out[p.ID] = base.Add(time.Duration(p.AtSec) * time.Second)
	}
	return out
}

func FindCombo(combos []Combo, id string) (Combo, error) {
	for _, c := range combos {
		if c.ID == id {
			return c, nil
		}
	}
	return Combo{}, fmt.Errorf("combo %q not found", id)
}
