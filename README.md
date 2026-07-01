# SH-MVDoS v2

**S**tealth **H**ybrid **M**ulti-**V**ector **D**enial-of-**S**ervice — authorized lab framework for 2026 DDoS defense research.

> **Ethics:** Lab-only. Set `ethics_ack: I_OWN_THIS_LAB` and allowlist targets in `data/lab-policy.yaml`. Never aim at public sites without written permission.

## Research

See [docs/research/ADVANCED-DDOS-2026.md](docs/research/ADVANCED-DDOS-2026.md) for the 2025–2026 literature review (Cloudflare, Akamai, AdaDoS, MadeYouReset, QUIC exhaustion, API abuse).

## Phase 1 — Test vectors (no Redis)

Bench every method, measure effectiveness and attacker effort, then decide what to wire into Redis.

```bash
export PATH="$HOME/.local/go/bin:$PATH"
make build-all

# List all vectors
make list-vectors

# Bench all against lab victim (auto-starts native-victim.py)
make bench

# Single vector
./bin/vector-bench -vector l7-baseline -target http://127.0.0.1:8443 -duration 10s
./bin/vector-bench -all -json > results.json
```

Output columns: **effectiveness** (impact on victim), **effort** (attacker cost/errors), **redis-ready** (recommend for phase 2).

## Phase 2 — Multiprocess via Redis

After benching, enable only `redis-ready` vectors in `configs/phases.yaml`.

```bash
# Terminal 1 — victim
python3 deploy/scripts/native-victim.py

# Terminal 2 — Redis
redis-server &

# Terminal 3 — workers
./bin/l7-abuser &
./bin/h2-thrasher &

# Terminal 4 — dashboard
./bin/dashboard
# Open http://127.0.0.1:8089
```

## Modes

| Mode | Behavior |
|------|----------|
| manual | Operator triggers phases from dashboard |
| hybrid | Dashboard + timed conductor |
| auto | Conductor-only phase schedule |
| adaptive | RTT-probe scaling (future) |

## Vector catalog (2026)

- `l7-abuser` — HTTP flood, WordPress/GraphQL paths
- `h2-rapid-reset` / `h2-thrasher` — HTTP/2 stream abuse (needs HTTP/2 victim)
- `quic-burner` — HTTP/3 / QUIC stress
- Combos in `configs/combos.yaml`

## Docker lab

```bash
make lab-up    # requires Docker daemon
make lab-down
```

## Lessons baked into v2

- Attack uses `context.Background()` in dashboard (not request context)
- UI polls status only — form edits are not overwritten
- `watchdog_cpu_percent: 0` disables host kill switch
- Docker uses `env_file: data/runtime.env` (no compose `${VAR}` drift)

## New: Target Intel & Attack Planner (Dashboard)

Enter any website URL in the Dashboard → **Analyze Target**.

- Auto-detects WAF (Cloudflare, Akamai, Fastly), server, CMS (WordPress etc.), frameworks (React, Next.js...), tech stack from headers + body.
- Recommends the best vectors + combo from the catalog.
- "Extreme" mode sets very high workers/batch/streams for multi-container load.
- "Apply to Policy" + "Start Recommended" fully managed from UI.
- For true extreme load: `docker compose --profile vectors up --scale l7-abuser=12 --scale h2-thrasher=8`

Inspired by bypass/recon tools like bypasscloudflare-main (origin + stack detection) but 100% self-contained in Go (no external APIs).

## Project layout

```
cmd/{conductor,dashboard,h2-thrasher,l7-abuser,quic-burner,sync-runtime}
internal/{guard,labpolicy,redisbus,orchestrator,worker,dashboard}
configs/{phases.yaml,combos.yaml}
data/{lab-policy.yaml,runtime.env}
```