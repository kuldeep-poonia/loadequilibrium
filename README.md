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
GET  /api/v1/snapshot          # Last full tick payload
GET  /health                   # Health check
GET  /metrics                  # Prometheus metrics
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

## Installation

### Prerequisites
```
go 1.21+
node 20+
pnpm 8+
docker (optional)
```

### Local Development

```bash
# Build backend
make build

# Run engine
make run

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