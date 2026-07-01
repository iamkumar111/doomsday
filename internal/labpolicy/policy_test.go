package labpolicy_test

import (
	"os"
	"path/filepath"
	"strings"
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

func TestValidateRuntimeBounds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.yaml")
	p := &labpolicy.Policy{
		EthicsAck:          guard.EthicsAckValue,
		LabMode:            "isolated",
		TargetURL:          "http://127.0.0.1:8443",
		Workers:            labpolicy.MaxWorkers + 1,
		Streams:            100,
		BatchSize:          50,
		MaxDurationSec:     300,
		WatchdogCPUPercent: 10,
	}
	if err := p.ValidateRuntimeBounds(path); err == nil || !strings.Contains(err.Error(), "workers") {
		t.Fatalf("expected workers bound error, got %v", err)
	}

	p.Workers = 32
	p.Streams = labpolicy.MaxStreams + 1
	if err := p.ValidateRuntimeBounds(path); err == nil || !strings.Contains(err.Error(), "streams") {
		t.Fatalf("expected streams bound error, got %v", err)
	}

	p.Streams = 200
	p.BatchSize = labpolicy.MaxBatchSize + 1
	if err := p.ValidateRuntimeBounds(path); err == nil || !strings.Contains(err.Error(), "batch_size") {
		t.Fatalf("expected batch_size bound error, got %v", err)
	}
}

func TestValidateProxyFile(t *testing.T) {
	p := &labpolicy.Policy{}
	path := filepath.Join(t.TempDir(), "policy.yaml")

	p.ProxyFile = "../outside.txt"
	if err := p.ValidateProxyFile(path); err == nil {
		t.Fatal("expected path traversal rejection")
	}

	p.ProxyFile = "/etc/passwd"
	if err := p.ValidateProxyFile(path); err == nil {
		t.Fatal("expected absolute path rejection")
	}

	p.ProxyFile = "/data/proxies.txt"
	if err := p.ValidateProxyFile(path); err != nil {
		t.Fatalf("expected /data proxy path allowed, got %v", err)
	}

	p.ProxyFile = "proxies.txt"
	if err := p.ValidateProxyFile(path); err != nil {
		t.Fatalf("expected relative proxy path allowed, got %v", err)
	}
}
