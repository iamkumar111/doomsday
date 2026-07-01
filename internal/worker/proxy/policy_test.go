package proxy

import "testing"

func TestEffectivePath(t *testing.T) {
	if got := EffectivePath("", "  /data/p.txt ", ""); got != "/data/p.txt" {
		t.Fatalf("got %q", got)
	}
	if got := EffectivePath("", "", ""); got != "" {
		t.Fatalf("expected empty")
	}
}