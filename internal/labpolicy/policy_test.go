package labpolicy_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/kjsst/sh-mvdos/internal/guard"
	"github.com/kjsst/sh-mvdos/internal/labpolicy"
)

func TestSaveAtomicConcurrent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.yaml")
	p := &labpolicy.Policy{
		EthicsAck: guard.EthicsAckValue,
		LabMode:   "isolated",
		TargetURL: "http://127.0.0.1:8443",
		Workers:   4,
	}
	if err := p.Save(path); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			cp, _ := labpolicy.Load(path)
			cp.Workers = n + 1
			_ = cp.Save(path)
		}(i)
	}
	wg.Wait()

	loaded, err := labpolicy.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Workers < 1 || loaded.Workers > 9 {
		t.Fatalf("unexpected workers after concurrent save: %d", loaded.Workers)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 {
		t.Fatal("policy file empty after concurrent saves")
	}
}