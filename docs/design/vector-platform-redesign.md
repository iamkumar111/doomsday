# Vector Platform Redesign — Safe, High-Performance Plan

**Status:** Phase A implemented (2026-07-01) — Phase B/C in progress  
**Goal:** Match or exceed Slayer-L7 lab performance while keeping ethics guardrails, allowlists, and observability the dashboard stack provides.  
**Non-goal:** Remove safety controls or enable use against non-owned targets.

---

## 0. Why Slayer Wins Today (Honest Root Cause)

Slayer (`Slayer-L7/Slayer.go`) is a **single-process, zero-hop load generator**:

| Slayer | Current dashboard stack |
|--------|-------------------------|
| `./slayer -w 500` → 500 goroutines immediately | Workers=500 split across Redis phases, combos, containers |
| Direct `http.Client` pool (up to 256 clients) | Per-worker client pool, busloop overhead |
| RUDY: 1 blocking slow POST per goroutine | Was batch-capped; improved but still behind orchestration |
| No Redis, no phase delay, no combo dilution | Pub/Sub + durable replay + multi-vector combos divide load |
| `rapidreset` / `wsflood` raw protocol paths | Renamed vectors, combo-only routing until recent fixes |
| Operator sets exact scale once | `streams`/`batch` historically mis-bound per vector |

**Conclusion:** Performance gap is primarily **architecture**, not missing HTTP code. Fixes require a **typed vector model**, **capability-bound dashboard**, and a **fast path** that can run Slayer-parity without Redis when doing lab benchmarks.

---

## 1. Vector Model (Typed, First-Class)

Every attack mode becomes a **VectorSpec** — not a string in `l7_mode`.

### 1.1 Core types (`internal/vector/spec.go`)

```go
type VectorID string

const (
    VectorHTTPGet       VectorID = "httpget"
    VectorHTTPPost      VectorID = "httppost"
    VectorRUDY          VectorID = "rudy"
    VectorAPIFlood      VectorID = "apiflood"
    VectorH2RapidReset  VectorID = "h2-rapid-reset"
    VectorWSFlood       VectorID = "ws-flood"
)

type Capability struct {
    Workers          bool // goroutine / connection pool size
    Streams          bool // parallel in-flight (H2 streams, WS conns, L7 parallel)
    Batch            bool // requests per burst / messages per session
    Paths            bool // explicit URL/path list
    PayloadProfile   bool // form/json/graphql/cms profile
    ProxyFile        bool
    DurationOverride bool // per-phase duration
}

type VectorSpec struct {
    ID               VectorID
    Aliases          []string          // rapidreset, wsflood, post, api
    WorkerBinary     string            // l7-abuser | h2-thrasher | ws-flood
    RedisVector      string            // bus channel identity
    ExpectedProtocol string            // http/1.1 | h2 | ws | quic
    Capabilities     Capability
    DefaultScale     ScaleDefaults
    PathProfiles     []string          // keys into path-profiles.yaml
}

type ScaleDefaults struct {
    Workers int
    Streams int
    Batch   int
}
```

### 1.2 Registry file (`configs/vectors.yaml`)

Single source of truth replacing scattered mode strings:

```yaml
vectors:
  - id: httpget
    aliases: [get]
    worker: l7-abuser
    redis_vector: l7-abuser
    protocol: http/1.1
    capabilities:
      workers: true
      streams: true   # parallel requests per batch step
      batch: true
      paths: true
      payload_profile: false
      proxy_file: true
    defaults: { workers: 64, streams: 4, batch: 50 }

  - id: rudy
    aliases: []
    worker: l7-abuser
    protocol: http/1.1
    capabilities:
      workers: true
      streams: false  # 1 slow POST per worker — Slayer model
      batch: false
      paths: false
      proxy_file: true
    defaults: { workers: 500 }

  - id: h2-rapid-reset
    aliases: [rapidreset, h2rapid]
    worker: h2-thrasher
    redis_vector: h2-rapid-reset
    protocol: h2
    capabilities:
      workers: true
      streams: true
      batch: true
    defaults: { workers: 32, streams: 500, batch: 100 }

  - id: ws-flood
    aliases: [wsflood]
    worker: ws-flood
    protocol: websocket
    capabilities:
      workers: true
      streams: true   # conns per worker
      batch: true     # msgs per session
      paths: true     # REQUIRED for lab accuracy
    defaults: { workers: 64, streams: 8, batch: 50 }
```

### 1.3 Alias resolver

```go
func ResolveVectorID(input string) (VectorID, error)
```

Maps UI/config: `rapidreset` → `h2-rapid-reset`, `post` → `httppost`, `api` → `apiflood`, `wsflood` → `ws-flood`.

---

## 2. Dashboard Binding (Capability-Driven UI)

**Rule:** Controls are enabled only when `VectorSpec.Capabilities` says so.

### 2.1 API: `GET /api/vectors`

Returns full registry + capabilities for dynamic UI (replaces hardcoded `updateScaleBindings()`).

### 2.2 UI behavior

| Control | Enabled when |
|---------|----------------|
| Workers | `capabilities.workers` |
| Streams | `capabilities.streams` |
| Batch | `capabilities.batch` |
| Duration | always (policy `max_duration_sec`) |
| Path / WS path | `capabilities.paths` |
| Payload profile | `capabilities.payload_profile` |
| Proxy file | `capabilities.proxy_file` |
| Watchdog % | always (0 = off) |

### 2.3 Preview panel (before Start)

`POST /api/plan/preview` returns:

```json
{
  "combo": "rudy-stress",
  "phases": [
    {
      "vector": "rudy",
      "worker": "l7-abuser",
      "start_after_sec": 0,
      "duration_sec": 300,
      "scale": { "workers": 500 },
      "paths": ["/"],
      "protocol": "http/1.1"
    }
  ],
  "required_workers": ["l7-abuser"],
  "warnings": []
}
```

User sees **exactly** what will run — fixes “dashboard cannot prove what ran.”

---

## 3. Combo Engine (Planner, Not Static YAML)

Replace `configs/combos.yaml` phase ID lists with a **ComboPlan**.

### 3.1 Plan schema (`configs/combo-plans.yaml`)

```yaml
plans:
  - id: slayer-rudy
    label: Slayer RUDY parity
    phases:
      - vector: rudy
        start_after_sec: 0
        duration_sec: 0        # inherit run max_duration
        weight: 1.0
        params:
          path_profile: default

  - id: protocol-mix
    label: L7 + H2 + WS
    phases:
      - vector: httppost
        start_after_sec: 0
        weight: 0.4
      - vector: h2-rapid-reset
        start_after_sec: 2
        weight: 0.4
      - vector: ws-flood
        start_after_sec: 5
        weight: 0.2
        params:
          paths: ["/ws", "/socket.io/"]
```

### 3.2 Weight → scale budget

Total dashboard scale `(W, S, B)` is split by weight:

```
phase_workers = floor(run_workers * weight / sum_weights)
```

Prevents combo from running **full 500 workers per phase** (current dilution bug when multiple phases run concurrently).

### 3.3 Planner (`internal/planner/plan.go`)

```go
func BuildRunPlan(policy Policy, plan ComboPlan, registry VectorRegistry) (RunPlan, error)
```

Outputs ordered `[]PlannedPhase` with resolved scales, paths, worker binaries, Redis events.

---

## 4. Worker Registry (Rich Heartbeat)

Extend current `shmv:worker:{vector}` TTL keys.

### 4.1 Heartbeat payload

```json
{
  "vector": "l7-abuser",
  "version": "8b1f56d",
  "capacity": {
    "max_workers": 2048,
    "max_streams": 10000
  },
  "active_runs": 2,
  "last_seen": 1751366400,
  "host": "l7-abuser-3"
}
```

### 4.2 Preflight (`internal/registry/preflight.go`)

Before Start:

1. Resolve required worker binaries from `RunPlan`
2. Check `last_seen < 30s` and `version` compatible
3. Return `503` with missing list (already partially implemented — extend with capacity)

### 4.3 Optional: worker admission

Workers report `active_runs`; dashboard warns if container is saturated.

---

## 5. Metrics (Prove What Ran)

### 5.1 Per-vector metrics event

Extend `redisbus.MetricsEvent`:

```go
type MetricsEvent struct {
    RunID            string
    PhaseID          string
    Vector           string            // canonical id: rudy, h2-rapid-reset
    ActualMode       string            // what worker executed
    Protocol         string            // negotiated: h2, http/1.1, ws
    Attempts         uint64
    Success          uint64
    Errors           uint64
    OpenConnections  uint64            // RUDY/slowloris/WS
    LatencyP50Ms     float64
    LatencyP99Ms     float64
    RPS              float64
    Timestamp        int64
}
```

### 5.2 Worker-side collector

Each vector reports every 2s during run (not only at end). Dashboard charts live open connections for RUDY — the metric that proves Slayer-style hold.

### 5.3 Run receipt

On phase end, worker writes `shmv:run:{id}:receipt:{phase}` JSON — durable proof for audit.

---

## 6. Benchmark Mode (Lab-Only Slayer Parity)

**Purpose:** Compare vector implementations against **your** lab victim without full Redis orchestration.

### 6.1 CLI (extend `cmd/slayer` + `cmd/vector-bench`)

```bash
# Slayer-identical flags (allowlist enforced)
./slayer -t https://lab.example -m rudy -w 500 -d 300 -p proxies.txt

# Typed benchmark with JSON report
vector-bench -vector rudy -target $LAB_URL -workers 500 -duration 300s -json

# Compare all vectors
vector-bench -matrix -target $LAB_URL -workers 500 -duration 60s
```

### 6.2 Benchmark matrix output

| vector | rps | errors | open_conns | vs_slayer_pct |
|--------|-----|--------|------------|---------------|
| rudy | 12 | 707 | 498 | 98% |

Acceptance gate: **≥90% of Slayer** on same host for `rudy`, `httpget`, `httppost` before enabling in dashboard default combos.

### 6.3 Fast path architecture

```
┌─────────────────┐     ┌──────────────────┐
│  cmd/slayer     │     │  dashboard+redis   │
│  (direct run)   │     │  (orchestrated)    │
└────────┬────────┘     └────────┬─────────┘
         │                       │
         └───────────┬───────────┘
                     ▼
           internal/vector/runner.go
           (shared VectorSpec + engines)
```

Both paths call the same `vector.Run(ctx, VectorSpec, Scale, Target)` — no duplicate HTTP code.

---

## 7. Naming Cleanup

### 7.1 Alias table (config + code)

| Alias | Canonical |
|-------|-----------|
| `rapidreset`, `h2rapid` | `h2-rapid-reset` |
| `wsflood` | `ws-flood` |
| `post` | `httppost` |
| `api` | `apiflood` |
| `get` | `httpget` |

### 7.2 Deprecation

- `l7_mode` policy field → alias of `attack_vector` (transitional)
- `configs/phases.yaml` phase IDs remain for backward compat; planner is primary

---

## 8. Path Profiles

### 8.1 File (`configs/path-profiles.yaml`)

```yaml
profiles:
  default:
    paths: ["/", "/index.html"]
  api:
    paths: ["/api/v1/users", "/api/search", "/graphql"]
  graphql:
    paths: ["/graphql"]
  websocket:
    paths: ["/ws", "/websocket", "/socket.io/", "/api/ws"]
  wordpress:
    paths: ["/wp-admin/admin-ajax.php", "/wp-json/wp/v2/posts"]
  magento:
    paths: ["/catalogsearch/result/", "/rest/V1/guest-carts"]
```

### 8.2 Binding

- `httpget`/`httppost`/`apiflood`: rotate paths from profile
- `ws-flood`: **use explicit paths only** when profile set; no guessing
- Dashboard: Path profile dropdown + custom path list override

---

## 9. Performance Targets (Lab SLOs)

Measured on same EC2 → same lab target as Slayer:

| Vector | Slayer baseline | Platform target |
|--------|-----------------|-----------------|
| rudy | 500 open POSTs, ~700 errors/300s | ≥450 open conns, ≥90% error rate |
| httpget | max RPS | ≥85% Slayer RPS |
| h2-rapid-reset | RST batch rate | ≥80% (protocol-sensitive) |
| ws-flood | msgs/sec with known path | ≥85% with explicit path |

**Scale limits (single container):**

- `l7-abuser`: up to 2048 workers (match Slayer default)
- Client pool: `workers/8` capped at 256 (Slayer `maxDirectPool`)
- RUDY: never batch-cap; 1 hold per worker goroutine

---

## 10. Safety Guardrails (Non-Negotiable)

1. `ethics_ack: I_OWN_THIS_LAB` + `allowed_hosts` — unchanged
2. Benchmark + slayer CLI require policy allowlist
3. No bypass mode for production targets
4. Watchdog CPU optional (0 = off)
5. `max_duration_sec` enforced **in worker** (already added) + dashboard
6. All runs logged with vector receipt metrics

---

## 11. Implementation Roadmap (PR Stack)

### Phase A — Foundation (week 1)
| PR | Scope |
|----|-------|
| A1 | `configs/vectors.yaml` + `internal/vector` registry + alias resolver |
| A2 | `configs/path-profiles.yaml` + path resolver |
| A3 | Unify runners: `vector.Run()` shared by slayer + workers |
| A4 | Benchmark matrix + Slayer comparison script |

### Phase B — Control plane (week 2)
| PR | Scope |
|----|-------|
| B1 | `internal/planner` combo engine + weight-based scale split |
| B2 | `GET /api/vectors`, `POST /api/plan/preview` |
| B3 | Dashboard capability UI (replace static hints) |
| B4 | Rich worker registry + preflight v2 |

### Phase C — Observability (week 3)
| PR | Scope |
|----|-------|
| C1 | Extended metrics + live open_connections |
| C2 | Run receipts in Redis |
| C3 | Dashboard “what ran” panel with protocol + actual_mode |

### Phase D — Performance hardening (week 4)
| PR | Scope |
|----|-------|
| D1 | RUDY/H2/WS Slayer parity audit per vector |
| D2 | Multi-container scale guide (`docker compose --scale l7-abuser=N`) |
| D3 | Proxy pool shared across all L7 vectors |
| D4 | Acceptance tests: ≥90% Slayer on rudy/httpget |

---

## 12. Immediate Actions (Before Full Redesign)

What you can do **today** on EC2 for Slayer-like RUDY:

```bash
# Direct — highest performance
go build -o slayer ./cmd/slayer
./slayer -t https://YOUR-LAB-TARGET -m rudy -w 500 -d 300

# Dashboard — after git pull
# Attack mode: rudy | Combo: rudy-stress | Workers: 500 | max_duration: 300
# Ensure only l7-abuser running (no combo dilution)
docker compose --profile vectors up -d --scale l7-abuser=1
```

---

## 13. Decision Summary

| Question | Decision |
|----------|----------|
| One worker or many modes? | **Typed vectors**; `l7-abuser` is a worker binary hosting multiple L7 vector IDs |
| Combos? | **Planner with weights** — not flat phase lists |
| Slayer compatibility? | **`cmd/slayer` fast path** + shared engines |
| Dashboard streams? | **Capability-gated** per vector |
| ws-flood paths? | **Required path profile** — no guessing in production plans |

This plan keeps the platform safe and observable while closing the performance gap by eliminating orchestration overhead on the hot path and binding scale controls to what each vector actually uses.