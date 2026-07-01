package vector

// ID is the canonical vector identifier.
type ID string

const (
	IDHTTPGet      ID = "httpget"
	IDHTTPPost     ID = "httppost"
	IDRUDY         ID = "rudy"
	IDAPIFlood     ID = "apiflood"
	IDH2RapidReset ID = "h2-rapid-reset"
	IDWSFlood      ID = "ws-flood"
	IDSlowloris    ID = "slowloris"
	IDBaseline     ID = "baseline"
)

// Capability declares which dashboard scale knobs apply.
type Capability struct {
	Workers        bool `yaml:"workers" json:"workers"`
	Streams        bool `yaml:"streams" json:"streams"`
	Batch          bool `yaml:"batch" json:"batch"`
	Paths          bool `yaml:"paths" json:"paths"`
	PayloadProfile bool `yaml:"payload_profile" json:"payload_profile"`
	ProxyFile      bool `yaml:"proxy_file" json:"proxy_file"`
}

// ScaleDefaults are suggested starting values per vector.
type ScaleDefaults struct {
	Workers int `yaml:"workers" json:"workers"`
	Streams int `yaml:"streams" json:"streams"`
	Batch   int `yaml:"batch" json:"batch"`
}

// Spec is the typed vector definition from configs/vectors.yaml.
type Spec struct {
	ID            ID            `yaml:"id" json:"id"`
	Label         string        `yaml:"label" json:"label"`
	Aliases       []string      `yaml:"aliases" json:"aliases,omitempty"`
	Worker        string        `yaml:"worker" json:"worker"`
	RedisVector   string        `yaml:"redis_vector" json:"redis_vector"`
	Protocol      string        `yaml:"protocol" json:"protocol"`
	BenchID       string        `yaml:"bench_id" json:"bench_id,omitempty"`
	Capabilities  Capability    `yaml:"capabilities" json:"capabilities"`
	Defaults      ScaleDefaults `yaml:"defaults" json:"defaults"`
	PathProfiles  []string      `yaml:"path_profiles" json:"path_profiles,omitempty"`
}

// Scale is runtime tuning passed to Run.
type Scale struct {
	Workers   int
	Streams   int
	Batch     int
	ProxyFile string
	WSPath    string
	Paths     []string
}

// RunResult is the outcome of a vector execution.
type RunResult struct {
	VectorID        ID
	Protocol        string
	Attempts        uint64
	Errors          uint64
	OpenConnections uint64
	Elapsed         float64
	RPS             float64
	ActualMode      string
}