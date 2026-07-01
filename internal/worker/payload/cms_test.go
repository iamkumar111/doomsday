package payload

import (
	"strings"
	"testing"
)

func TestMagentoCatalogSearchPath(t *testing.T) {
	p := MagentoCatalogSearchPath()
	if !strings.Contains(p, "/catalogsearch/") {
		t.Fatalf("expected catalogsearch path, got %q", p)
	}
	if !strings.Contains(p, "q=") && !strings.Contains(p, "name=") {
		t.Fatalf("expected search query params, got %q", p)
	}
}

func TestHeavySearchQueryNonEmpty(t *testing.T) {
	if q := HeavySearchQuery(); len(q) < 10 {
		t.Fatalf("query too short: %q", q)
	}
}