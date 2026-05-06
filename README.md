# loadequilibrium

**An adaptive, tick-based autopilot control system** that monitors distributed services in real time, identifies instability before it becomes an outage, and issues precise scaling directives — autonomously, without human intervention.

---

## What It Is

Most autoscalers react to a metric crossing a threshold. loadequilibrium does something fundamentally different: it builds a **continuous mathematical model of your system** — arrival rates, queue dynamics, service rates, upstream dependency health — and uses that model to anticipate failure and act with precision before the queue overflows or latency spikes.

The result is a system that distinguishes between a genuine load surge and a noisy sensor, scales capacity at exactly the right speed (not too fast, not too slow), and remains stable even when the signal it's reading is volatile.

**It is not a Kubernetes controller. It is not a threshold alarm.** It is a closed-loop control system, the same class of technology used in aircraft autopilots and industrial process controllers — applied to distributed software.

---

## How It Works — The Full Pipeline

Every two seconds, the following sequence executes deterministically:

```
                         ┌───────────────────────────────────────────────────┐
                         │              EXTERNAL SERVICES                    │
                         │   service-A   service-B   service-C   ...         │
                         └──────────────┬────────────────────────────────────┘
                                        │  POST /api/v1/ingest
                                        │  { request_rate, latency, error_rate,
                                        │    cpu, mem, queue_depth, upstream_calls }
                                        ▼
                         ┌─────────────────────────────┐
                         │      Telemetry Store         │
                         │  ring buffer per service     │
                         │  300 samples × N services    │
                         │  prune stale after 5 min     │
                         └──────────────┬──────────────┘
                                        │
                    ┌───────────────────┼────────────────────────────┐
                    │                   │                            │
                    ▼                   ▼                            ▼
           ┌──────────────┐   ┌─────────────────┐       ┌──────────────────┐
           │  Topology    │   │   Window        │       │  Stochastic      │
           │  Graph       │   │   Computation   │       │  Model           │
           │              │   │                 │       │                  │
           │ builds DAG   │   │ EWMA fast+slow  │       │ burst amplitude  │
           │ of upstream  │   │ variance, CoV   │       │ arrival variance │
           │ call edges   │   │ confidence score│       │ risk propagation │
           │ critical path│   │ signal quality  │       │ probability      │
           └──────┬───────┘   └────────┬────────┘       └────────┬─────────┘
                  │                   │                          │
                  └───────────────────┼──────────────────────────┘
                                      │
                                      ▼
                         ┌────────────────────────────┐
                         │    Reasoning Engine         │
                         │                            │
                         │  hysteresis-gated rules    │
                         │  cause → model →           │
                         │  prediction → action       │
                         │                            │
                         │  emits structured events:  │
                         │  · collapse_risk           │
                         │  · cascade_risk            │
                         │  · saturation_predicted    │
                         │  · keystone_degraded       │
                         └──────────────┬─────────────┘
                                        │
                                        ▼
                         ┌────────────────────────────┐
                         │     Simulation Engine       │
                         │                            │
                         │  discrete-event sim        │
                         │  60-second horizon         │
                         │  stochastic arrival model  │
                         │  shock factor 2×           │
                         │  async, drop-oldest        │
                         └──────────────┬─────────────┘
                                        │
                                        ▼
                    ┌───────────────────────────────────────┐
                    │           AUTOPILOT CORE              │
                    │                                       │
                    │  ┌──────────────────────────────┐    │
                    │  │   System Identification       │    │
                    │  │                              │    │
                    │  │  · EWMA fast (α=0.30)        │    │
                    │  │  · EWMA slow (α=0.10)        │    │
                    │  │  · ArrivalVar tracker        │    │
                    │  │  · BurstEnergy (decay=0.25)  │    │
                    │  │  · NoiseEnergy tracker       │    │
                    │  │  · ConfidenceScore           │    │
                    │  └──────────────┬───────────────┘    │
                    │                 │                     │
                    │  ┌──────────────▼───────────────┐    │
                    │  │   Instability Engine          │    │
                    │  │                              │    │
                    │  │  load-scaled oscillation     │    │
                    │  │  pressure + momentum score   │    │
                    │  │  variance scale [0,1]        │    │
                    │  │  instabilityScore → mode     │    │
                    │  └──────────────┬───────────────┘    │
                    │                 │                     │
                    │  ┌──────────────▼───────────────┐    │
                    │  │   MPC + Rollout Controller    │    │
                    │  │                              │    │
                    │  │  target capacity computation │    │
                    │  │  ramp rate: Normal=30/tick   │    │
                    │  │  ramp rate: Emergency=14/tick│    │
                    │  │  ramp rate: scale-down=1.5   │    │
                    │  │  queue pressure gradient     │    │
                    │  │  mode: Normal/Degraded/      │    │
                    │  │        Emergency/Safe        │    │
                    │  └──────────────┬───────────────┘    │
                    │                 │                     │
                    │  ┌──────────────▼───────────────┐    │
                    │  │   Decision Policy             │    │
                    │  │                              │    │
                    │  │  Regime Memory lookup        │    │
                    │  │  oscillation history         │    │
                    │  │  cooldown enforcement        │    │
                    │  └──────────────┬───────────────┘    │
                    └─────────────────┼─────────────────────┘
                                      │
                                      ▼
                         ┌────────────────────────────┐
                         │   Control Authority         │
                         │                            │
                         │  PID controller             │
                         │  trajectory planning        │
                         │  N-candidate evaluation     │
                         │  3-tick direction cooldown  │
                         │  Bundle.Replicas (integer)  │
                         │  ScaleFactor (float)        │
                         └──────────────┬─────────────┘
                                        │
                                        ▼
                         ┌────────────────────────────┐
                         │   Coalescing Actuator       │
                         │                            │
                         │  deduplicates directives   │
                         │  async, non-blocking        │
                         │  LogOnlyBackend (default)  │
                         │  HTTPBackend (production)  │
                         └──────────────┬─────────────┘
                                        │
                                        ▼
                         ┌────────────────────────────┐
                         │   External Execution        │
                         │   (Kubernetes, Nomad, etc.) │
                         └────────────────────────────┘
```

---

## The Control Loop in Detail

### 1. Telemetry Ingestion

Services push metrics via `POST /api/v1/ingest` authenticated with `INGEST_TOKEN`. Each point is sanitised (NaN/Inf clamped, error rate bounded to [0,1], latency floored at 0.1ms to prevent division-by-zero) and written into a **per-service ring buffer** of 300 samples. Stale services (no ingest for 5 minutes) are pruned automatically.

### 2. Window Computation

Each tick, a sliding window is computed per service from the ring buffer. It produces:

- `MeanRequestRate` / `LastRequestRate` — arrival rate signal
- `MeanLatencyMs` / `LastP99LatencyMs` — latency distribution
- `ConfidenceScore` — composite [0,1] signal quality based on sample count, arrival rate stability (CoV), and data freshness
- `SignalQuality` — categorical: `good` / `degraded` / `sparse`

Low-confidence windows cause the autopilot to downgrade its predictions, not ignore them — graceful degradation under bad data.

### 3. Topology Graph

Every service that reports upstream calls (`upstream_calls[]` in the ingest payload) is wired into a directed acyclic graph. The graph:

- Computes **critical paths** — chains of services where failure propagates end-to-end
- Identifies **keystone services** — nodes with the highest fan-out (failure impact)
- Propagates **load estimates** through edges to model induced upstream pressure
- Prunes edges below a weight floor to stay `O(N + E_significant)`

This makes it possible for the reasoning engine to issue a `cascade_risk` alert for service-B *before* it is overloaded, because service-A's queue depth is already rising and B is directly downstream.

### 4. Stochastic Model

For each service window, three quantities are computed:

- **Arrival variance** — how unpredictable the load is
- **Burst amplification** — how much worse the 95th-percentile load is vs mean
- **Risk propagation probability** — likelihood that instability spreads to dependents

These feed into the trajectory planner's probabilistic cost evaluation: a capacity action that looks cheap in deterministic analysis but sits in a high-variance zone is penalised upward.

### 5. System Identification

The identification engine runs a set of recursive estimators on the arrival signal:

```
ArrivalFast ─── α=0.30 EWMA  ──► fast-tracking signal (responds in ~3 ticks)
ArrivalSlow ─── α=0.10 EWMA  ──► trend signal (responds in ~10 ticks)
ArrivalBlend ── weighted mix  ──► main arrival estimate

ArrivalVar  ─── online variance tracker
BurstEnergy ─── excess / relative-floor → decay=0.25 (half-life 2.8 ticks)
NoiseEnergy ─── high-frequency jitter detector
```

**Key design decision**: The burst normalisation uses a relative floor of `max(mean × 0.10, 1.0)` as the denominator. This prevents the variance tracker decaying toward zero during stable periods from producing explosively large burst signals when load increases. A +1.67 rps increment on a 20 rps baseline produces `excess = 1.67/2.0 = 0.84` — a real but proportionate signal. Without the floor it would produce `excess = 9.98`, maxing out BurstEnergy for the entire rising-load scenario.

### 6. Instability Scoring

The instability engine combines four signals multiplicatively:

```
pressure  = queue_depth / sla_limit          (how full is the queue?)
momentum  = burst_energy / burst_cap         (is load growing fast?)
variance  = arrival_var / relative_floor     (how unpredictable is arrival?)
oscillation = osc_score × load_context      (is the controller hunting?)

instabilityScore = bounded_aggregate(pressure, momentum, variance, oscillation)
```

A score above 0.5 AND backlog above 10% of SLA triggers an `instability_high` event. The backlog gate is critical: it prevents a noisy signal during a stable period (workload volatility) from being classified as system instability.

### 7. Governance Mode

The autopilot operates in one of four modes, selected each tick:

| Mode | Trigger | Behaviour |
|---|---|---|
| `Normal` | backlog < 48% SLA | Standard ramp rates, confidence-weighted predictions |
| `Degraded` | backlog > 48% SLA | Emergency ramp rate, higher urgency |
| `Emergency` | instabilityScore > 0.7 OR backlog > 80% SLA | Maximum ramp, full priority |
| `Safe` | signal quality critically low | Conservative holds, waits for confidence recovery |

### 8. Capacity Target Computation (MPC + Rollout)

Given the current arrival estimate and utilisation setpoint (70%), the **Model Predictive Controller** computes a target replica count:

```
targetCapacity = arrivalEstimate / (serviceRate × utilisationSetpoint)
```

The **rollout controller** then ramps the current capacity toward the target:

```
rampRate (Normal)    = 30 replicas/tick
rampRate (Emergency) = 14 replicas/tick  + proportional boost (capped at 2×)
rampRate (scale-down)= 1.5 replicas/tick (conservative — never crash the fleet)

queuePressureMultiplier = 1.0 + 0.5 × ((backlog - 50) / (SLA - 50))  [max 1.5×]
```

The proportional boost for large deficits (`min(err × 0.3, rate)`) means the system accelerates toward a target that is far away, but cannot panic-scale. A 100-replica deficit at emergency rate produces `boost = min(30, 14) = 14`, total rate `= 28`. Not `164`.

### 9. Control Authority

The authority converts the float capacity target into an integer `Bundle.Replicas` decision. It also:

- Enforces a **3-tick direction-change cooldown** — prevents oscillating between scale_up and scale_down within a burst transition
- Runs a **PID controller** (Kp=-1.5, Ki=-0.3, Kd=-0.1) for fine-grained utilisation tracking
- Evaluates **N trajectory candidates** via `PlanTrajectory`, ranking them by probabilistic cost
- Applies **anti-windup integral clamping** and a **hysteresis deadband** to suppress chattering near setpoint

### 10. Actuator Dispatch

The `CoalescingActuator` receives a `DirectiveSnapshot` — an immutable, decoupled control directive — and dispatches it asynchronously. It deduplicates identical consecutive directives. The default backend (`LogOnlyBackend`) logs what would be done; a production `HTTPBackend` sends the scaling command to the target orchestrator.

---

## Why This Architecture

### Why tick-based?

A fixed control interval (`TICK_INTERVAL=2s`) makes the system **deterministic and auditable**. Every decision is traceable to a specific tick, a specific set of measurements, and a specific computation path. Interrupt-driven or fully async systems make it much harder to reason about why a decision was made.

The orchestrator enforces a **tick deadline** (`TICK_DEADLINE=1800ms`). If any pipeline stage runs long, the orchestrator adaptively stretches the interval rather than dropping the tick — preventing cascading overruns. It also **pre-emptively stretches** when EWMA-predicted stage cost exceeds 85% of the deadline, before the first overrun occurs.

### Why a physics plant model?

The `FluidPlant` and `FinalFluidPlant` models treat the request queue as a **stochastic fluid system** with:

- Nonlinear inflow driven by measured arrival rates
- Hazard accumulation when utilisation approaches 1.0
- Reservoir dynamics that capture burst absorption capacity
- Reflection forces that prevent queue from going negative

This is more accurate than a simple `arrivals - departures` model because it captures the nonlinear behaviour of real queues near saturation — where latency degrades superlinearly and small changes in load produce large changes in queue depth.

### Why policy gradient + PID?

The system uses **two complementary controllers**:

- The **PID controller** handles steady-state tracking with known dynamics — it is fast, interpretable, and provably stable for linear systems
- The **Policy Gradient Optimizer** (PPO-style actor-critic) handles the non-linear, delayed-reward regime where the PID's linearisation breaks down — burst transitions, topology-induced load shifts, regime changes

The PG optimizer uses **Generalised Advantage Estimation (GAE)**, reward normalisation, and a **safety cost function** that penalises actions with `r > 0.85` utilisation, making it risk-aware rather than purely reward-maximising.

### Why a sandbox?

The `sandbox` package provides a safe **pre-flight simulation environment** where proposed control strategies can be evaluated against historical or synthetic scenarios before being applied in production. It supports:

- Deterministic replay of past incidents
- Uncertainty-inflated scenarios (what if the load was 2× worse?)
- Phase-comparison between candidate policies

---

## Data Flow: Ingest to Decision

```
External Service
      │
      │  POST /api/v1/ingest
      │  Authorization: Bearer <INGEST_TOKEN>
      │  Content-Type: application/json
      │
      ▼
┌─────────────────────────────────────────────────────┐
│  handleIngest()                                      │
│  1. Authenticate token                              │
│  2. Decode JSON → MetricPoint                       │
│  3. Sanitise: clamp NaN/Inf, floor latency 0.1ms   │
│  4. store.Ingest(point)                             │
│  5. hub.Broadcast(snapshot) → WebSocket clients    │
└──────────────────────────┬──────────────────────────┘
                           │
              [every 2 seconds — tick boundary]
                           │
                           ▼
┌─────────────────────────────────────────────────────┐
│  orchestrator.tick()                                 │
│                                                     │
│  Stage 1: prune stale services                     │
│  Stage 2: compute windows (parallel, worker pool)  │
│  Stage 3: topology graph update                    │
│  Stage 4: stochastic model per service             │
│  Stage 5: reasoning engine → events                │
│  Stage 6: async simulation (60s horizon)           │
│  Stage 7: autopilot tick (see below)               │
│  Stage 8: actuator dispatch                        │
│  Stage 9: persistence write (async, non-blocking)  │
│                                                     │
│  [deadline enforced — stages skip-or-defer on      │
│   overrun; EWMA latency tracked per stage]         │
└──────────────────────────┬──────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────┐
│  autopilot.Tick(state, measuredArrival)              │
│                                                     │
│  1. Anomaly gate: reject >10× EWMA estimate        │
│  2. updateIdentification() — EWMA, var, burst      │
│  3. forecastBacklog() — MPC queue model            │
│  4. instabilityEngine.Score() → instScore          │
│  5. modeProb() → governance mode                   │
│  6. delay() → latency model                        │
│  7. rollout.Step() → next CapacityActive           │
│  8. safetyState() → safety margin                  │
│  9. mpcState() → MPC inputs                        │
│ 10. Return TelemetryOutput{Capacity, Backlog, ...} │
└──────────────────────────┬──────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────┐
│  control.Authority.Evaluate(input)                   │
│                                                     │
│  1. Compute required capacity from utilisation     │
│  2. Generate candidate bundle set                  │
│  3. PlanTrajectory() — rank N candidates           │
│  4. PID adjustment                                 │
│  5. Cooldown enforcement (3-tick direction lock)   │
│  6. Return AuthorityDecision{Bundle, Directive}    │
└──────────────────────────┬──────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────┐
│  actuator.Submit(DirectiveSnapshot)                  │
│  → LogOnlyBackend.Execute() [default]               │
│  → HTTPBackend.Execute()    [production]            │
└─────────────────────────────────────────────────────┘
```

---

## Observability

### Metrics — GET /metrics

The `/metrics` endpoint serves Prometheus text format. All metrics are computed from live state — zero additional scraping, zero external dependencies.

| Metric | Type | What it tells you |
|---|---|---|
| `loadequilibrium_request_rate{service}` | gauge | Live arrival rate per service (rps) |
| `loadequilibrium_queue_depth{service}` | gauge | Current pending request count |
| `loadequilibrium_latency_mean_ms{service}` | gauge | Mean observed latency |
| `loadequilibrium_latency_p99_ms{service}` | gauge | P99 latency — SLA signal |
| `loadequilibrium_applied_scale{service}` | gauge | Last scaling directive (1.0 = no change) |
| `loadequilibrium_confidence_score{service}` | gauge | Signal quality [0,1] — below 0.3 means degraded mode |
| `loadequilibrium_hazard_score{service}` | gauge | Physics-engine hazard [0,1] — above 0.7 means collapse risk |
| `loadequilibrium_scale_decisions_total{direction}` | counter | scale_up / scale_down / hold counts |
| `loadequilibrium_sla_breaches_total` | counter | Cumulative SLA breaches (target: 0) |
| `loadequilibrium_ticks_total` | counter | Control ticks completed |
| `loadequilibrium_tracked_services` | gauge | Active services in telemetry store |
| `go_goroutines` | gauge | Runtime goroutine count |
| `go_heap_alloc_bytes` | gauge | Live heap allocation |

### Health — GET /health

```json
{ "status": "ok", "component": "api_headless", "clients": 3 }
```

Returns 200 when the process is alive and the WebSocket hub is operational. Used by Kubernetes liveness and readiness probes.

### WebSocket — GET /ws

Streams the live tick payload to connected dashboards in real time. Each tick broadcasts a JSON snapshot of all service windows, reasoning events, and the latest control decision. Supports up to `MAX_STREAM_CLIENTS` (default 50) concurrent connections.

### Grafana Dashboard

Provisioned automatically at startup. Five sections:

1. **System Health** — SLA breach counter, tracked services, ticks, uptime, goroutines
2. **Per-Service Traffic** — arrival rate and queue depth time series
3. **Latency** — mean and P99 per service
4. **Autopilot Decisions** — scale_up/scale_down/hold rates, applied scale factor
5. **Signal Quality** — confidence score and hazard score per service

---

## API Reference

| Method | Path | Auth | Description |
|---|---|---|---|
| `POST` | `/api/v1/ingest` | Bearer token | Push a telemetry point for a service |
| `GET` | `/api/v1/snapshot` | — | Last cached tick output as REST (no WebSocket needed) |
| `GET` | `/api/v1/runtime/step` | — | Trigger a manual control tick |
| `POST` | `/api/v1/policy/update` | — | Update control policy parameters at runtime |
| `POST` | `/api/v1/control/toggle` | — | Enable/disable autopilot |
| `POST` | `/api/v1/control/chaos-run` | — | Inject a chaos experiment |
| `POST` | `/api/v1/control/replay-burst` | — | Replay a historical burst scenario |
| `POST` | `/api/v1/sandbox/trigger` | — | Run a sandbox experiment |
| `POST` | `/api/v1/simulation/control` | — | Control the simulation engine |
| `POST` | `/api/v1/intelligence/rollout` | — | Issue a policy gradient rollout |
| `POST` | `/api/v1/alerts/ack` | — | Acknowledge a reasoning event |
| `GET` | `/metrics` | — | Prometheus text format metrics |
| `GET` | `/health` | — | Liveness probe |
| `GET` | `/ws` | — | WebSocket stream |

---

## Configuration Reference

All configuration is via environment variables. Defaults are production-safe.

### Core Engine

| Variable | Default | Description |
|---|---|---|
| `LISTEN_ADDR` | `:8080` | HTTP server bind address |
| `TICK_INTERVAL` | `2s` | Control tick frequency |
| `TICK_DEADLINE` | `1800ms` | Max time per tick before adaptive stretch |
| `MIN_TICK_INTERVAL` | `1s` | Floor for adaptive tick stretching |
| `MAX_TICK_INTERVAL` | `10s` | Ceiling for adaptive tick stretching |
| `TICK_ADAPT_STEP` | `1.25` | Multiplicative stretch factor on overrun |
| `WORKER_POOL_SIZE` | `8` | Parallel workers for window computation |

### Telemetry

| Variable | Default | Description |
|---|---|---|
| `RING_BUFFER_DEPTH` | `300` | Samples retained per service |
| `MAX_SERVICES` | `200` | Maximum tracked services |
| `STALE_SERVICE_AGE` | `5m` | Prune threshold for inactive services |
| `INGEST_TOKEN` | `` | Bearer token for POST /api/v1/ingest — **required** |

### Control Policy

| Variable | Default | Description |
|---|---|---|
| `UTILISATION_SETPOINT` | `0.70` | Target utilisation (70% leaves 30% headroom) |
| `COLLAPSE_THRESHOLD` | `0.90` | Utilisation above which collapse risk is flagged |
| `EWMA_FAST_ALPHA` | `0.30` | Fast EWMA decay — responds in ~3 ticks |
| `EWMA_SLOW_ALPHA` | `0.10` | Slow EWMA decay — responds in ~10 ticks |
| `SPIKE_Z_SCORE` | `3.0` | Z-score threshold for spike detection |
| `PID_KP` | `-1.5` | Proportional gain (negative: reduce on error) |
| `PID_KI` | `-0.3` | Integral gain |
| `PID_KD` | `-0.1` | Derivative gain |
| `PID_DEADBAND` | `0.02` | Suppress output for errors < 2% |
| `PID_INTEGRAL_MAX` | `2.0` | Anti-windup clamp |

### Simulation

| Variable | Default | Description |
|---|---|---|
| `SIM_BUDGET` | `45ms` | Wall-clock budget per tick for simulation |
| `SIM_HORIZON_MS` | `60000` | Simulation lookahead (60 seconds) |
| `SIM_SHOCK_FACTOR` | `2.0` | Worst-case load multiplier in simulation |
| `SIM_STOCHASTIC_MODE` | `exponential` | Arrival distribution: exponential, poisson |

### Prediction & Safety

| Variable | Default | Description |
|---|---|---|
| `ARRIVAL_ESTIMATOR_MODE` | `ewma` | Arrival estimator: ewma, blend |
| `PREDICTIVE_HORIZON_TICKS` | `5` | Ticks ahead for backlog forecast |
| `SAFETY_MODE_THRESHOLD` | `3` | Consecutive overruns before safe mode |
| `SLA_LATENCY_THRESHOLD_MS` | `500.0` | Latency SLA breach threshold (ms) |
| `STALENESS_BYPASS_THRESHOLD` | `0.70` | Confidence below which staleness bypass activates |

### Persistence

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | `` | PostgreSQL DSN — if empty, persistence is disabled |
| `PERSIST_INTERVAL` | `30s` | How often snapshots are flushed to DB |

---

## Deployment

### Docker Compose (Local / Single-Node)

```bash
# 1. Copy the env file template
cp .env.example .env   # set INGEST_TOKEN and DATABASE_URL

# 2. Start everything
docker compose up -d

# 3. Verify
curl http://localhost:8080/health
# Open http://localhost:3000  (Grafana — admin/changeme)
# Open http://localhost:9090  (Prometheus)
```

Services start in dependency order: postgres → loadequilibrium → prometheus → grafana.

### Kubernetes

```bash
# 1. Set real secrets (never commit the YAML values)
kubectl create secret generic loadequilibrium-secrets \
  --from-literal=database-url='postgres://le:PASS@postgres-svc:5432/le?sslmode=require' \
  --from-literal=ingest-token='your-real-token' \
  -n loadequilibrium

# 2. Apply all manifests in order
kubectl apply -f k8s/00-namespace.yml
kubectl apply -f k8s/01-secrets.yml        # replace with your real secrets first
kubectl apply -f k8s/02-configmap.yml
kubectl apply -f k8s/03-postgres.yml
kubectl apply -f k8s/04-deployment.yml
kubectl apply -f k8s/05-prometheus.yml
kubectl apply -f k8s/06-grafana.yml
kubectl apply -f k8s/07-ingress.yml        # update hostnames first

# 3. Watch rollout
kubectl rollout status deployment/loadequilibrium -n loadequilibrium

# 4. Access (before ingress is ready)
kubectl port-forward svc/loadequilibrium-svc 8080:80 -n loadequilibrium
kubectl port-forward svc/grafana-svc 3000:3000 -n loadequilibrium
```

**Important**: loadequilibrium is a **singleton control-plane**. Do not set replicas > 1 on the Deployment. The control loop maintains in-memory state that is not distributed. If you need HA, run it in active-passive mode with a shared PostgreSQL state backend.

### CI/CD (GitHub Actions)

The pipeline in `.github/workflows/ci.yml` runs automatically:

```
On every pull request:
  lint (go vet + gofmt) → unit tests (race detector) → system tests (10/10 autopilot scenarios)

On merge to main (after all tests pass):
  build Docker image → push to GHCR → SSH deploy to production server
```

**Secrets required in GitHub:**

| Secret | Where | Value |
|---|---|---|
| `DEPLOY_HOST` | Repository secrets | Server IP or hostname |
| `DEPLOY_USER` | Repository secrets | SSH username (docker group) |
| `DEPLOY_KEY` | Repository secrets | SSH private key (full PEM) |
| `INGEST_TOKEN` | Environment: production | Production auth token |
| `DATABASE_URL` | Environment: production | Production Postgres DSN |

---

## System Tests

The autopilot ships with a **10-scenario production-readiness test suite** that validates control behaviour under real conditions — not mocked:

| Scenario | What it validates |
|---|---|
| `burst_load_recovery` | 3× sudden load spike — recovers within 8 ticks, no SLA breach |
| `rising_load_tracking` | Gradual ramp from 20→80 rps — tracks without overshoot |
| `queue_saturation_emergency` | 11× instantaneous overload — emergency ramp, max backlog < 130 |
| `retry_storm_cascade` | Exponential retry amplification — identifies as burst, not instability |
| `noisy_signal_ewma` | High-variance sensor data — EWMA smoothing prevents false decisions |
| `scale_down_intelligence` | Load drops — scales down slowly (1.5/tick), no premature cutback |
| `oscillating_load_damping` | Sinusoidal load — direction cooldown prevents hunting |
| `sustained_high_load_sla` | 90% utilisation sustained — holds stable without SLA breach |
| `mpc_autopilot_alignment` | MPC prediction vs actual — < 5% divergence |
| `signal_integrity_nan_spike` | Injected NaN/Inf — anomaly gate absorbs, no crash |

**Gate**: all 10 must pass (`STABLE_PRODUCTION_GRADE`, 0 SLA breaches) or the CI build fails.

```bash
go build -o system_test_runner ./cmd/system_test_runner/
./system_test_runner 2>/dev/null
```

---

## Architecture Decisions

**Zero external dependencies** — the entire system is pure Go stdlib. No client libraries for Prometheus, no ORM, no framework. This means no dependency supply-chain risk, trivially fast builds, and a 7MB binary.

**Async persistence, sync control** — PostgreSQL writes are fully off the hot path. The control tick never waits for I/O. If the database is unreachable, the system continues operating and logs the gap.

**Drop-oldest simulation** — when the simulation engine cannot finish within `SIM_BUDGET`, it drops the oldest pending result rather than blocking. The control loop always has *some* simulation output, even if it is one tick stale.

**Hysteresis everywhere** — mode transitions (Normal → Degraded → Emergency), reasoning event cooldowns, PID deadband, and direction-change cooldown all use hysteresis. This is the primary mechanism for preventing the oscillation that would otherwise arise from a purely threshold-reactive system.

---

## License

See `LICENSE` for terms.