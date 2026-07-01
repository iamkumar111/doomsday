package proxy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePoolDirect(t *testing.T) {
	clients, err := ResolvePool("", 2048)
	if err != nil {
		t.Fatal(err)
	}
	if len(clients) != MaxDirectPool {
		t.Fatalf("expected %d direct clients, got %d", MaxDirectPool, len(clients))
	}
	clientsSmall, err := ResolvePool("", 32)
	if err != nil {
		t.Fatal(err)
	}
	if len(clientsSmall) != 4 {
		t.Fatalf("expected 4 direct clients for 32 workers, got %d", len(clientsSmall))
	}
}

func TestLoadProxies(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "proxies.txt")
	if err := os.WriteFile(path, []byte("http://127.0.0.1:8080\n# comment\n\nhttp://127.0.0.1:8081\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lines, err := LoadProxies(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 proxies, got %d", len(lines))
	}
}