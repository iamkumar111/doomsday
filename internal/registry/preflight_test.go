package registry

import "testing"

func TestPreflightErrorFormatsMissing(t *testing.T) {
	msg := PreflightError(PreflightResult{Missing: []string{"l7-abuser", "ws-flood"}})
	if msg == "" || msg == "preflight failed" {
		t.Fatalf("unexpected message: %q", msg)
	}
}

func TestLookupCapacityDefaults(t *testing.T) {
	c := LookupCapacity("l7-abuser")
	if c.MaxWorkers != 2048 {
		t.Fatalf("l7-abuser max workers=%d", c.MaxWorkers)
	}
	unknown := LookupCapacity("unknown-vector")
	if unknown.MaxWorkers <= 0 {
		t.Fatal("expected positive default capacity")
	}
}