package labpolicy_test

import (
	"testing"

	"github.com/kjsst/sh-mvdos/internal/labpolicy"
)

func TestAddAllowedHost(t *testing.T) {
	p := &labpolicy.Policy{AllowedHosts: []string{"127.0.0.1"}}
	if !p.AddAllowedHost("example.com") {
		t.Fatal("expected add")
	}
	if p.AddAllowedHost("https://Example.com/path") {
		t.Fatal("expected duplicate skip")
	}
	if !p.IsHostAllowed("example.com") {
		t.Fatal("expected host allowed")
	}
}

func TestRemoveAllowedHost(t *testing.T) {
	p := &labpolicy.Policy{AllowedHosts: []string{"127.0.0.1", "victim"}}
	if !p.RemoveAllowedHost("victim") {
		t.Fatal("expected remove")
	}
	if p.IsHostAllowed("victim") {
		t.Fatal("expected host removed")
	}
}