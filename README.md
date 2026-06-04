# LoadEquilibrium

![Go](https://img.shields.io/badge/Go-1.20+-blue?style=flat-square)
![Docker](https://img.shields.io/badge/Docker-Ready-brightgreen?style=flat-square)
![Kubernetes](https://img.shields.io/badge/Kubernetes-Compatible-326ce5?style=flat-square)
![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)

> Predictive auto-scaling for Docker & Kubernetes. Watches your services, predicts failures 60 seconds ahead, and scales automatically using control theory (MPC + RL).

---

## What It Does

You add one label to each service you want monitored. LoadEquilibrium does the rest:

- Watches your services every 2 seconds
- Builds a live mathematical model of each service's queue, latency, and load
- Predicts when a service is about to fail — before it actually does
- Issues precise scaling decisions automatically
- Shows you everything in a live dashboard

It is not a threshold alarm. It does not wait until latency spikes to react. It uses the same class of control system used in aircraft autopilots — applied to your software.

---

## Getting Started in 3 Steps

### Step 1 — Add a label to each service you want monitored

```yaml
# your existing docker-compose.yml
services:
  my-api:
    image: your-app:latest
    labels:
      le.enable: "true"    # ← add this one line
```

That is the only change you make to your existing service. No agents to install. No config files to write.

### Step 2 — Add LoadEquilibrium to your compose file

```yaml
  loadequilibrium:
    image: ghcr.io/your-org/loadequilibrium:latest
    ports:
      - "8080:8080"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
```

### Step 3 — Start it

```bash
docker compose up -d
```

Open your browser at `http://localhost:8080`. You will see your services appear automatically within about 5 seconds.

---

## What Your Services Need to Expose

Your services must expose a `/metrics` endpoint in Prometheus format. This is standard for any service built with:

- Go (`prometheus/client_golang`)
- Python (`prometheus_client`)
- Node.js (`prom-client`)
- Java (`micrometer`)
- Any language with a Prometheus client library

The collector auto-detects the port. If your service exposes metrics on a non-standard port, add one more label:

```yaml
labels:
  le.enable: "true"
  le.port: "9100"    # only needed for non-standard ports
```

---

## What You See in the Dashboard

**Monitor page** — read-only view of what is happening:

- Live metrics per service: requests per second, queue depth, wait time, capacity used
- Failure risk score — how likely each service is to stop responding in the next 60 seconds
- Incident timeline — every problem detected, what the engine predicted, what action it took
- Engine reasoning feed — plain-English explanation of what the autopilot is thinking right now
- Live event stream — everything happening across all services in chronological order

**Control page** — actions you can take:

- Enable or freeze the autopilot (freeze = it keeps watching but stops issuing commands)
- Switch operating policy: Safe Mode / Normal / Performance
- Run a stress test on any service to see how the autopilot responds
- Force the engine to step manually or retrain its model

---

## Architecture: One Image, One Port

The entire system runs in a single Docker container exposing port `8080`:

```
Your Services (with le.enable=true label)
        │
        │  Auto-discovered via Docker socket
        │  Scraped every 2 seconds
        ▼
┌─────────────────────────────────────────┐
│         LoadEquilibrium :8080           │
│                                         │
│  Collector ──► Telemetry Store          │
│                      │                  │
│               Tick Engine (2s)          │
│                 │         │             │
│          Autopilot    Reasoning         │
│          (MPC+RL)     (Events)          │
│                 │         │             │
│          WebSocket Broadcast            │
│                 │                       │
│            UI (React)                  │
└─────────────────────────────────────────┘
        │
        ▼
  http://localhost:8080
```

- Port `8080` — dashboard UI, WebSocket live feed, REST API, Prometheus metrics
- No separate collector container
- No separate nginx
- No Prometheus required (it is optional, for Grafana users)
- No database required (optional, for persistent history)

---

## Optional: Grafana + Prometheus

If you already use Grafana, LoadEquilibrium exposes a `/metrics` endpoint that Prometheus can scrape. A pre-built dashboard is included.

Add to your compose file:

```yaml
  prometheus:
    image: prom/prometheus:v2.52.0
    volumes:
      - ./monitoring/prometheus.yml:/etc/prometheus/prometheus.yml:ro
    ports:
      - "9090:9090"

  grafana:
    image: grafana/grafana:10.4.0
    ports:
      - "3000:3000"
    volumes:
      - ./monitoring/grafana/datasource.yml:/etc/grafana/provisioning/datasources/datasource.yml:ro
      - ./monitoring/grafana/provider.yml:/etc/grafana/provisioning/dashboards/provider.yml:ro
      - ./monitoring/grafana/loadequilibrium-dashboard.json:/var/lib/grafana/dashboards/loadequilibrium-dashboard.json:ro
```

Grafana dashboard shows: traffic, latency, queue depth, autopilot decisions, signal quality, and Go runtime metrics.

---

## Optional: Persistent History (PostgreSQL)

Without a database, the engine runs entirely in memory. Data is lost on restart but the system works perfectly for real-time monitoring.

To keep a history of engine snapshots:

```yaml
  loadequilibrium:
    environment:
      DATABASE_URL: "postgres://le:yourpassword@postgres:5432/le?sslmode=disable"

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER:     le
      POSTGRES_PASSWORD: yourpassword
      POSTGRES_DB:       le
```

The schema is created automatically on first start. No migrations to run.

---

## Environment Variables

All settings have safe defaults. You only need to set `INGEST_TOKEN` in production.

| Variable | Default | What it does |
|---|---|---|
| `INGEST_TOKEN` | *(empty)* | Auth token for the ingest API. Set this in production. |
| `DATABASE_URL` | *(empty)* | Postgres DSN. Leave empty to run in-memory. |
| `LISTEN_ADDR` | `:8080` | Port to listen on. |
| `TICK_INTERVAL` | `2s` | How often the engine runs. Do not change unless you have a reason. |
| `UTILISATION_SETPOINT` | `0.70` | Target capacity utilisation (70%). Leave 30% headroom. |
| `MAX_SERVICES` | `200` | Maximum number of services to track. |
| `LE_PORT` | `8080` | Host port (compose only). Change if 8080 is taken on your machine. |

Full list of all variables is in the [Configuration Reference](#configuration-reference) section below.

---

## Kubernetes Deployment

For production on Kubernetes, use the manifests in `k8s/`. The image is the same — just one Deployment.

```bash
# 1. Create secrets (never put real values in the YAML files)
kubectl create secret generic loadequilibrium-secrets \
  --from-literal=database-url='postgres://le:YOURPASS@postgres-svc:5432/le?sslmode=require' \
  --from-literal=ingest-token='your-real-token' \
  -n loadequilibrium

# 2. Apply manifests
kubectl apply -f k8s/00-namespace.yml
kubectl apply -f k8s/01-secrets.yml
kubectl apply -f k8s/02-configmap.yml
kubectl apply -f k8s/03-postgres.yml
kubectl apply -f k8s/04-deployment.yml
kubectl apply -f k8s/05-prometheus.yml
kubectl apply -f k8s/06-grafana.yml
kubectl apply -f k8s/07-ingress.yml   # edit hostnames first

# 3. Watch it start
kubectl rollout status deployment/loadequilibrium -n loadequilibrium

# 4. Access before ingress is ready
kubectl port-forward svc/loadequilibrium-svc 8080:80 -n loadequilibrium
```

**Important**: Run exactly 1 replica. The engine keeps in-memory state that is not distributed across pods. If you need high availability, run active-passive with a shared PostgreSQL backend (bot[...]

---

## How to Send Metrics Manually (Without Docker)

If you are not using Docker (e.g. running services as systemd units, on bare metal, or in a different container runtime), use `collector.py` to push metrics from an existing Prometheus server:

```bash
pip install requests
PROMETHEUS_URL=http://your-prometheus:9090 \
INGEST_URL=http://loadequilibrium:8080/api/v1/ingest \
python3 collector.py
```

Or push directly to the ingest API from your application code:

```bash
curl -X POST http://localhost:8080/api/v1/ingest \
  -H "Content-Type: application/json" \
  -d '[{
    "service_id":    "my-api",
    "request_rate":  142.5,
    "error_rate":    0.002,
    "latency": { "p50": 12.1, "p95": 48.3, "p99": 91.2, "mean": 18.4 },
    "queue_depth":   23,
    "active_conns":  87
  }]'
```

---

## API Reference

| Method | Path | What it does |
|---|---|---|
| `GET` | `/` | Dashboard UI |
| `GET` | `/ws` | WebSocket — live tick stream |
| `GET` | `/api/v1/snapshot` | Last tick state as JSON (no WebSocket needed) |
| `GET` | `/health` | Liveness check — returns `{"status":"ok"}` |
| `GET` | `/metrics` | Prometheus metrics |
| `POST` | `/api/v1/ingest` | Push telemetry points |
| `POST` | `/api/v1/control/toggle` | Enable / freeze autopilot |
| `POST` | `/api/v1/policy/update` | Change policy preset |
| `POST` | `/api/v1/control/chaos-run` | Inject a load spike for testing |
| `POST` | `/api/v1/control/replay-burst` | Replay a traffic burst |
| `POST` | `/api/v1/runtime/step` | Force one engine tick manually |
| `POST` | `/api/v1/alerts/ack` | Acknowledge a reasoning event |

---

## How the Engine Works (For Those Who Want to Know)

Every 2 seconds, this sequence runs:

1. **Collect** — scrape `/metrics` from all labelled services
2. **Window** — compute EWMA fast/slow, variance, confidence score, signal quality per service
3. **Topology** — build a dependency graph from upstream call data (if services report it)
4. **Model** — queue physics per service: utilisation ρ, mean wait time, queue depth, burst amplification
5. **Reason** — rule engine fires events: `collapse_risk`, `cascade_risk`, `saturation_predicted`, `keystone_degraded`
6. **Simulate** — Monte Carlo forward projection: what happens in the next 60 seconds under current trend?
7. **Autopilot** — MPC (Model Predictive Control) + RL (policy gradient) computes target capacity
8. **Decide** — Control authority converts float target to integer replica count, enforces cooldowns
9. **Actuate** — Send scaling directive to your orchestrator (Kubernetes, Nomad, etc.)
10. **Broadcast** — Push full tick state to all WebSocket clients (your dashboard)

The sandbox runs every 10 ticks: it takes the current service state, generates a synthetic load spike from real statistics, runs two competing control strategies against it, and uses the result t[...]

---

## Verifying Everything Works

Run these three commands. All three must pass before deploying to production.

```bash
# Compile check — catches any broken wiring
go build ./...

# Unit + integration tests with race detector
go test -race -count=1 -timeout=300s ./internal/...

# Full autopilot system test — 10 scenarios, must all pass
go build -o system_test_runner ./cmd/system_test_runner/
./system_test_runner 2>/dev/null
# Expected output line: "STABLE_PRODUCTION_GRADE — 10/10 — 0 SLA breaches"
```

---

## Files You Can Delete

These files are safe to remove. They are developer tools that are never included in the Docker image:

| File | What it is | Safe to delete? |
|---|---|---|
| `Dockerfile.collector` | Old sidecar build — replaced by embedded goroutine | ✅ Yes |
| `ui/Dockerfile` | Old standalone nginx UI build — replaced by Go binary | ✅ Yes |
| `run_physics_validation.sh` | Developer script to validate physics model output | ✅ Yes (keep if you develop the engine) |
| `collector.py` | Alternative Python collector for non-Docker environments | ❌ Keep — useful for Prometheus users |
| `Makefile` | Build convenience commands | ❌ Keep — useful for developers |
| `cmd/system_test_runner/` | Autopilot test suite | ❌ Keep — CI depends on it |

---

## Configuration Reference

### Core Engine

| Variable | Default | Description |
|---|---|---|
| `LISTEN_ADDR` | `:8080` | HTTP server bind address |
| `TICK_INTERVAL` | `2s` | Control tick frequency |
| `TICK_DEADLINE` | `1800ms` | Max time per tick before adaptive stretch |
| `MIN_TICK_INTERVAL` | `1s` | Minimum tick interval under stretch |
| `MAX_TICK_INTERVAL` | `10s` | Maximum tick interval under stretch |
| `WORKER_POOL_SIZE` | `8` | Parallel workers for window computation |

### Telemetry

| Variable | Default | Description |
|---|---|---|
| `RING_BUFFER_DEPTH` | `300` | Samples retained per service |
| `MAX_SERVICES` | `200` | Maximum tracked services |
| `STALE_SERVICE_AGE` | `5m` | Prune threshold for inactive services |
| `INGEST_TOKEN` | `` | Auth token — set this in production |

### Control Policy

| Variable | Default | Description |
|---|---|---|
| `UTILISATION_SETPOINT` | `0.70` | Target utilisation — 70% leaves 30% headroom |
| `COLLAPSE_THRESHOLD` | `0.90` | Utilisation above which collapse risk is flagged |
| `EWMA_FAST_ALPHA` | `0.30` | Fast EWMA — responds in ~3 ticks |
| `EWMA_SLOW_ALPHA` | `0.10` | Slow EWMA — trend signal, responds in ~10 ticks |
| `PID_KP` | `-1.5` | PID proportional gain |
| `PID_KI` | `-0.3` | PID integral gain |
| `PID_KD` | `-0.1` | PID derivative gain |

### Simulation

| Variable | Default | Description |
|---|---|---|
| `SIM_BUDGET` | `45ms` | Wall-clock budget per tick for Monte Carlo sim |
| `SIM_HORIZON_MS` | `60000` | Simulation lookahead — 60 seconds |
| `SIM_SHOCK_FACTOR` | `2.0` | Worst-case load multiplier in simulation |

### Persistence

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | `` | Postgres DSN — if empty, runs in-memory |
| `PERSIST_INTERVAL` | `30s` | How often snapshots flush to DB |

---

## License

See `LICENSE` for terms.