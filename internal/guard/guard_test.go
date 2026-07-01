package guard_test

import (
	"testing"

	"github.com/kjsst/sh-mvdos/internal/guard"
	"github.com/kjsst/sh-mvdos/internal/labpolicy"
)

func TestMustAuthorizeBlocksDisallowedHost(t *testing.T) {
	path := t.TempDir() + "/policy.yaml"
	p := &labpolicy.Policy{
		LabMode:      "isolated",
		EthicsAck:    guard.EthicsAckValue,
		TargetURL:    "https://example.com",
		AllowedHosts: []string{"127.0.0.1"},
	}
	if err := p.Save(path); err != nil {
		t.Fatal(err)
	}
	_, err := guard.MustAuthorize(guard.Config{PolicyPath: path, Vector: "test"})
	if err == nil {
		t.Fatal("expected refusal for public target")
	}
}

func TestMustAuthorizeAllowsLoopback(t *testing.T) {
	path := t.TempDir() + "/policy.yaml"
	p := &labpolicy.Policy{
		LabMode:        "isolated",
		EthicsAck:      guard.EthicsAckValue,
		TargetURL:      "http://127.0.0.1:8443",
		AllowedHosts:   []string{"127.0.0.1", "localhost"},
		MaxDurationSec: 5,
	}
	if err := p.Save(path); err != nil {
		t.Fatal(err)
	}
	ctx, err := guard.MustAuthorize(guard.Config{PolicyPath: path, Vector: "test"})
	if err != nil {
		t.Fatal(err)
	}
	ctx.Done()
}
