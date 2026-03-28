# LoadEquilibrium

status :- in progress not completed yet 

LoadEquilibrium is a simulation-driven decision engine for backend systems. It ingests live service telemetry, reconstructs operational state, projects system behavior forward, estimates risk, and produces control recommendations or actuator outputs before overload becomes visible in standard dashboards.

This repository should be understood as a deterministic predictive control system packaged as a Dockerized sidecar service.

It is not:

- machine learning
- anomaly detection by pattern matching
- a general-purpose AI platform

It is:

- model-based
- control-oriented
- simulation-driven
- built around queue physics, topology coupling, and MPC-style optimization

## Contents

1. [Project Overview](#project-overview)
2. [Core Concept](#core-concept)
3. [System Architecture](#system-architecture)
4. [Key Features](#key-features)
5. [How It Works (Deep Dive)](#how-it-works-deep-dive)
6. [Real-World Use Cases](#real-world-use-cases)
7. [Installation (Docker)](#installation-docker)
8. [Configuration](#configuration)
9. [API Reference](#api-reference)
10. [Dashboard](#dashboard)
11. [Example Workflow](#example-workflow)
12. [Limitations](#limitations)
13. [Roadmap](#roadmap)
14. [Contributing](#contributing)
15. [License](#license)

## Project Overview

### What LoadEquilibrium Is

LoadEquilibrium is a runtime control companion for distributed backend systems. It runs beside an application or platform stack, consumes operational telemetry, models service pressure and dependency effects, and decides whether the system is moving toward a stable or unstable operating region.

The system is designed for environments where raw CPU, error rate, and tail latency are not enough to answer the operational question that matters:

"What is likely to happen next, and what action should be taken before the system degrades further?"

### Why It Exists

Conventional monitoring is mostly retrospective. A typical monitoring stack shows:

- what the current metrics are
- whether thresholds have been crossed
- which services are already unhealthy

That approach is necessary, but it is often too late for systems dominated by:

- queue growth
- retry amplification
- topology-driven backpressure
- nonlinear saturation
- delayed actuation effects

LoadEquilibrium exists to work one step earlier in the chain. Instead of waiting for visible failure, it estimates pressure build-up, simulates likely trajectories, evaluates stability, and derives bounded control actions.

### Problem It Solves

The project is aimed at the class of problems where backend systems fail gradually before they fail obviously:

- request queues grow before CPU reaches its absolute ceiling
- one dependency under stress destabilizes several neighbors
- retry policy changes can worsen overload instead of helping
- a scale decision helps only after warmup lag, so late action is expensive

LoadEquilibrium addresses those problems by treating a backend as a control system rather than as a collection of unrelated service metrics.

## Core Concept

### Simulation-Driven Decision Making

The central idea is simple:

1. Observe the system as it is now.
2. Infer the hidden state that matters operationally.
3. Project forward under current conditions and candidate interventions.
4. Compare the likely outcomes.
5. Select a bounded action or recommendation.

The project does not search for patterns in historical labels. It does not train a model to classify incidents. It uses explicit models and deterministic calculations with bounded stochastic simulation where uncertainty needs to be explored.

### Predictive Control

LoadEquilibrium uses predictive control ideas rather than threshold-only automation. In practice that means:

- the system keeps a short-horizon view of future load and backlog
- control actions are evaluated as trajectories, not one-off toggles
- the cost of action is balanced against risk, latency, backlog, and stability
- safety constraints remain in the loop

The result is closer to operational steering than to static alerting.

### Difference From Traditional Monitoring

| Traditional Monitoring | LoadEquilibrium |
| --- | --- |
| Reports current metric values | Projects future system trajectories |
| Primarily threshold-based | Primarily model-based |
| Often service-local | Explicitly topology-aware |
| Alerts after symptoms emerge | Seeks intervention before visible degradation |
| Optimized for human inspection | Designed for inspection plus decision support and actuation |

### Difference From AI/ML Systems

The repository contains packages with names such as `intelligence` and `autopilot`, but the system should still be understood as deterministic engineering software. The control path is not based on trained ML models.

The core logic comes from:

- queueing theory
- control logic
- bounded predictive search
- explicit risk and safety heuristics
- topology analysis
- Monte Carlo style simulation for uncertainty exploration

This distinction matters for operations, compliance, and expectations. You tune this system like an engineered controller, not like a black-box model.

## System Architecture

### Runtime Pipeline

The runtime is a tick-driven engine. Each tick processes new telemetry, updates models, evaluates risk, and emits outputs.

```text
POST /api/v1/ingest
    -> telemetry.Store
        -> windowing and confidence scoring
        -> topology reconstruction
        -> queue and stability modelling
        -> optimization and control fusion
        -> simulation and reasoning
        -> dashboard streaming
        -> optional persistence
        -> actuator execution
```

The orchestration flow in the codebase is implemented as a staged pipeline:

```text
Ingest
  -> Telemetry Store
  -> Stage 1: Prune stale services
  -> Stage 2: Build service windows and confidence
  -> Stage 3: Build topology and coupled equilibrium state
  -> Stage 4: Run queue, stochastic, signal, and stability models
  -> Stage 5: Compute optimization directives
  -> Stage 5b: Apply phase runtime control fusion
  -> Stage 6: Run asynchronous simulation
  -> Stage 7: Generate reasoning events
  -> Stage 8: Broadcast state to dashboard
  -> Stage 9: Persist snapshots when enabled
  -> Actuator: dispatch directives to configured backend
```

### Stage-by-Stage Explanation

| Stage | Purpose | Main Output |
| --- | --- | --- |
| Ingest | Accept telemetry points from services or collectors | `MetricPoint` records |
| Telemetry Store | Maintain bounded per-service history, freshness, and confidence | `ServiceWindow` maps |
| Topology | Build dependency graph and sensitivity context | graph snapshot and coupling inputs |
| Modelling | Estimate queue pressure, stochastic burst characteristics, and stability state | service model bundles |
| Optimization | Compute direct control directives from current modeled state | scale and utilization directives |
| Phase Runtime | Fuse policy, sandbox, autopilot, and safety layers into final control output | merged directives |
| Simulation | Explore future trajectories under the current modeled system | collapse and SLA risk overlays |
| Reasoning | Convert raw model states into operator-facing events | reasoning event stream |
| Streaming | Publish full tick state to WebSocket clients and dashboard | `TickPayload` |
| Persistence | Store snapshots when a database is configured | durable runtime history |
| Actuation | Route directives to queue, HTTP, or fallback backends | external or simulated action result |

### Main Subsystems

| Subsystem | Directory | Responsibility |
| --- | --- | --- |
| Runtime engine | `internal/runtime` | orchestrates the tick loop |
| Telemetry ingestion | `internal/telemetry` | stores metric history and computes windows |
| Modelling | `internal/modelling` | queue physics, coupling, equilibrium, and stability analysis |
| Optimization | `internal/optimisation` | computes directive candidates and objective scores |
| Autopilot | `internal/autopilot` | MPC-style horizon control, rollout, and safety logic |
| Scenario engine | `internal/scenario` | optional synthetic disturbances for testing |
| Simulation | `internal/simulation` | budget-bounded predictive simulation |
| Reasoning | `internal/reasoning` | operator-facing event generation |
| Streaming | `internal/streaming` | live state transport schema and hub |
| Dashboard | `internal/dashboard` | HTTP UI, ingest endpoint, WebSocket wiring |
| Actuator | `internal/actuator` | directive coalescing and backend execution |
| Persistence | `internal/persistence` | optional PostgreSQL snapshot writer |

### Deployment Model

LoadEquilibrium is intended to run as a sidecar-style service or colocated control service. A common deployment model is:

```text
application / telemetry collector
    -> LoadEquilibrium sidecar
        -> local dashboard on :8080
        -> websocket stream
        -> optional persistence
        -> actuator backend or external control endpoint
```

This arrangement keeps observation, prediction, and response close to the workload being supervised.

### Repository Layout

```text
cmd/loadequilibrium              main executable
internal/runtime                 tick engine and phase runtime
internal/telemetry               metric ingestion and windowing
internal/modelling               queue, stability, topology, coupling
internal/optimisation            control objective and directives
internal/autopilot               MPC-style runtime control logic
internal/intelligence            deterministic high-level decision modules
internal/scenario                optional disturbance injection
internal/simulation              predictive simulation runner
internal/reasoning               operator-facing event generation
internal/dashboard               HTTP API, dashboard, WebSocket endpoint
internal/streaming               live payload schema and hub
internal/actuator                coalescer, router, backend interface
internal/actuator/backends       queue and HTTP actuator backends
internal/persistence             snapshot persistence
docker-compose.yml               local stack definition
Dockerfile                       runtime container build
```

## Key Features

LoadEquilibrium provides the following core capabilities:

- Predictive simulation of service behavior under current load and candidate actions.
- Queue modelling based on service rate, concurrency, backlog, and utilization.
- Topology-aware coupling and equilibrium analysis across service dependencies.
- Stability and collapse risk estimation rather than threshold-only alerting.
- MPC-style short horizon optimization for capacity and control decisions.
- Real-time dashboard streaming over WebSocket.
- Optional snapshot persistence to PostgreSQL.
- Built-in actuator framework with queue, HTTP, and routed backend support.
- Authenticated ingest path for production use.
- Scenario injection support for controlled testing, disabled by default for production safety.

### Actuation Features

The current actuator layer has three concrete pieces:

- `QueueBackend`
  - Maintains an in-memory worker count per service.
  - Applies `round(current_workers * scale_factor)`.
  - Enforces a minimum of one worker.
  - Useful for safe local validation and deterministic backend simulation.
- `HTTPBackend`
  - Sends synchronous `POST` requests to an external control endpoint.
  - Uses the runtime execution context and timeout provided by the coalescing actuator.
  - Emits `service_id` and `scale_factor` in JSON.
- `RouterBackend`
  - Routes service-specific directives to different backends.
  - Uses a default backend when no explicit route exists.
  - Falls back to `LogOnlyBackend` if no concrete backend is available.

## How It Works (Deep Dive)

### Telemetry Windowing and Confidence

LoadEquilibrium ingests `MetricPoint` records and stores them in bounded ring buffers keyed by service ID. From those buffers it builds `ServiceWindow` views that contain:

- mean and last request rate
- latency summaries
- queue and connection estimates
- upstream call aggregation
- confidence score
- signal quality class

The confidence score matters because the runtime does not assume all telemetry is equally trustworthy. Confidence is reduced when:

- the sample count is low
- rate variance is high
- the most recent data is old

This lets the system degrade predictions rather than pretending uncertain inputs are precise.

### Queue Modelling

The queue modelling layer treats services as queueing systems rather than simple CPU meters. The important quantities are:

- arrival rate `lambda`
- service rate `mu`
- effective concurrency `c`
- utilization `rho`

Conceptually, the model works in the same family as M/M/c queue analysis:

- arrival pressure increases waiting time nonlinearly near saturation
- concurrency changes service capacity
- backlog and latency reflect queue state, not just host utilization

This gives the controller more useful information than a pure threshold on CPU or latency percentiles.

### Topology Coupling and Equilibrium

Services do not fail independently. A pressure spike in one service can amplify another service's load or delay. LoadEquilibrium therefore builds a dependency graph from upstream call data and computes:

- network coupling
- path saturation risk
- equilibrium utilization
- topology sensitivity
- keystone services

This makes it possible to distinguish:

- a locally overloaded service
- a service that appears healthy but is structurally exposed
- a graph region that is converging toward instability

### Predictive Simulation

The simulation layer runs budget-bounded predictive experiments. It is not a generic physics engine; it is a constrained operational simulator used to answer questions such as:

- If the current trend continues, which services are likely to saturate?
- What is the likely queue distribution at the horizon?
- What is the estimated SLA violation probability?
- How fragile is recovery under the current topology?

The simulation results are exposed in the dashboard and also contribute to the broader risk picture used by the decision engine.

### MPC-Style Optimization

The control layer uses short-horizon optimization rather than a one-shot heuristic. In broad terms:

1. A control sequence is warm-started from the prior state.
2. Candidate control trajectories are evaluated over a prediction horizon.
3. Cost includes backlog, latency, variance, scaling effort, smoothness, and safety barrier terms.
4. Safety logic can override or tighten actions when risk is too high.

The implementation is "MPC-style" rather than a textbook industrial MPC package. The important property is that the system evaluates future action paths under explicit cost and constraint logic.

### Control Fusion

The final directive is not produced by a single scalar heuristic. It is the result of model-based fusion across:

- optimization engine output
- policy layer output
- autopilot / MPC runtime
- sandbox recommendation metadata
- safety constraints

This allows the system to stay interpretable while still combining multiple model-derived signals.

### Control Loop Timing

By default, the system runs on a 2 second tick interval with an 1800 ms deadline. The engine tracks:

- per-stage latency
- predicted stage cost
- jitter
- safety escalation level
- adaptive interval stretching under stress

This is important operationally because the controller itself must remain stable under pressure. A control system that misses its own cadence is not trustworthy.

### Actuation Path

Directive execution happens through the coalescing actuator:

1. Directives are coalesced per service so stale duplicates are not executed.
2. Each execution is wrapped in a context timeout.
3. Success or failure is returned as feedback into the runtime.
4. Errors do not crash the engine.

The current main program wires:

- `QueueBackend` as the default backend
- `HTTPBackend` for explicitly routed services when configured through environment variables
- `RouterBackend` to decide which backend handles which service

This keeps the runtime generic and lets platform-specific actuation be added without changing the orchestrator.

## Real-World Use Cases

### Backend Load Systems

A service platform can use LoadEquilibrium to track arrival pressure, queue growth, and dependency-induced overload before SLO breach becomes obvious.

### Queue Management

Systems with explicit worker pools, consumers, or internal processing queues can use the queue model and worker-scaling actuator to reason about backlog growth and workforce sizing.

### Infrastructure Optimization

The project can be used as a decision support layer for scaling, brownout, retry, or rate-limiting decisions where naive autoscaling rules are too slow or too coarse.

### Planning Systems

Because the runtime can simulate future behavior, it is also useful for:

- change planning
- stress exercises
- capacity reviews
- controlled fault rehearsal

### Sidecar Control for Platform Teams

A platform team can deploy LoadEquilibrium beside a service or gateway component and use it as:

- a local predictive control service
- a risk-aware recommendation engine
- a safe pre-actuation validation layer

## Installation (Docker)

### Prerequisites

- Docker
- Docker Compose plugin

The repository targets Go 1.22, but Docker is the recommended starting point because it reproduces the intended runtime shape with the dashboard and persistence dependencies.

### Quick Start With Docker Compose

From the project root:

```bash
docker compose up --build
```

This starts:

- `loadequilibrium` on port `8080`
- `postgres` for local snapshot persistence

Open:

- dashboard: `http://localhost:8080/`
- health endpoint: `http://localhost:8080/healthz`

### What the Compose Stack Configures

The supplied `docker-compose.yml` sets:

- tick interval and deadline
- simulation budget
- local PostgreSQL connection
- ingest authentication token

The bundled PostgreSQL service is configured for local compatibility with the current persistence driver by forcing MD5 authentication settings.

### Build the Image Directly

```bash
docker build -t loadequilibrium .
docker run --rm -p 8080:8080 loadequilibrium
```

### Run With Optional HTTP Actuation

If you want selected services to be actuated through an HTTP endpoint instead of the default in-memory queue backend:

```bash
docker run --rm -p 8080:8080 \
  -e INGEST_TOKEN=changeme \
  -e ACTUATOR_HTTP_ENDPOINT=http://controller:9000/scale \
  -e ACTUATOR_HTTP_SERVICES=checkout,payment \
  loadequilibrium
```

Behavior:

- services listed in `ACTUATOR_HTTP_SERVICES` are routed to `HTTPBackend`
- all other services use `QueueBackend`
- if no backend is available, the router falls back to `LogOnlyBackend`

### Local Development Without Docker

Docker is the primary installation path, but local development is straightforward:

```bash
make build
make run
```

or

```bash
go run ./cmd/loadequilibrium/
```

## Configuration

LoadEquilibrium is configured mainly through environment variables. The defaults are defined in `internal/config/config.go`, and a small number of actuator routing variables are read directly in `cmd/loadequilibrium/main.go`.

### Recommended Production Baseline

At minimum:

- set `INGEST_TOKEN` to a non-empty value
- keep `SCENARIO_MODE=off`
- point `DATABASE_URL` at a reachable PostgreSQL instance if persistence is required
- configure `ACTUATOR_HTTP_ENDPOINT` and `ACTUATOR_HTTP_SERVICES` if you want external actuation

### Interface and Security

| Variable | Default | Meaning |
| --- | --- | --- |
| `LISTEN_ADDR` | `:8080` | HTTP bind address for dashboard, ingest, health, and WebSocket endpoints |
| `INGEST_TOKEN` | empty | shared token for `/api/v1/ingest`; empty disables auth and should be used only for development |
| `MAX_STREAM_CLIENTS` | `50` | maximum concurrent WebSocket clients |

### Runtime Loop and Capacity

| Variable | Default | Meaning |
| --- | --- | --- |
| `TICK_INTERVAL` | `2s` | nominal engine cadence |
| `TICK_DEADLINE` | `1800ms` | per-tick execution budget |
| `WORKER_POOL_SIZE` | `8` | bounded concurrency for modelling work |
| `RING_BUFFER_DEPTH` | `300` | telemetry history depth per service |
| `MAX_SERVICES` | `200` | maximum number of tracked services |
| `STALE_SERVICE_AGE` | `5m` | eviction threshold for inactive services |
| `MIN_TICK_INTERVAL` | `1s` | minimum adaptive interval |
| `MAX_TICK_INTERVAL` | `10s` | maximum adaptive interval |
| `TICK_ADAPT_STEP` | `1.25` | stretch factor when the runtime is under sustained pressure |
| `SAFETY_MODE_THRESHOLD` | `3` | consecutive overruns required before safety escalation |

### Control and Optimization

| Variable | Default | Meaning |
| --- | --- | --- |
| `UTILISATION_SETPOINT` | `0.70` | target utilization for the controller |
| `COLLAPSE_THRESHOLD` | `0.90` | risk threshold used in stability logic |
| `WINDOW_FRACTION` | `0.10` | fraction of ring buffer used for the active analysis window |
| `ARRIVAL_ESTIMATOR_MODE` | `ewma` | arrival estimator mode; `median` is more burst-resistant |
| `PREDICTIVE_HORIZON_TICKS` | `5` | forward horizon used by prediction logic |
| `PID_KP` | `1.5` | proportional gain for directive generation |
| `PID_KI` | `0.3` | integral gain |
| `PID_KD` | `0.1` | derivative gain |
| `PID_DEADBAND` | `0.02` | deadband for controller stability |
| `PID_INTEGRAL_MAX` | `2.0` | integral clamp |

### Simulation and Scenario Controls

| Variable | Default | Meaning |
| --- | --- | --- |
| `SIM_BUDGET` | `45ms` | per-tick simulation time budget |
| `SIM_HORIZON_MS` | `60000` | simulation horizon in milliseconds |
| `SIM_SHOCK_FACTOR` | `2.0` | disturbance multiplier used by simulation shock logic |
| `SIM_ASYNC_BUFFER` | `4` | async result channel depth for simulation |
| `SIM_STOCHASTIC_MODE` | `exponential` | inter-arrival distribution; `pareto` is also supported |
| `SLA_LATENCY_THRESHOLD_MS` | `500.0` | latency threshold for SLA violation probability |
| `SCENARIO_MODE` | `off` | enables or disables scenario disturbance logic; keep `off` in production |

### Signal Quality and Safety

| Variable | Default | Meaning |
| --- | --- | --- |
| `EWMA_FAST_ALPHA` | `0.30` | fast smoothing parameter for signal processing |
| `EWMA_SLOW_ALPHA` | `0.10` | slow smoothing parameter |
| `SPIKE_Z_SCORE` | `3.0` | threshold for spike classification |
| `STALENESS_BYPASS_THRESHOLD` | `0.70` | staleness level beyond which deeper modelling is reduced |
| `MAX_REASONING_COOLDOWNS` | `500` | cap for reasoning cooldown state |

### Persistence

| Variable | Default | Meaning |
| --- | --- | --- |
| `DATABASE_URL` | empty | PostgreSQL connection string; when empty, persistence is disabled |
| `PERSIST_INTERVAL` | `30s` | persistence cadence setting reserved in the config surface |

### Actuation Routing

These are read directly by the main process:

| Variable | Default | Meaning |
| --- | --- | --- |
| `ACTUATOR_HTTP_ENDPOINT` | empty | external endpoint used by `HTTPBackend` |
| `ACTUATOR_HTTP_SERVICES` | empty | comma-separated list of services routed to `HTTPBackend` |

### Tuning Guidance

- Lower `UTILISATION_SETPOINT` for more conservative behavior.
- Lower `COLLAPSE_THRESHOLD` to intervene earlier.
- Increase `SIM_BUDGET` if you want richer predictive simulation and have runtime headroom.
- Increase `WORKER_POOL_SIZE` only if the host can support more concurrent modelling work.
- Keep `SCENARIO_MODE=off` for production telemetry.
- Treat low-level controller gains as advanced tuning parameters, not day-one defaults.

## API Reference

### `POST /api/v1/ingest`

Ingest one or more telemetry points into the runtime.

#### Authentication

If `INGEST_TOKEN` is non-empty, clients must provide it in one of two ways:

- `X-Ingest-Token` header
- `token` query parameter

The header form is the preferred production option.

#### Request Body

The endpoint accepts either:

- a single `MetricPoint`
- an array of `MetricPoint`

The request body is limited to 2 MB.

#### MetricPoint Fields

| Field | Type | Meaning |
| --- | --- | --- |
| `service_id` | string | logical service name |
| `timestamp` | RFC3339 time | observation timestamp; if omitted, server time is used |
| `request_rate` | float | observed incoming request rate |
| `error_rate` | float | fraction in `[0,1]` |
| `latency.p50` | float | 50th percentile latency in ms |
| `latency.p95` | float | 95th percentile latency in ms |
| `latency.p99` | float | 99th percentile latency in ms |
| `latency.mean` | float | mean latency in ms |
| `cpu_usage` | float | CPU usage proxy |
| `mem_usage` | float | memory usage proxy |
| `active_conns` | integer | active connections |
| `queue_depth` | integer | queued work depth |
| `upstream_calls[]` | object array | upstream dependency observations |

#### Upstream Call Fields

| Field | Type | Meaning |
| --- | --- | --- |
| `target_service_id` | string | downstream dependency name |
| `call_rate` | float | observed call rate to that dependency |
| `error_rate` | float | dependency error fraction |
| `latency_mean` | float | mean call latency in ms |

#### Example: Single Point

```json
{
  "service_id": "checkout",
  "timestamp": "2026-03-28T12:00:00Z",
  "request_rate": 180.5,
  "error_rate": 0.02,
  "latency": {
    "p50": 38,
    "p95": 130,
    "p99": 210,
    "mean": 62
  },
  "cpu_usage": 0.61,
  "mem_usage": 0.58,
  "active_conns": 124,
  "queue_depth": 18,
  "upstream_calls": [
    {
      "target_service_id": "payments",
      "call_rate": 74.0,
      "error_rate": 0.01,
      "latency_mean": 48
    }
  ]
}
```

#### Example: Batch

```json
[
  {
    "service_id": "frontend",
    "request_rate": 220,
    "error_rate": 0.01,
    "latency": { "p50": 22, "p95": 60, "p99": 90, "mean": 31 },
    "cpu_usage": 0.55,
    "mem_usage": 0.48,
    "active_conns": 80,
    "queue_depth": 4
  },
  {
    "service_id": "payment",
    "request_rate": 75,
    "error_rate": 0.03,
    "latency": { "p50": 40, "p95": 140, "p99": 240, "mean": 70 },
    "cpu_usage": 0.63,
    "mem_usage": 0.57,
    "active_conns": 42,
    "queue_depth": 10
  }
]
```

#### Responses

| Status | Meaning |
| --- | --- |
| `202 Accepted` | telemetry accepted; response body contains ingested count |
| `400 Bad Request` | invalid JSON or read error |
| `401 Unauthorized` | missing or incorrect ingest token when auth is enabled |
| `405 Method Not Allowed` | non-POST request |

#### Example Response

```json
{
  "ingested": 2
}
```

### `GET /ws`

Returns a live WebSocket stream of runtime state. The payload is the streaming `TickPayload`, which includes:

- modeled service bundles
- directives
- objective score
- topology snapshot
- reasoning events
- simulation overlay
- prediction timeline
- risk timeline
- runtime health metrics
- safety level

This is the primary feed used by the built-in dashboard.

### `GET /healthz`

Returns a small JSON document with:

- status
- UTC time
- connected WebSocket client count

### `GET /`

Serves the built-in dashboard UI.

## Dashboard

The dashboard is intended as an operator control room, not as a decorative status page. It is backed by the WebSocket tick stream and exposes the runtime state directly.

### What the Operator Sees

The dashboard can render and summarize:

- current service model bundles
- queue pressure and backlog indicators
- control directives
- topology structure
- prediction trajectories
- reasoning events
- simulation overlay state
- runtime latency and safety metrics

### Important Dashboard Signals

Based on the current streaming schema, the dashboard surface can include:

- `Bundles`
  - queue metrics
  - stochastic metrics
  - signal metrics
  - stability metrics
- `Directives`
  - service-specific control outputs
- `PredictionTimeline`
  - forward utilization curves
- `RiskTimeline`
  - forward collapse risk runway
- `PriorityRiskQueue`
  - urgency-ranked services
- `PressureHeatmap`
  - normalized service pressure
- `NetworkEquilibrium`
  - system-level coupled state
- `TopologySensitivity`
  - keystone and structurally fragile services
- `SimOverlay`
  - predicted queue and SLA risk at horizon
- `ScenarioComparison`
  - best/worst/median outcomes across simulation scenarios
- `RuntimeMetrics`
  - per-stage latency, predicted overrun, total overruns, safety level

### Why the Dashboard Matters

The dashboard is part of the control loop, not just a presentation layer. It lets operators verify:

- whether the controller is acting on good telemetry
- whether predicted risk is structural or local
- whether the runtime itself is under timing pressure
- whether simulation results support or contradict the current intervention

## Example Workflow

### 1. Start the Stack

```bash
docker compose up --build
```

### 2. Open the Dashboard

Open:

```text
http://localhost:8080/
```

Check health:

```bash
curl http://localhost:8080/healthz
```

### 3. Send Telemetry

Use the configured ingest token:

```bash
curl -X POST http://localhost:8080/api/v1/ingest \
  -H "Content-Type: application/json" \
  -H "X-Ingest-Token: changeme-set-in-production" \
  -d '[
    {
      "service_id": "frontend",
      "request_rate": 200,
      "error_rate": 0.01,
      "latency": { "p50": 20, "p95": 55, "p99": 85, "mean": 30 },
      "cpu_usage": 0.52,
      "mem_usage": 0.47,
      "active_conns": 70,
      "queue_depth": 6
    },
    {
      "service_id": "checkout",
      "request_rate": 120,
      "error_rate": 0.03,
      "latency": { "p50": 45, "p95": 140, "p99": 230, "mean": 72 },
      "cpu_usage": 0.64,
      "mem_usage": 0.59,
      "active_conns": 58,
      "queue_depth": 14,
      "upstream_calls": [
        {
          "target_service_id": "payment",
          "call_rate": 120,
          "error_rate": 0.02,
          "latency_mean": 50
        }
      ]
    }
  ]'
```

### 4. Observe the Runtime

After a few ticks, inspect:

- queue pressure and stability state in the dashboard
- directives produced for each service
- reasoning events
- prediction and risk timeline panels

### 5. Observe Actuation

By default:

- the queue backend will log worker scaling changes per service

Example log shape:

```text
[actuator:queue] svc=checkout workers=1->2 scale=1.800 tick=42
```

### 6. Route a Service to an External HTTP Actuator

Restart with:

```bash
ACTUATOR_HTTP_ENDPOINT=http://controller:9000/scale \
ACTUATOR_HTTP_SERVICES=checkout,payment \
docker compose up --build
```

For those routed services, the HTTP backend sends:

```json
{
  "service_id": "checkout",
  "scale_factor": 1.8
}
```

### 7. Iterate on Tuning

Adjust:

- `UTILISATION_SETPOINT`
- `COLLAPSE_THRESHOLD`
- `SIM_BUDGET`
- `PREDICTIVE_HORIZON_TICKS`
- `WORKER_POOL_SIZE`

Then repeat the telemetry feed and observe how predictions and directives change.

## Limitations

LoadEquilibrium is an engineering control system, which means it has real strengths and real boundaries.

- It depends on correct telemetry. Bad service rate, latency, or dependency signals will degrade model quality.
- It is not plug-and-play. Operators still need to understand the target system and tune thresholds and horizons.
- Queue models are approximations. They are extremely useful, but they are not a perfect replica of every production path.
- Topology quality depends on upstream call visibility. Missing edge data reduces coupling fidelity.
- The default `QueueBackend` is a deterministic in-memory backend, not a platform-native scaler.
- The `HTTPBackend` is generic. Production-grade infrastructure usually needs platform-specific authentication, retries, and idempotency semantics around it.
- Persistence compatibility depends on the database environment. The bundled Compose setup handles local compatibility, but managed PostgreSQL deployments require explicit operator configuration.
- The system helps with decision support and controlled intervention. It does not replace good capacity planning, SLO design, or incident process.

## Roadmap

The current codebase is functional, but several engineering improvements are still natural next steps.

- Add a Prometheus `/metrics` endpoint for runtime and model health export.
- Add more platform-specific actuator backends such as Kubernetes or gateway integrations.
- Externalize scenario definitions instead of relying only on code or environment-driven wiring.
- Expand operator APIs beyond ingest, health, and WebSocket streaming.
- Add structured logging options for better production observability.
- Improve persistence and operational compatibility for broader PostgreSQL environments.
- Add more deployment profiles for larger service graphs with different tick and budget defaults.
- Strengthen controller calibration workflows and operator tuning guidance.

## Contributing

Contributions should preserve the identity of the project as a deterministic predictive control system.

### General Guidelines

- Keep changes model-based and explicit.
- Do not reframe the project as AI, ML, or black-box automation.
- Prefer clear engineering tradeoffs over feature inflation.
- Keep actuation, safety, and observability changes easy to audit.

### Recommended Workflow

1. Open an issue or design note for non-trivial changes.
2. Keep patches scoped to one subsystem where possible.
3. Add or update tests when behavior changes.
4. Run formatting and build checks before submission.
5. Update the README or config documentation if the runtime surface changes.

### Useful Commands

```bash
gofmt -w ./...
go build ./cmd/loadequilibrium/
docker compose config
docker compose up --build
```

### Areas Where Contributions Are Especially Useful

- actuator integrations
- metrics export
- dashboard clarity
- scenario management
- persistence hardening
- operational docs and examples

## License

License selection is currently a placeholder.

Before external distribution or commercial use, add an organization-approved `LICENSE` file at the repository root. Until then, treat the project as unlicensed proprietary source unless your organization states otherwise.
