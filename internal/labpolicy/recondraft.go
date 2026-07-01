package labpolicy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/kjsst/sh-mvdos/internal/recon"
)

const DefaultReconDraftPath = "data/.recon-draft.json"

// ReconDraft holds candidate intel separate from approved runnable policy.
type ReconDraft struct {
	TargetURL       string                `json:"target_url"`
	Host            string                `json:"host,omitempty"`
	TargetAllowed   bool                  `json:"target_allowed"`
	Combo           string                `json:"combo,omitempty"`
	L7Mode          string                `json:"l7_mode,omitempty"`
	Workers         int                   `json:"workers,omitempty"`
	Streams         int                   `json:"streams,omitempty"`
	BatchSize       int                   `json:"batch_size,omitempty"`
	Intensity       string                `json:"intensity,omitempty"`
	Profile         *recon.TargetProfile  `json:"profile,omitempty"`
	AppliedAt       time.Time             `json:"applied_at"`
	PersistedPolicy bool                  `json:"persisted_policy"`
}

func SaveReconDraft(path string, draft *ReconDraft) error {
	if path == "" {
		path = DefaultReconDraftPath
	}
	if draft.AppliedAt.IsZero() {
		draft.AppliedAt = time.Now().UTC()
	}
	raw, err := json.MarshalIndent(draft, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dirOf(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func LoadReconDraft(path string) (*ReconDraft, error) {
	if path == "" {
		path = DefaultReconDraftPath
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var d ReconDraft
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func ReconDraftPath(baseDir string) string {
	if baseDir == "" {
		return DefaultReconDraftPath
	}
	return filepath.Join(baseDir, ".recon-draft.json")
}