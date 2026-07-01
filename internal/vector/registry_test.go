package vector_test

import (
	"testing"

	"github.com/kjsst/sh-mvdos/internal/vector"
)

func TestResolveAliases(t *testing.T) {
	reg, err := vector.LoadRegistry("../../configs/vectors.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		in   string
		want vector.ID
	}{
		{"rapidreset", vector.IDH2RapidReset},
		{"wsflood", vector.IDWSFlood},
		{"post", vector.IDHTTPPost},
		{"api", vector.IDAPIFlood},
		{"rudy", vector.IDRUDY},
	}
	for _, tc := range cases {
		spec, err := reg.Resolve(tc.in)
		if err != nil {
			t.Fatalf("resolve %q: %v", tc.in, err)
		}
		if spec.ID != tc.want {
			t.Fatalf("resolve %q = %s, want %s", tc.in, spec.ID, tc.want)
		}
	}
}

func TestRUDYStripsStreamsBatch(t *testing.T) {
	reg, err := vector.LoadRegistry("../../configs/vectors.yaml")
	if err != nil {
		t.Fatal(err)
	}
	spec, _ := reg.Resolve("rudy")
	out := spec.ApplyCapabilities(vector.Scale{Workers: 500, Streams: 100, Batch: 50})
	if out.Streams != 0 || out.Batch != 0 {
		t.Fatalf("rudy should zero streams/batch, got %+v", out)
	}
	if out.Workers != 500 {
		t.Fatalf("workers preserved: %d", out.Workers)
	}
}