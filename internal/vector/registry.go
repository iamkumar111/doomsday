package vector

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const DefaultVectorsPath = "configs/vectors.yaml"

type file struct {
	Vectors []Spec `yaml:"vectors"`
}

// Registry holds loaded vector specs indexed by canonical id and alias.
type Registry struct {
	byID    map[ID]Spec
	aliases map[string]ID
	order   []Spec
}

func LoadRegistry(path string) (*Registry, error) {
	if path == "" {
		path = DefaultVectorsPath
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("vector: read %s: %w", path, err)
	}
	var f file
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("vector: parse: %w", err)
	}
	r := &Registry{
		byID:    make(map[ID]Spec, len(f.Vectors)),
		aliases: make(map[string]ID, len(f.Vectors)*2),
		order:   f.Vectors,
	}
	for _, s := range f.Vectors {
		if s.ID == "" {
			continue
		}
		r.byID[s.ID] = s
		r.aliases[string(s.ID)] = s.ID
		for _, a := range s.Aliases {
			r.aliases[strings.ToLower(strings.TrimSpace(a))] = s.ID
		}
	}
	if len(r.byID) == 0 {
		return nil, fmt.Errorf("vector: no vectors in %s", path)
	}
	return r, nil
}

func (r *Registry) Resolve(input string) (Spec, error) {
	if r == nil {
		return Spec{}, fmt.Errorf("vector: nil registry")
	}
	key := strings.ToLower(strings.TrimSpace(input))
	if key == "" {
		return Spec{}, fmt.Errorf("vector: empty id")
	}
	if id, ok := r.aliases[key]; ok {
		return r.byID[id], nil
	}
	return Spec{}, fmt.Errorf("vector: unknown %q", input)
}

func (r *Registry) Get(id ID) (Spec, bool) {
	s, ok := r.byID[id]
	return s, ok
}

func (r *Registry) List() []Spec {
	out := make([]Spec, len(r.order))
	copy(out, r.order)
	return out
}

// ApplyCapabilities zeroes scale fields the vector does not support.
func (s Spec) ApplyCapabilities(in Scale) Scale {
	out := in
	if !s.Capabilities.Workers {
		out.Workers = 0
	}
	if !s.Capabilities.Streams {
		out.Streams = 0
	}
	if !s.Capabilities.Batch {
		out.Batch = 0
	}
	if !s.Capabilities.ProxyFile {
		out.ProxyFile = ""
	}
	if !s.Capabilities.Paths {
		out.WSPath = ""
		out.Paths = nil
	}
	return out
}

// FillDefaults applies registry defaults for unset positive fields.
func (s Spec) FillDefaults(in Scale) Scale {
	out := in
	if out.Workers <= 0 && s.Defaults.Workers > 0 {
		out.Workers = s.Defaults.Workers
	}
	if out.Streams <= 0 && s.Defaults.Streams > 0 {
		out.Streams = s.Defaults.Streams
	}
	if out.Batch <= 0 && s.Defaults.Batch > 0 {
		out.Batch = s.Defaults.Batch
	}
	return out
}