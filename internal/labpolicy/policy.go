package labpolicy

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/kjsst/sh-mvdos/internal/fsutil"
	"gopkg.in/yaml.v3"
)

var policySaveMu sync.Mutex

type Policy struct {
	LabMode            string   `yaml:"lab_mode" json:"lab_mode"`
	EthicsAck          string   `yaml:"ethics_ack" json:"ethics_ack"`
	TargetURL          string   `yaml:"target_url" json:"target_url"`
	AllowedHosts       []string `yaml:"allowed_hosts" json:"allowed_hosts"`
	ConductorMode      string   `yaml:"conductor_mode" json:"conductor_mode"`
	Combo              string   `yaml:"combo" json:"combo"`
	L7Mode             string   `yaml:"l7_mode" json:"l7_mode,omitempty"`
	WatchdogCPUPercent int      `yaml:"watchdog_cpu_percent" json:"watchdog_cpu_percent"`
	MaxDurationSec     int      `yaml:"max_duration_sec" json:"max_duration_sec"`
	Workers            int      `yaml:"workers" json:"workers"`
	Streams            int      `yaml:"streams" json:"streams"`
	BatchSize          int      `yaml:"batch_size" json:"batch_size"`
	ProxyFile          string   `yaml:"proxy_file" json:"proxy_file,omitempty"`
}

const DefaultPath = "data/lab-policy.yaml"

func Load(path string) (*Policy, error) {
	if path == "" {
		path = DefaultPath
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("labpolicy: read %s: %w", path, err)
	}
	var p Policy
	if err := yaml.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("labpolicy: parse: %w", err)
	}
	p.applyDefaults()
	return &p, nil
}

func (p *Policy) Save(path string) error {
	if path == "" {
		path = DefaultPath
	}
	policySaveMu.Lock()
	defer policySaveMu.Unlock()
	raw, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	return fsutil.WriteFile(path, raw, 0o644)
}

func dirOf(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[:i]
	}
	return "."
}

func (p *Policy) applyDefaults() {
	if p.LabMode == "" {
		p.LabMode = "isolated"
	}
	if p.ConductorMode == "" {
		p.ConductorMode = "hybrid"
	}
	if p.Combo == "" {
		p.Combo = "baseline"
	}
	// watchdog_cpu_percent: 0 disables host watchdog (lab default)
	if p.MaxDurationSec <= 0 {
		p.MaxDurationSec = 300
	}
	if p.Workers <= 0 {
		p.Workers = 4
	}
	if p.Streams <= 0 {
		p.Streams = 100
	}
	if p.BatchSize <= 0 {
		p.BatchSize = 50
	}
	if p.TargetURL == "" {
		p.TargetURL = "http://127.0.0.1:8443"
	}
	if len(p.AllowedHosts) == 0 {
		p.AllowedHosts = []string{"127.0.0.1", "localhost", "::1", "victim", "nginx-victim"}
	}
}

func (p *Policy) TargetHost() (string, error) {
	u, err := url.Parse(p.TargetURL)
	if err != nil {
		return "", err
	}
	host := u.Hostname()
	if host == "" {
		return "", fmt.Errorf("labpolicy: empty target host")
	}
	return strings.ToLower(host), nil
}

func (p *Policy) IsHostAllowed(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, allowed := range p.AllowedHosts {
		a := strings.ToLower(allowed)
		if host == a || strings.HasSuffix(host, "."+a) {
			return true
		}
	}
	return false
}

// NormalizeHost lowercases and strips trailing dots from a hostname or URL host part.
func NormalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if strings.Contains(host, "://") {
		if u, err := url.Parse(host); err == nil {
			host = u.Hostname()
		}
	}
	// Allow host:port input without scheme.
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.ToLower(strings.TrimSuffix(host, "."))
}

// AddAllowedHost appends host if not already present. Returns true if added.
func (p *Policy) AddAllowedHost(host string) bool {
	host = NormalizeHost(host)
	if host == "" {
		return false
	}
	if p.IsHostAllowed(host) {
		return false
	}
	p.AllowedHosts = append(p.AllowedHosts, host)
	return true
}

// RemoveAllowedHost removes host from the allowlist. Returns true if removed.
func (p *Policy) RemoveAllowedHost(host string) bool {
	host = NormalizeHost(host)
	if host == "" {
		return false
	}
	out := p.AllowedHosts[:0]
	removed := false
	for _, h := range p.AllowedHosts {
		if NormalizeHost(h) == host {
			removed = true
			continue
		}
		out = append(out, h)
	}
	p.AllowedHosts = out
	return removed
}
