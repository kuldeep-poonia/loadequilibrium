# LoadEquilibrium

**LoadEquilibrium is an autonomous backend intelligence and control system for distributed services.**

It takes live service telemetry, reconstructs hidden queueing and dependency pressure, predicts instability before it becomes visible in standard dashboards, and turns that into runtime control directives through a five-phase intelligence pipeline.

If you want the short version:

- In simple words: this project is an autopilot for backend systems.
- In technical words: this is a deterministic, tick-driven control plane that combines telemetry inference, queueing theory, topology analysis, predictive optimisation, simulation, and autonomous decision fusion.

## Why This Project Exists

Modern production systems usually react too late.

Most platforms scale or shed traffic after CPU, error rate, or tail latency has already moved into the danger zone. By that point, queue growth, retry storms, downstream pressure, and collapse cascades may already be underway.

LoadEquilibrium is built around a different assumption:

**the safest time to intervene is before saturation is obvious.**

That means the system focuses on:

- hidden queue pressure instead of only resource counters
- topology-aware risk instead of isolated service metrics
- predictive intervention instead of reactive dashboards
- safe autonomous control instead of pure recommendation-only analytics

## What It Does

### In Simple Language

Imagine a smart control room for your backend:

- it watches services talk to each other
- it notices which services are quietly getting overloaded
- it predicts which one will break next
- it tests safer responses in a sandbox
- it picks a control action and sends it to an actuator

The goal is to stop a backend incident before it turns into a visible outage.

### In Technical Language

LoadEquilibrium runs a continuous closed loop:

1. ingest telemetry for services and upstream edges
2. build confidence-scored service windows
3. reconstruct queue state, utilisation, latency pressure, and topology coupling
4. estimate stability, cascade risk, and saturation horizons
5. compute control directives with optimisation logic
6. pass those directives through a five-phase intelligence chain
7. dispatch runtime-safe directives to the actuator
8. stream full state to a live WebSocket dashboard

## End-to-End Pipeline

```text
Telemetry / Infra State
  -> Service Windows + Freshness Scoring
  -> Topology + Coupling + Queue Modelling
  -> Objective / Control Optimisation
  -> Phase 1: Policy Intelligence
  -> Phase 2: Recommendation Engine
  -> Phase 3: Closed-Loop Autopilot
  -> Phase 4: Sandbox Simulation (when risk warrants)
  -> Phase 5: Advanced Intelligence / Autonomy Layer
  -> Actuator Dispatch
  -> Dashboard + Persistence + Reasoning Events
```

This flow is implemented in the runtime orchestrator and the phase runtime bridge, with the phase chain executed before final actuation.

## The Five Intelligence Phases

| Phase | Name | Purpose | Runtime Role |
| --- | --- | --- | --- |
| 1 | Policy Intelligence | Evaluates scaling, retry, queue, and cache policy signals | Produces coarse defensive policy decisions and global policy risk |
| 2 | Recommendation Engine | Converts policy and system stress into intervention recommendations | Refines capacity, damping, retry pressure, and brownout guidance |
| 3 | Closed-Loop Autopilot | Uses predictor, identification, MPC, rollout, and safety logic | Produces runtime telemetry-backed autopilot actions and bounded control behavior |
| 4 | Sandbox Simulation | Runs deterministic experiment comparisons under candidate actions | Tests potential interventions under disturbance before trusting them more deeply |
| 5 | Advanced Intelligence | Uses autonomy telemetry, hazard estimation, rollout certification, safety projection, and decision fusion | Certifies or degrades actions, switches autonomy modes, and keeps the final action safe |

### Phase 1: Policy Intelligence

Policy intelligence lives in `internal/policy` and evaluates:

- scaling pressure
- retry policy pressure
- queue policy pressure
- cache policy pressure
- fused policy-level risk and cost

This is the first intelligence layer that turns raw system stress into control meaning.

### Phase 2: Recommendation Engine

The recommendation layer lives in `internal/sandbox/policy_recommendation.go` and related comparison logic.

It translates model output into more actionable recommendations such as:

- capacity increase or reduction
- damping suggestions
- retry pressure shifts
- efficiency and brownout signals

It is the bridge between "the system is under stress" and "here is the style of intervention that makes sense."

### Phase 3: Closed-Loop Autopilot

The autopilot layer in `internal/autopilot` gives the project its control-theory character.

It includes:

- predictor
- MPC optimiser
- identification engine
- rollout controller
- safety engine
- runtime orchestrator

This phase is responsible for turning recommendations into controlled behavior rather than one-shot heuristics.

### Phase 4: Sandbox Simulation

The sandbox in `internal/sandbox` acts like a digital twin harness.

When risk is elevated, the runtime can:

- generate disturbance scenarios
- run baseline vs candidate experiments
- compare temporal robustness and collapse energy
- produce recommendation metadata from simulated outcomes

This is how the system reduces the danger of blindly trusting an intervention under uncertain conditions.

### Phase 5: Advanced Intelligence / Autonomy Layer

The autonomy stack in `internal/intelligence` is the highest-level reasoning and certification layer.

It adds:

- autonomy telemetry modelling
- predictive rollout
- hazard value estimation
- safety projection
- adaptive learning hooks
- mode switching between advisory, supervised, autonomous, and safety-only

This phase makes the system more than a controller. It makes it an autonomy framework with explicit safety behavior.

## Core Runtime Design

LoadEquilibrium is built as a deterministic tick engine.

On every tick, the runtime:

1. prunes stale services
2. computes service windows with freshness and confidence
3. applies scenario superposition if enabled
4. computes topology state and network coupling
5. runs queue and stochastic modelling
6. computes optimisation objectives and directives
7. executes the five-phase intelligence chain
8. dispatches directives asynchronously to the actuator
9. broadcasts full runtime state to the dashboard
10. persists snapshots when a database is configured

The runtime also includes:

- adaptive tick cadence
- hard tick deadlines
- pressure-aware stage skipping
- safety escalation levels
- simulation budget control
- degraded-intelligence handling for stale or sparse telemetry

## Research Character

This project sits in an interesting middle ground:

- too operational to be only a toy simulation
- too intelligence-heavy to be only a dashboard
- too control-oriented to be only an autoscaler

It mixes ideas from:

- queueing theory
- distributed systems reliability
- model predictive control
- online optimisation
- topology sensitivity analysis
- Monte Carlo scenario analysis
- safe autonomy and runtime decision certification

That makes it suitable both as a practical engineering platform and as a research-grade experimentation base.

## Main Building Blocks

### Telemetry

`internal/telemetry`

- ingests service metric points
- stores bounded ring buffers
- computes service windows
- infers missing signals when possible
- assigns confidence and signal quality

### Modelling

`internal/modelling`

- queue physics
- signal processing
- stochastic modelling
- network coupling
- fixed-point equilibrium analysis
- topology sensitivity
- stability assessment

### Optimisation and Control

`internal/optimisation`
`internal/control`
`internal/autopilot`

- objective scoring
- trajectory planning
- controller output
- actuator dynamics
- autopilot rollout and safety

### Sandbox and Simulation

`internal/sandbox`
`internal/simulation`
`internal/scenario`

- digital twin experiments
- Monte Carlo risk estimation
- synthetic disturbance injection
- scenario comparison

### Intelligence and Reasoning

`internal/intelligence`
`internal/reasoning`

- autonomy orchestration
- hazard and rollout intelligence
- safety-constrained action projection
- operator-facing reasoning events

### Runtime and Interfaces

`internal/runtime`
`internal/dashboard`
`internal/streaming`
`internal/actuator`
`internal/persistence`

- orchestrates the tick engine
- exposes HTTP and WebSocket interfaces
- streams live control-room state
- dispatches runtime directives
- optionally persists snapshots

## API Surface

The application exposes a simple operator-friendly runtime surface:

- `POST /api/v1/ingest`
  Push telemetry into the engine.
- `GET /ws`
  Subscribe to the live state stream.
- `GET /healthz`
  Health status and connected client count.
- `GET /`
  Built-in live dashboard.

### Telemetry Payload Example

```json
{
  "service_id": "checkout",
  "timestamp": "2026-03-27T12:00:00Z",
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

You can submit either a single metric point or an array of metric points.

## Dashboard

The built-in dashboard is not an afterthought. It is a control-room view over the runtime.

It exposes views for:

- overview
- topology
- stability envelope
- prediction runway
- optimisation output
- simulation comparison
- telemetry freshness
- reasoning/event log
- runtime health and safety state

This is useful for both incident response and research observation.

## Actuation Model

The runtime does not hardcode a cloud vendor or a single orchestrator.

Instead, it uses an actuator interface:

- dispatch directives asynchronously
- collect feedback safely
- allow coalescing and non-blocking actuation

That design makes it easier to map the output into:

- a custom control plane
- a gateway policy engine
- a Kubernetes-style scaler
- a traffic shaping or brownout layer

## Scenarios and Disturbances

The runtime can superimpose synthetic disturbances on observed telemetry.

This is useful for:

- testing the control loop under bursts
- studying propagation through dependencies
- evaluating recovery behavior
- stress-testing the dashboard and reasoning pipeline

The default main program wires example burst scenarios for `frontend` and `payment`.

## Running The Project

### Local Development

```bash
go run ./cmd/loadequilibrium/
```

### With Make

```bash
make build
make run
```

### With Docker

```bash
docker build -t loadequilibrium .
docker run -p 8080:8080 loadequilibrium
```

### With Docker Compose

```bash
docker compose up --build
```

The compose stack starts:

- the LoadEquilibrium runtime
- PostgreSQL for snapshot persistence

## Important Configuration

Key environment variables include:

| Variable | Meaning | Default |
| --- | --- | --- |
| `LISTEN_ADDR` | HTTP listen address | `:8080` |
| `TICK_INTERVAL` | Main engine cadence | `2s` |
| `TICK_DEADLINE` | Hard budget per tick | `1800ms` |
| `RING_BUFFER_DEPTH` | Per-service telemetry history | `300` |
| `MAX_SERVICES` | Maximum tracked services | `200` |
| `STALE_SERVICE_AGE` | Service eviction threshold | `5m` |
| `SIM_BUDGET` | Per-tick simulation time budget | `45ms` |
| `SIM_HORIZON_MS` | Monte Carlo horizon | `60000` |
| `SIM_SHOCK_FACTOR` | Shock intensity multiplier | `2.0` |
| `UTILISATION_SETPOINT` | Controller target utilisation | `0.70` |
| `COLLAPSE_THRESHOLD` | Collapse risk threshold | `0.90` |
| `SCENARIO_MODE` | Enable or disable disturbances | `on` |
| `DATABASE_URL` | Snapshot persistence target | empty |

## What Makes It Different

Many observability systems tell you what happened.

LoadEquilibrium tries to answer what happens next and what should be done now.

That difference matters.

This project is intentionally built around:

- predictive rather than reactive control
- topology-aware rather than service-isolated reasoning
- runtime-safe actuation rather than alert-only analysis
- autonomy with explicit safety fallback rather than naive automation

## Current Strengths

- strong runtime visibility through the dashboard and stream payloads
- explicit five-phase intelligence flow
- deterministic tick engine with pressure-aware degradation
- topology and stability modelling beyond basic autoscaling heuristics
- built-in sandboxing for candidate intervention comparison
- autonomy layer with safety-only fallback modes

## Current Limitations

This project is ambitious, and it is honest about that.

- It is still a bespoke control system, not a turnkey managed platform.
- Real-world actuator integrations can be expanded further.
- Large, dense topologies will eventually require deeper horizontal scaling strategy.
- Model quality depends on telemetry quality, especially for tail and burst behavior.
- Some advanced intelligence modules are still better described as runtime-capable research primitives than mature ML products.

## Who This Is For

LoadEquilibrium is a good fit for:

- platform engineers
- reliability engineers
- control-systems-minded backend builders
- applied researchers working on autonomous infrastructure
- teams that want to experiment with predictive incident prevention

## Project Structure

```text
cmd/loadequilibrium      application entrypoint
internal/runtime         main orchestration loop and phase bridge
internal/telemetry       ingestion, windows, ring buffers
internal/modelling       queue, signal, topology, stability, coupling
internal/optimisation    objective and directive generation
internal/policy          policy intelligence
internal/autopilot       closed-loop control runtime
internal/sandbox         experiment and recommendation sandbox
internal/intelligence    autonomy and advanced intelligence layer
internal/simulation      Monte Carlo simulation runtime
internal/reasoning       operator-facing event reasoning
internal/dashboard       built-in browser UI
internal/streaming       live payload schema and hub
internal/actuator        dispatch interface and implementations
internal/persistence     snapshot writer
```

## Vision

The long-term vision is straightforward:

**turn backend operations from reactive firefighting into safe, explainable, predictive autonomy.**

That means a system that can:

- understand service stress structurally
- predict unstable futures before they happen
- test possible responses
- act conservatively and safely
- explain why it acted

## Closing Summary

LoadEquilibrium is not just a simulator, not just a dashboard, and not just a controller.

It is an integrated backend intelligence runtime that combines modelling, optimisation, simulation, and autonomy into one operational loop.

If you want a one-line description for GitHub:

> **An autonomous control plane for backend systems that predicts saturation, simulates interventions, and safely turns telemetry into action.**
