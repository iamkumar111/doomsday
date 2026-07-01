# Deploy Guide — SH-MVDoS Lab

Setup and run instructions for a fresh server (Ubuntu/EC2) or local machine.

> **Lab only.** You must own the target or have written permission. Set `ethics_ack: I_OWN_THIS_LAB` and allowlist hosts in `data/lab-policy.yaml`.

---

## 1. Requirements

| Component | Version |
|-----------|---------|
| Go | **1.23+** |
| Docker + Compose | Optional (recommended for full lab) |
| Redis | 7.x (included in Docker profile) |
| OS | Linux amd64 (tested on Ubuntu 22.04/24.04) |

---

## 2. Clone the repository

```bash
git clone git@github.com:iamkumar111/finalsemester.git
cd finalsemester
```

Or with HTTPS:

```bash
git clone https://github.com/iamkumar111/finalsemester.git
cd finalsemester
```

---

## 3. Install Go 1.23

```bash
sudo rm -rf /usr/local/go
wget https://go.dev/dl/go1.23.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.23.0.linux-amd64.tar.gz
rm go1.23.0.linux-amd64.tar.gz

echo 'export PATH=/usr/local/go/bin:$PATH' >> ~/.bashrc
source ~/.bashrc

go version
# go version go1.23.0 linux/amd64
```

Optional — faster module downloads:

```bash
echo 'export GOPROXY=https://proxy.golang.org,direct' >> ~/.bashrc
source ~/.bashrc
```

---

## 4. Create runtime config (required)

`data/runtime.env` is **not** in Git (contains secrets). Create it on every new server:

```bash
cat > data/runtime.env <<EOF
REDIS_ADDR=redis:6379
POLICY_PATH=/data/lab-policy.yaml
PHASES_PATH=/configs/phases.yaml
COMBOS_PATH=/configs/combos.yaml
DASHBOARD_ADDR=0.0.0.0:8089
DASHBOARD_TOKEN=$(openssl rand -hex 32)
TARGET_URL=http://victim:80
EOF

# Save your token — you need it to open the dashboard
grep DASHBOARD_TOKEN data/runtime.env
```

For **native** (non-Docker) runs, use local paths:

```bash
cat > data/runtime.env <<EOF
REDIS_ADDR=127.0.0.1:6379
POLICY_PATH=data/lab-policy.yaml
PHASES_PATH=configs/phases.yaml
COMBOS_PATH=configs/combos.yaml
DASHBOARD_ADDR=127.0.0.1:8089
DASHBOARD_TOKEN=$(openssl rand -hex 32)
TARGET_URL=http://127.0.0.1:8443
EOF
```

---

## 5. Build binaries

```bash
cd /path/to/finalsemester
export PATH=/usr/local/go/bin:$PATH

go mod download
go mod tidy
make build-all
ls -la bin/
```

Expected binaries: `conductor`, `dashboard`, `l7-abuser`, `h2-thrasher`, `quic-burner`, `slowloris`, `ws-flood`, `sync-runtime`, `vector-bench`.

### Build troubleshooting

| Problem | Fix |
|---------|-----|
| `go: command not found` | `export PATH=/usr/local/go/bin:$PATH` |
| Hangs on `go: downloading ...` | Wait (first build) or set `GOPROXY=https://proxy.golang.org,direct` |
| `go.sum` missing | `go mod tidy` |
| Permission error on `bin/` | `mkdir -p bin` |

Verbose single build:

```bash
go build -v -trimpath -o bin/conductor ./cmd/conductor
```

---

## 6. Policy setup (before any attack)

Edit `data/lab-policy.yaml`:

```yaml
lab_mode: isolated
ethics_ack: I_OWN_THIS_LAB
target_url: https://YOUR-LAB-TARGET.example/
allowed_hosts:
  - 127.0.0.1
  - localhost
  - victim
  - YOUR-LAB-TARGET.example   # hostname only, no scheme
conductor_mode: hybrid          # manual | hybrid | learn-and-attack | auto
combo: slayer-mix               # see configs/combos.yaml
l7_mode: baseline               # optional override
workers: 64
streams: 500
batch_size: 200
max_duration_sec: 300
watchdog_cpu_percent: 0
```

**Rules:**
- Target must be `http` or `https`
- Host must be in `allowed_hosts`
- `conductor_mode: auto` disables dashboard manual Start (use conductor profile instead)

---

## 7. Run with Docker (recommended)

### Install Docker (Ubuntu)

```bash
sudo apt update
sudo apt install -y docker.io docker-compose-v2
sudo usermod -aG docker $USER
newgrp docker
```

### Start full lab

```bash
make lab-up
```

This starts: **Redis**, **dashboard**, **all attack vectors**, **nginx victim** (monitoring profile).

### Start dashboard only (manual control)

```bash
docker compose --profile attacker --profile dashboard up -d --build
```

### Start with auto conductor

```bash
make lab-up-auto
# or
docker compose --profile attacker --profile auto --profile vectors up -d --build
```

### Scale workers for higher load

```bash
docker compose --profile vectors up -d \
  --scale l7-abuser=8 \
  --scale h2-thrasher=4 \
  --scale slowloris=2
```

### Stop lab

```bash
make lab-down
# or
docker compose --profile attacker --profile dashboard --profile vectors --profile monitoring down
```

---

## 8. Open the dashboard

Dashboard binds to **all interfaces** (`0.0.0.0:8089`) for public IP access.

### Option A — SSH tunnel (more secure)

```bash
ssh -L 8089:127.0.0.1:8089 ubuntu@3.7.127.77
```

Then on your laptop:

```bash
TOKEN=$(ssh ubuntu@3.7.127.77 'grep DASHBOARD_TOKEN /home/ubuntu/doomsday/data/runtime.env | cut -d= -f2')
echo "http://127.0.0.1:8089/?token=$TOKEN"
```

### Option B — Public URL (EC2: `http://3.7.127.77:8089/`)

**1. Restart after pull** (picks up `0.0.0.0` bind):

```bash
cd /home/ubuntu/doomsday
git pull
cp data/runtime.env.example data/runtime.env   # if missing
# edit DASHBOARD_TOKEN in data/runtime.env
docker compose --profile attacker --profile dashboard --profile vectors up -d --build
ss -tlnp | grep 8089   # expect 0.0.0.0:8089
```

**2. Open AWS Security Group**

In EC2 console → Security Groups → Inbound rules → Add:

| Type | Port | Source |
|------|------|--------|
| Custom TCP | 8089 | **Your IP**/32 (not 0.0.0.0/0 unless you accept risk) |

**3. Use API token in URL**

```bash
grep DASHBOARD_TOKEN data/runtime.env
```

Open:

```
http://3.7.127.77:8089/?token=YOUR_TOKEN_HERE
```

Without `?token=...` the API returns 401 and the UI will not load data.

The UI stores the token in browser `localStorage` after the first visit.

### Troubleshooting public access

| Cause | Fix |
|-------|-----|
| Security group blocks 8089 | Add inbound TCP 8089 |
| Missing token | `http://IP:8089/?token=FROM_runtime.env` |
| Dashboard not running | `docker compose ps` |
| Ubuntu firewall | `sudo ufw allow 8089/tcp` |
| Localhost-only override | Remove `.env` with `DASHBOARD_HOST_BIND=127.0.0.1` |

---

## 9. Run an attack (dashboard workflow)

1. Open dashboard with token (see above).
2. **Target Intel** → enter URL → **Analyze**.
3. **Apply to policy** (saves recon draft; writes policy if host is allowlisted).
4. If blocked → **Validate** → **Add** host to allowlist → **Promote saved draft** (or re-apply).
5. **Save** policy (required — unsaved changes block Start).
6. **Start** → status should show **Running**.
7. **Stop** when finished.

### Combos (attack profiles)

| Combo | Use case |
|-------|----------|
| `baseline` | Simple L7 flood |
| `magento-abuse` | Magento catalog-search + guest cart |
| `wordpress-abuse` | Heartbeat + WP search |
| `shopify-abuse` | Shopify search/cart |
| `api-abuse` | API / GraphQL / WS / H2 |
| `slayer-mix` | Mixed L7 + H2 + slowloris |
| `cms-abuse` | CMS rotate (auto by detected CMS) |

List in UI combo dropdown or `configs/combos.yaml`.

---

## 10. Native run (no Docker)

Terminal 1 — Redis:

```bash
redis-server --port 6379
```

Terminal 2 — victim (optional lab target):

```bash
python3 deploy/scripts/native-victim.py
# listens on http://127.0.0.1:8443
```

Terminal 3 — workers:

```bash
export $(grep -v '^#' data/runtime.env | xargs)
# Fix paths for native:
export REDIS_ADDR=127.0.0.1:6379
export POLICY_PATH=data/lab-policy.yaml
export PHASES_PATH=configs/phases.yaml
export COMBOS_PATH=configs/combos.yaml

./bin/l7-abuser &
./bin/h2-thrasher &
./bin/slowloris &
./bin/ws-flood &
```

Terminal 4 — dashboard:

```bash
export POLICY_PATH=data/lab-policy.yaml
export PHASES_PATH=configs/phases.yaml
export COMBOS_PATH=configs/combos.yaml
export REDIS_ADDR=127.0.0.1:6379
export DASHBOARD_ADDR=127.0.0.1:8089
export DASHBOARD_TOKEN=$(grep DASHBOARD_TOKEN data/runtime.env | cut -d= -f2)

./bin/dashboard
```

---

## 11. Bench vectors (no Redis)

Quick test without full stack:

```bash
make build-all
make list-vectors

./bin/vector-bench -vector l7-magento-search -target http://127.0.0.1:8443 -duration 15s
./bin/vector-bench -all -json > bench-results.json
```

---

## 12. EC2 quick checklist

```bash
# 1. Clone
git clone git@github.com:iamkumar111/finalsemester.git && cd finalsemester

# 2. Go
sudo tar -C /usr/local -xzf go1.23.0.linux-amd64.tar.gz
export PATH=/usr/local/go/bin:$PATH

# 3. Runtime env + token
openssl rand -hex 32   # paste into data/runtime.env

# 4. Build
go mod download && make build-all

# 5. Docker lab
sudo apt install -y docker.io docker-compose-v2
make lab-up

# 6. Tunnel from laptop
ssh -L 8089:127.0.0.1:8089 ubuntu@EC2_IP

# 7. Open dashboard with token
```

---

## 13. Files reference

| File | Purpose |
|------|---------|
| `data/runtime.env` | Redis, paths, **DASHBOARD_TOKEN** (local only) |
| `data/lab-policy.yaml` | Target, allowlist, combo, scale |
| `configs/phases.yaml` | Phase definitions per vector |
| `configs/combos.yaml` | Attack combos |
| `data/.recon-draft.json` | Candidate intel (not runnable until promoted) |
| `data/.dashboard-run-snapshot.json` | Dashboard run state (auto) |

---

## 14. Security notes

- Change `DASHBOARD_TOKEN` on every deployment; never commit `data/runtime.env`.
- Dashboard listens on `0.0.0.0:8089` by default — restrict security group to your IP.
- Do not remove `allowed_hosts` checks or point at systems you do not own.
- `watchdog_cpu_percent: 0` disables CPU kill switch (lab default).

---

## 15. Get help

```bash
go test ./...
make build-all 2>&1 | tee build.log
docker compose --profile dashboard logs dashboard
docker compose --profile vectors ps
```

Repository: https://github.com/iamkumar111/finalsemester