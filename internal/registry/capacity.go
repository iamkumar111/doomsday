package registry

// Capacity declares per-worker container limits for preflight admission.
type Capacity struct {
	MaxWorkers int `json:"max_workers"`
	MaxStreams int `json:"max_streams"`
}

// DefaultCapacity maps heartbeat vector keys to declared limits.
var DefaultCapacity = map[string]Capacity{
	"l7-abuser":       {MaxWorkers: 2048, MaxStreams: 10000},
	"h2-rapid-reset":  {MaxWorkers: 512, MaxStreams: 10000},
	"ws-flood":        {MaxWorkers: 512, MaxStreams: 4096},
	"slowloris":       {MaxWorkers: 2048, MaxStreams: 0},
	"quic-burner":     {MaxWorkers: 256, MaxStreams: 0},
}

func LookupCapacity(vector string) Capacity {
	if c, ok := DefaultCapacity[vector]; ok {
		return c
	}
	return Capacity{MaxWorkers: 512, MaxStreams: 4096}
}