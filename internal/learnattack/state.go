package learnattack

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const DefaultStatePath = "data/.learn-state.json"

// PersistedState survives dashboard restarts.
type PersistedState struct {
	TargetURL  string               `json:"target_url"`
	Baseline   ProbeSample          `json:"baseline"`
	Model      OnlineModel          `json:"model"`
	ComboStats map[string]ComboStat `json:"combo_stats"`
	Best       Scale                `json:"best"`
	BestScore  float64              `json:"best_score"`
	Round      int                  `json:"round"`
	LastCombo  string               `json:"last_combo"`
	History    []RoundResult        `json:"history,omitempty"`
}

type ComboStat struct {
	Pulls       int     `json:"pulls"`
	TotalReward float64 `json:"total_reward"`
}

func LoadState(path string) (*PersistedState, error) {
	if path == "" {
		path = DefaultStatePath
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var st PersistedState
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

func SaveState(path string, st *PersistedState) error {
	if path == "" {
		path = DefaultStatePath
	}
	if st.ComboStats == nil {
		st.ComboStats = make(map[string]ComboStat)
	}
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func (s *PersistedState) export() *PersistedState {
	if len(s.History) > 30 {
		s.History = s.History[len(s.History)-30:]
	}
	return s
}

// ClearState removes persisted learn session (fresh start after manual stop).
func ClearState(path string) error {
	if path == "" {
		path = DefaultStatePath
	}
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}