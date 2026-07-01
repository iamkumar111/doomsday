package vector

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const DefaultPathProfilesPath = "configs/path-profiles.yaml"

type pathFile struct {
	Profiles map[string]struct {
		Paths []string `yaml:"paths"`
	} `yaml:"profiles"`
}

// PathProfiles maps profile name to path suffixes.
type PathProfiles struct {
	profiles map[string][]string
}

func LoadPathProfiles(path string) (*PathProfiles, error) {
	if path == "" {
		path = DefaultPathProfilesPath
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("vector paths: read %s: %w", path, err)
	}
	var f pathFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("vector paths: parse: %w", err)
	}
	p := &PathProfiles{profiles: make(map[string][]string, len(f.Profiles))}
	for name, prof := range f.Profiles {
		p.profiles[name] = append([]string(nil), prof.Paths...)
	}
	return p, nil
}

func (p *PathProfiles) Paths(profile string) []string {
	if p == nil {
		return nil
	}
	return append([]string(nil), p.profiles[profile]...)
}

func (p *PathProfiles) ResolveForVector(spec Spec, profile string, explicit []string) []string {
	if len(explicit) > 0 {
		return explicit
	}
	if profile != "" {
		if paths := p.Paths(profile); len(paths) > 0 {
			return paths
		}
	}
	for _, key := range spec.PathProfiles {
		if paths := p.Paths(key); len(paths) > 0 {
			return paths
		}
	}
	return nil
}

// BuildTargetURLs joins base target with path suffixes.
func BuildTargetURLs(base string, paths []string) []string {
	if len(paths) == 0 {
		return []string{strings.TrimRight(base, "/")}
	}
	u, err := url.Parse(base)
	if err != nil {
		return []string{base}
	}
	origin := strings.TrimRight(base, "/")
	if u.Scheme != "" && u.Host != "" {
		origin = fmt.Sprintf("%s://%s", u.Scheme, u.Host)
	}
	var out []string
	seen := map[string]struct{}{}
	for _, p := range paths {
		if p == "" {
			p = "/"
		}
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		full := origin + p
		if _, ok := seen[full]; ok {
			continue
		}
		seen[full] = struct{}{}
		out = append(out, full)
	}
	return out
}