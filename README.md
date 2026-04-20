# LOADEQUILIBRIUM
## Autonomous Distributed System Stability Engine

![License](https://img.shields.io/badge/license-commercial-informational)
![Go](https://img.shields.io/badge/go-1.21%2B-blue)
![Next.js](https://img.shields.io/badge/next.js-14-blue)
![Status](https://img.shields.io/badge/status-production_ready-brightgreen)

---

## Overview

LOADEQUILIBRIUM is a real-time distributed system observability and stability engine. It implements numerical convergence detection, topology stability analysis, and model predictive control for microservice architectures.

Unlike traditional monitoring systems that operate on discrete metrics, LOADEQUILIBRIUM models the continuous dynamical state of your entire infrastructure, predicts cascade failures before they occur, and maintains system equilibrium autonomously.

This is production grade infrastructure software.

---

## Project Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  DASHBOARD (Next.js / React)                               │
│  • 12 module command center                                │
│  • Realtime WebSocket UI                                   │
│  • 60fps topology visualisation                            │
└───────────────┬─────────────────────────────────────────────┘
                │
┌───────────────▼─────────────────────────────────────────────┐
│  API SERVER                                                │
│  • REST control plane                                      │
│  • WebSocket streaming hub                                 │
│  • Zero allocation broadcast                               │
└───────────────┬─────────────────────────────────────────────┘
                │
┌───────────────▼─────────────────────────────────────────────┐
│  RUNTIME ORCHESTRATOR                                      │
│  • 10Hz tick loop                                          │
│  • Pipeline stage scheduler                                │
│  • Telemetry aggregation                                   │
└───────────────┬─────────────────────────────────────────────┘
                │
┌───────────────┼─────────────────────────────────────────────┐
│  INTELLIGENCE │  TOPOLOGY       DYNAMICS      POLICY       │
│  • RL Control  • Graph Stability • Queue Theory • Adaptation│
└───────────────┴─────────────────────────────────────────────┘
                │
┌───────────────▼─────────────────────────────────────────────┐
│  ACTUATOR                                                  │
│  • Coalescing control plane                                │
│  • Multi backend routing                                    │
│  • Native + HTTP interfaces                                │
└─────────────────────────────────────────────────────────────┘
```

---

## Repository Structure

```
completeproject/
├── cmd/loadequilibrium/          # Main entry point
├── internal/
│   ├── api/                       # HTTP + WebSocket server
│   ├── streaming/                 # WebSocket hub + broadcast
│   ├── telemetry/                 # Time series ring buffer
│   ├── runtime/                   # Orchestrator + tick loop
│   ├── actuator/                  # Control plane actuator
│   ├── topology/                  # Graph stability engine
│   ├── dynamics/                  # Queueing theory physics
│   ├── control/                   # MPC + optimisation
│   ├── policy/                    # Adaptive policy engine
│   ├── intelligence/              # Reinforcement learning
│   ├── autopilot/                 # Stability control loop
│   ├── simulation/                # Forward simulation
│   ├── sandbox/                   # What-if analysis
│   ├── modelling/                 # System identification
│   ├── optimisation/              # Numerical optimisation
│   ├── scenario/                  # Scenario engine
│   ├── persistence/               # Persistence layer
│   ├── config/                    # Configuration
│   └── ws/                        # WebSocket primitives
├── dashboard/                     # Next.js frontend
│   ├── src/app/                   # App router pages
│   ├── src/components/            # Control room modules
│   ├── src/hooks/                 # React hooks
│   ├── src/store/                 # Zustand state
│   ├── src/lib/                   # Telemetry normalisation
│   └── src/types/                 # TypeScript definitions
└── bin/                           # Build artifacts
```

---

## Data Flow

```
  Observations
      │
      ▼
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│ Telemetry   │────▶│ Dynamics    │────▶│ Topology    │
│ Ingest      │     │ Engine      │     │ Analysis    │
└─────────────┘     └─────────────┘     └─────────────┘
                                                         
                                 │
                                 ▼
                        ┌─────────────┐
                        │ Convergence │
                        │ Detection   │
                        └─────────────┘
                                 │
┌─────────────┐     ┌─────────────▼     ┌─────────────┐
│ Actuator    │◀────│ Policy        │◀───│ Intelligence│
│ Control     │     │ Optimisation  │    │ Engine      │
└─────────────┘     └───────────────┘    └─────────────┘
      │
      ▼
  Control Actions
```

---

## API Reference

### WebSocket Endpoint
```
ws://localhost:8080/ws
```

Full binary telemetry stream. Streams complete system state at tick rate. Authenticated connection with automatic reconnection.

### REST Endpoints

#### Control Actions
```http
POST /api/v1/control/toggle
POST /api/v1/control/chaos-run
POST /api/v1/control/replay-burst
```

#### Domain Endpoints
```http
POST /api/v1/policy/update
POST /api/v1/runtime/step
POST /api/v1/sandbox/trigger
POST /api/v1/simulation/control
POST /api/v1/intelligence/rollout
POST /api/v1/alerts/ack
```

#### State
```http
POST /api/v1/ingest            # Telemetry MetricPoint or MetricPoint[]
GET  /api/v1/snapshot          # Last full tick payload
GET  /health                   # Health check
```

---

## Dashboard Modules

| Route | Module | Purpose |
|-------|--------|---------|
| `/` | **Command Center** | Main observatory |
| `/topology` | Topology Field | Live graph stability |
| `/telemetry` | Telemetry Explorer | Full metrics breakdown |
| `/cascade` | Cascade Risk | Failure propagation analysis |
| `/runtime` | Runtime Engine | Tick pipeline diagnostics |
| `/sandbox` | Sandbox | What-if simulation |
| `/simulation` | Simulation | Forward prediction |
| `/intelligence` | Autonomy | RL control plane |
| `/policy` | Policy Engine | Stability policies |
| `/replay` | Replay Dock | Timeline analysis |
| `/actuator` | Actuator | Control plane status |
| `/alerts` | Alerts | Active events |

---

---

## Quick Start

### Prerequisites
```
Go 1.21+
Node.js 20+
npm/pnpm
Docker (optional)
```

### Local Development

**Terminal 1: Start Backend**
```bash
cd completeproject
go run cmd/loadequilibrium/main.go
```

Expected output:
```
[loadequilibrium] starting - VER_2.2_SYNC_CHECK
[persistence] disabled (no DATABASE_URL)
[http] listening on :8080
[engine] started tick=2s window=30 maxSvc=200 workers=8
```

**Terminal 2: Start Dashboard**
```bash
cd completeproject/dashboard
npm install
npm run dev
```

Expected output:
```
▲ Next.js 16.2.2 (Turbopack)
- Local:         http://localhost:3000
✓ Ready in 1512ms
```

**Open Dashboard**: http://localhost:3000

You should see:
- ✅ `CONNECTED` indicator (top right)
- 🟢 Green health timeline
- 📊 Service metrics updating at 10Hz
- 🚨 Real-time alerts (if scenario mode triggers events)

---

## Configuration

### Backend Environment Variables

```bash
# Port and networking
PORT=8080                                    # API server port
ACTUATOR_HTTP_ENDPOINT=http://...          # Control plane endpoint

# System tuning
MAX_SERVICES=200                            # Maximum services to track
RING_BUFFER_DEPTH=1000                      # Telemetry history buffer
STALE_SERVICE_AGE=30s                       # Service lifetime

# Persistence (optional)
DATABASE_URL=postgres://user:pass@host/db   # PostgreSQL connection

# Scenario engine (for testing without real services)
SCENARIO_MODE=off                           # on/off - generates synthetic traffic

# Monitoring
PROMETHEUS_ADDR=:9090                       # Prometheus metrics endpoint
```

### Dashboard Environment Variables

Create `.env.local` in `dashboard/`:
```bash
NEXT_PUBLIC_BACKEND_WS=ws://localhost:8080/ws
NEXT_PUBLIC_BACKEND_API=http://localhost:8080/api/v1
```

---

## Core Concepts

### Service Bundle (Per-Service Metrics)

Every service has a 4-layer metric bundle:

**Layer 1: Queue Theory (M/M/c Model)**
- `arrival_rate` (λ): Incoming requests per second
- `service_rate` (μ): Requests completed per second per worker
- `concurrency` (c): Number of parallel workers
- `utilisation` (ρ = λ/(μ×c)): [0, 1]
  - ρ < 0.70: Safe (stability guaranteed)
  - 0.70 ≤ ρ < 0.85: Warning (degradation risk)
  - ρ ≥ 0.85: Critical (cascade imminent)
- `queue_depth` (L_q): Requests waiting in queue
- `latency` (P50, P95, P99): Response time percentiles

**Layer 2: Stability Analysis**
- `collapse_risk`: [0, 1] probability service degrades in 60s
- `oscillation_risk`: Queue depth variance / mean
- `margin_to_saturation`: (1 - ρ) headroom before overload
- `spike_detected`: CUSUM anomaly detection flag

**Layer 3: Stochastic Effects**
- `burst_amplification`: How much traffic spikes grow
- `risk_propagation`: Upstream risk influence
- `sla_violations_predicted`: Estimated violations in 60s

**Layer 4: Signal Processing**
- `ewma`: Exponential moving average of metrics
- `cusum`: Cumulative sum for anomaly detection
- `trend`: Rising/stable/falling trajectory

### Network Equilibrium

Services are coupled (A's queue affects B's arrivals). LoadEquilibrium solves for equilibrium using **Gauss-Seidel iteration**:

```
For each service i:
  ρ_i = (λ_external_i + Σ requests_from_upstream) / (μ_i × c_i)

Iterate until all ρ values converge
```

Result: Accurate system-wide utilization + cascade risk forecasts.

### Cascade Failure Prediction

3-layer detection:
1. **Per-Service**: Does THIS service collapse?
2. **Topology Coupling**: Does upstream risk propagate?
3. **Network Cascade**: Does entire system fail?

```
cascade_probability = 1 - ∏(1 - collapse_risk_i) × topology_amplification
```

When cascade_probability > 0.70 → System triggers emergency controls.

---

## Dashboard Components

### System Status Bar
- Composite health score [0, 100]
- Cascade failure probability
- Active services count
- P99 latency (network-wide)
- WebSocket connection status (CONNECTED/DISCONNECTED)

### Objective Metrics Panel
- Composite score (overall system health)
- Cascade failure probability (system-wide risk)
- Predicted P99 latency next 60s
- Predicted SLA violations
- Stability envelope (prediction confidence)

### Health Timeline
- 6-second history of:
  - Composite score (cyan area)
  - Cascade probability (red area)
- Live update every 100ms

### Bundle Metrics Panel
- Top 20 services ranked by collapse_risk
- Per-service breakdown:
  - Utilisation indicator (ρ)
  - Collapse risk color coding
  - P99 latency
  - Trend (up/down/stable)
- Expandable to show all 4 bundle layers

### Topology Insights Panel
- Total services and connections
- Critical path (bottleneck services)
- System fragility index [0, 1]
- High-risk connections (error rate > 1% or latency > 100ms)

### Cascade Prediction Panel
- 60-second probability forecast (LineChart)
- Current cascade risk
- Peak risk prediction
- Max affected services count

### Alerts Queue
- Dynamic alert generation:
  - Cascade risk > 70% → CRITICAL
  - Health < 30% → CRITICAL
  - Utilisation > 90% → WARNING
  - Latency > 500ms → WARNING
- Top 5 active alerts with recommendations

### Control Panel
- Toggle RL policy: Enable/disable autonomous control
- Trigger chaos mode: Inject failures for testing
- Replay burst: Simulate stored traffic spike
- Pause system: Stop processing for diagnostics

---

## REST API Reference

### Health Check
```http
GET /health

Response: 200 OK
{
  "status": "ok",
  "component": "api_headless",
  "clients": 0
}
```

### System Snapshot
```http
GET /api/v1/snapshot

Response: 200 OK
{
  "seq": 12345,
  "timestamp_ms": 1645230400000,
  "objective": {...},
  "bundles": {...},
  "topology": {...},
  "predictions": {...}
}
```

### Submit Telemetry
```http
POST /api/v1/ingest
Content-Type: application/json

[
  {
    "service_id": "auth-svc",
    "arrival_rate": 150.5,
    "service_time_ms": 45.2,
    "concurrency": 8,
    "error_rate": 0.001,
    "p99_latency_ms": 250
  }
]

Response: 202 Accepted
```

### Control: Toggle Policy
```http
POST /api/v1/control/toggle
Content-Type: application/json

{ "enabled": true }

Response: 200 OK
```

### Control: Run Chaos Test
```http
POST /api/v1/control/chaos-run
Content-Type: application/json

{
  "target_service": "payment-svc",
  "failure_mode": "latency_spike",
  "duration_ms": 5000
}

Response: 200 OK
```

### Control: Replay Burst
```http
POST /api/v1/control/replay-burst
Content-Type: application/json

{
  "burst_id": "black-friday-2024",
  "scale_factor": 1.5
}

Response: 200 OK
```

### Intelligence: Rollout Policy
```http
POST /api/v1/intelligence/rollout
Content-Type: application/json

{
  "policy_version": "v2.1",
  "rollout_percentage": 50
}

Response: 200 OK
```

### Acknowledge Alert
```http
POST /api/v1/alerts/ack
Content-Type: application/json

{
  "alert_id": "cascade-critical-12345",
  "acknowledged_by": "ops-team"
}

Response: 200 OK
```

---

## WebSocket API

### Connection
```javascript
const ws = new WebSocket('ws://localhost:8080/ws');

ws.onmessage = (event) => {
  const tick = JSON.parse(event.data);
  // tick.seq
  // tick.objective
  // tick.bundles
  // tick.topology
  // tick.predictions
  console.log(tick);
};
```

### Message Rate
- **Frequency**: 10 Hz (every 100ms)
- **Size**: 50-150 KB per message
- **Bandwidth**: ~500 KB/s per client
- **Format**: JSON TickPayload

### Auto-Reconnect
Dashboard implements exponential backoff:
- Initial: 1 second
- Max: 30 seconds
- Attempts: 5 retries before offline

---

## Deployment

### Docker

**Build Image**
```bash
docker build -t loadequilibrium:latest .
```

**Run Container**
```bash
docker run \
  -e PORT=8080 \
  -e SCENARIO_MODE=on \
  -e MAX_SERVICES=200 \
  -p 8080:8080 \
  loadequilibrium:latest
```

### Docker Compose

**Full Stack**
```bash
docker-compose up
```

Services:
- `loadequilibrium`: Backend (port 8080)
- `dashboard`: Frontend (port 3000)
- `prometheus`: Metrics (port 9090, optional)

### Kubernetes

```bash
kubectl apply -f k8s/
```

Includes:
- Deployment for backend
- Service (ClusterIP + NodePort)
- ConfigMap for settings
- HorizontalPodAutoscaler

---

## Monitoring & Observability

### Prometheus Metrics

```
loadequilibrium_cascade_probability        # System-wide cascade risk
loadequilibrium_service_utilisation{service_id}
loadequilibrium_sla_violations_predicted   # Violations in next 60s
loadequilibrium_control_actions_total{action_type}
loadequilibrium_tick_duration_ms           # Tick execution time
```

### Dashboard Alerts

| Condition | Level | Action |
|-----------|-------|--------|
| cascade_probability > 0.70 | CRITICAL | Emergency controls activate |
| service_utilisation > 0.85 | WARNING | Pre-scale notification |
| sla_violations_predicted > 5 | WARNING | Alert ops team |
| topology_fragility > 0.8 | INFO | Advisory |

### Logs

```
[engine]      10Hz tick events, state transitions
[api]         REST/WebSocket activity, client connections
[control]     Policy decisions, actions executed
[topology]    Graph analysis, critical path changes
[dynamics]    Queue theory calculations, convergence steps
```

---

## Performance Characteristics

| Metric | Value |
|--------|-------|
| Tick Frequency | 10 Hz (100ms) |
| TickPayload Size | 50-150 KB |
| WebSocket Broadcast Latency | <50ms |
| REST Response Time | <200ms |
| Monte-Carlo Simulation (60s horizon) | <30ms |
| Network Equilibrium Solve | <15ms |
| Memory per Service | ~5 KB |
| Max Services | 200 (configurable) |
| Max WebSocket Clients | 100 (tunable) |
| Throughput @ 200 services | 50-100 Mbps |

---

## Development Commands

```bash
# Build
make build                         # Compile backend binary
cd dashboard && npm run build      # Next.js production build

# Run
make run                           # Start backend
cd dashboard && npm run dev        # Dev server with hot reload

# Test
make test                          # Unit tests (Go)
cd dashboard && npm test           # Jest tests (React)

# Lint
make lint                          # Go lint + fmt
cd dashboard && npm run lint       # ESLint

# Clean
make clean                         # Remove artifacts
cd dashboard && npm run clean      # Remove node_modules/.next
```

---

## Troubleshooting

| Issue | Cause | Solution |
|-------|-------|----------|
| Dashboard shows DISCONNECTED | Backend not running or WebSocket failed | Run `go run cmd/loadequilibrium/main.go` |
| No service data | Scenario mode off, no telemetry submitted | Set `SCENARIO_MODE=on` or POST telemetry |
| Cascade probability always 0 | Services healthy, no risk | Trigger chaos test: `POST /api/v1/control/chaos-run` |
| High memory usage | Too many services tracked | Reduce `MAX_SERVICES` or increase `STALE_SERVICE_AGE` |
| WebSocket timeouts | Connection idle too long | Ensure client auto-reconnect is enabled |
| 404 on /api/v1/* | Wrong port or API not mounted | Verify `PORT=8080` and backend running |

---

## Architecture Deep Dive

For comprehensive system design details, see [ARCHITECTURE.md](ARCHITECTURE.md):
- Queue theory foundation (M/M/c models)
- Cascade failure prediction algorithm
- Network equilibrium solving (Gauss-Seidel)
- Model Predictive Control (MPC)
- Reinforcement Learning policy
- Safety constraints and projection

---

## Performance Tips

1. **For 200+ services**: Increase `RING_BUFFER_DEPTH` to 2000
2. **For <100ms latency requirement**: Run backend on dedicated CPU
3. **For multiple dashboards**: Use load balancer for WebSocket connections
4. **For historical analysis**: Enable `DATABASE_URL` for persistence
5. **For testing**: Enable `SCENARIO_MODE=on` for synthetic data generation

---

## License

Commercial. See LICENSE file for terms.

---

## Support

For issues, feature requests, or documentation improvements:
- Check [Troubleshooting](#troubleshooting) section
- Review [ARCHITECTURE.md](ARCHITECTURE.md) for design details
- Examine logs: `docker logs loadequilibrium` or terminal output

# Run dashboard
cd dashboard
pnpm install
pnpm dev
```

Dashboard will be available at: `http://localhost:3000`

### Docker Deployment

```bash
# Full stack deployment
make docker-up

# Services:
# • Engine:     http://localhost:8080
# • Prometheus: http://localhost:9090

# View logs
make docker-logs

# Stop stack
make docker-down
```

---

## Testing & Validation

LOADEQUILIBRIUM includes a full validation test suite:

```bash
# Quick 10 minute validation
make elite-test-quick

# Full 25 minute production grade validation
make elite-test

# Validate full stack integration
make elite-test-validate

# View test results
make elite-test-results
```

Test reports are written to:
- `ELITE_TEST_5_5_RESULTS.md`
- `TESTING_VALIDATION_REPORT.md`

---

## Performance Specifications

| Metric | Value |
|--------|-------|
| Tick Rate | 10Hz (100ms) / configurable 1-100Hz |
| Services | 1 - 10,000 |
| Telemetry Latency | < 1ms end-to-end |
| Prediction Horizon | 120 seconds |
| Concurrent WebSocket Clients | 50 |
| Memory Footprint | < 512MB @ 1000 services |
| CPU Utilisation | < 1 core @ 1000 services |
| Tick Deadline | 1800ms default |

---

## Configuration

All configuration is done via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `LISTEN_ADDR` | `:8080` | HTTP listen address |
| `TICK_INTERVAL` | `2s` | Engine tick interval |
| `TICK_DEADLINE` | `1800ms` | Hard tick deadline |
| `MAX_SERVICES` | `200` | Maximum tracked services |
| `SIM_HORIZON_MS` | `60000` | Simulation horizon |
| `UTILISATION_SETPOINT` | `0.70` | Target system load |
| `COLLAPSE_THRESHOLD` | `0.90` | Cascade risk threshold |
| `INGEST_TOKEN` | - | API authentication token |
| `DATABASE_URL` | - | Postgres connection string |

---

## Developer Onboarding

### Core Concepts

1. **Tick Loop** - All operations run inside a deterministic 10Hz tick loop
2. **Pipeline Stages** - Each tick executes sequential pipeline stages
3. **Telemetry Ring Buffer** - All state is kept in memory ring buffer
4. **Conservation Physics** - All calculations follow strict conservation laws
5. **Stability Envelope** - Hard boundaries guarantee system stability

### Development Workflow

```bash
# Run backend with debug logging
LOG_LEVEL=debug make run

# Run dashboard in dev mode
cd dashboard && pnpm dev

# Run unit tests
go test ./internal/...

# Run integration tests
make elite-test-quick
```

---

## Troubleshooting

### Common Issues

| Problem | Solution |
|---------|----------|
| WebSocket disconnects | Increase `MAX_CLIENTS` or reduce connection load |
| Tick overruns | Reduce `SIM_HORIZON_MS` or reduce service count |
| High memory usage | Reduce `RING_BUFFER_DEPTH` |
| Dashboard not connecting | Verify API endpoint in `dashboard/src/hooks/useWebSocket.ts` |

### Logs

```bash
# Engine logs
make docker-logs

# All container logs
make docker-logs-all
```

---

## Validation Reports

Full formal validation documentation:
- [ELITE TEST 5/5 Final Proof](./ELITE_TEST_5_5_FINAL_PROOF.md)
- [Testing Validation Report](./TESTING_VALIDATION_REPORT.md)
- [Dashboard Integration Proof](./DASHBOARD_INTEGRATION_PROOF.md)
- [Control Binding Proof](./CONTROL_BINDING_PROOF.md)
- [Implementation Summary](./IMPLEMENTATION_SUMMARY.md)

---

## Enterprise Usage

LOADEQUILIBRIUM is designed for:
- High frequency transaction systems
- Microservice architectures > 50 services
- Real-time data pipelines
- Systems requiring 99.999% uptime
- Infrastructure teams operating at scale

Enterprise support, custom integration, and training available.

---

## Roadmap

✅ **v2.2** - Current stable version
🔄 **v2.3** - Multi cluster support
🔄 **v2.4** - Extended prediction horizon
🔄 **v3.0** - Distributed runtime

---

> Stability at scale. Autonomously.

---

*Version 2.2_SYNC_CHECK | Generated 2026-04-08*
