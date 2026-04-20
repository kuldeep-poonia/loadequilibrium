# LoadEquilibrium: Complete Project Structure Exploration

**Last Updated**: April 19, 2026  
**Version**: Schema V3  
**Purpose**: Comprehensive documentation for complete UI redesign

---

## Table of Contents
1. [Architecture Overview](#architecture-overview)
2. [Backend API Endpoints](#backend-api-endpoints)
3. [Data Models & Types](#data-models--types)
4. [WebSocket Streaming](#websocket-streaming)
5. [Frontend Components](#frontend-components)
6. [State Management](#state-management)
7. [Configuration & Environment](#configuration--environment)
8. [File Organization](#file-organization)
9. [Data Flow Diagrams](#data-flow-diagrams)

---

## Architecture Overview

### System Stack
```
┌─────────────────────────────────────┐
│  Dashboard (Next.js 16 + React 19)  │  TypeScript, Tailwind CSS 4
├─────────────────────────────────────┤
│  WebSocket Hub + REST API Server    │  Go 1.22, custom WS implementation
├─────────────────────────────────────┤
│  Runtime Orchestrator (10Hz Loop)   │  Pipeline stages, state broadcast
├─────────────────────────────────────┤
│  Intelligence Engine                │  Topology, dynamics, control
├─────────────────────────────────────┤
│  Telemetry Ring Buffer + Metrics    │  Per-service time-series storage
└─────────────────────────────────────┘
```

### Core Technologies
- **Backend**: Go 1.22 (minimal external dependencies)
- **Frontend**: Next.js 16.2.2, React 19.2.4
- **State**: Zustand 5.0.12 (client-side only)
- **WebSocket**: Custom implementation (RFC 6455 compliant)
- **UI Framework**: Tailwind CSS 4, Framer Motion
- **Charting**: Recharts 3.8.1
- **Icons**: Lucide React 1.7.0

### Key Design Principles
1. **Real-time streaming** - 10Hz tick-based state broadcast
2. **Schema versioning** - TickPayload schema version 3
3. **Memory efficient** - Ring buffers, EWMA smoothing, 60-point history
4. **Fault tolerant** - Signal quality indicators, confidence scores
5. **Numerical stability** - NaN/Inf sanitization before broadcast

---

## Backend API Endpoints

### 1. Telemetry Ingestion

#### `POST /api/v1/ingest`
**Purpose**: Accept metric points from observability agents  
**Authentication**: Optional `X-Ingest-Token` or `Authorization: Bearer`

**Request Format** (JSON):
```json
// Single metric point
{
  "service_id": "api-server",
  "timestamp": "2024-04-19T12:34:56Z",
  "request_rate": 1234.5,
  "error_rate": 0.002,
  "latency": {
    "p50": 45.2,
    "p95": 125.3,
    "p99": 342.1,
    "mean": 67.8
  },
  "cpu_usage": 0.65,
  "mem_usage": 0.42,
  "active_conns": 256,
  "queue_depth": 12,
  "upstream_calls": [
    {
      "target_service_id": "db-service",
      "call_rate": 456.2,
      "error_rate": 0.001,
      "latency_mean": 23.4
    }
  ]
}

// Or batch
[
  { service_id: "svc1", ... },
  { service_id: "svc2", ... }
]
```

**Response** (202 Accepted):
```json
{
  "status": "accepted",
  "points": 2
}
```

**Validation Rules**:
- `service_id` is required (non-empty string)
- Request/error rates clamped to [0, ∞) and [0, 1] respectively
- Latency minimum floor: 0.1ms (prevents div-by-zero)
- Services limited to `MaxServices` config (LRU replacement)

---

### 2. Control Plane: Actuation

#### `POST /api/v1/control/toggle`
**Purpose**: Enable/disable automated control actuation

**Request**:
```json
{
  "enabled": true
}
```

**Response** (200 OK):
```json
{
  "status": "applied",
  "action": "toggle",
  "actuation_enabled": true,
  "actuator_configured": true,
  "control_plane": { /* ControlPlaneState */ }
}
```

---

#### `POST /api/v1/control/chaos-run`
**Purpose**: Apply windowed service disturbance (load spike)

**Request**:
```json
{
  "service_id": "payment-service",
  "duration_ticks": 50,
  "request_factor": 3.5,
  "latency_factor": 2.0
}
```

**Behavior**:
- Multiplies arrival rate by `request_factor` for `duration_ticks`
- Multiplies latency by `latency_factor`
- Bounded: duration [1, 600] ticks, factors [1.0, 10.0]
- Falls back to service_id = "all" if not specified

**Response** (202 Accepted):
```json
{
  "status": "scheduled",
  "action": "chaos-run",
  "target_service": "payment-service",
  "start_tick": 1234,
  "until_tick": 1284,
  "request_factor": 3.5,
  "latency_factor": 2.0,
  "scenario_mode": "on",
  "control_plane": { /* ControlPlaneState */ }
}
```

---

#### `POST /api/v1/control/replay-burst`
**Purpose**: Replay burst scenario (resettable disturbance)

**Request**:
```json
{
  "service_id": "cache-layer",
  "duration_ticks": 20,
  "factor": 2.0
}
```

**Response** (202 Accepted):
Similar to chaos-run; duration bounded [1, 600], factor [1.0, 10.0]

---

### 3. Domain-Specific Control

#### `POST /api/v1/policy/update`
**Purpose**: Apply control policy preset

**Request**:
```json
{
  "preset": "aggressive"  // or "conservative", "balanced"
}
```

**Response** (200 OK):
```json
{
  "status": "applied",
  "domain": "policy_update",
  "preset": "aggressive",
  "control_plane": { /* ControlPlaneState */ }
}
```

---

#### `POST /api/v1/runtime/step`
**Purpose**: Force a single orchestrator tick immediately

**Request**: `{}` (optional, no parameters)

**Response** (200 OK | 409 Conflict):
```json
{
  "status": "executed",  // or "busy" if tick in flight
  "domain": "runtime_step",
  "tick": 5678,
  "control_plane": { /* ControlPlaneState */ }
}
```

**Error Codes**:
- 409: Tick already in flight (concurrent step)
- 503: Orchestrator offline

---

#### `POST /api/v1/sandbox/trigger`
**Purpose**: Enqueue what-if analysis sandbox experiment

**Request**:
```json
{
  "type": "experiment",
  "duration_ticks": 10
}
```

**Response** (202 Accepted):
```json
{
  "status": "scheduled",
  "domain": "sandbox_trigger",
  "type": "experiment",
  "until_tick": 5688,
  "duration_ticks": 10,
  "control_plane": { /* ControlPlaneState */ }
}
```

**Duration**: Bounded [1, 120] ticks

---

#### `POST /api/v1/simulation/control`
**Purpose**: Manage forward simulation runner (Monte-Carlo scenarios)

**Request**:
```json
{
  "action": "run",        // "run" | "start" | "force" | "reset" | "stop"
  "duration_ticks": 50
}
```

**Action Semantics**:
- `run`, `start`, `force`: Force simulation until `duration_ticks` ahead
- `reset`: Reset simulation state, clear history
- `stop`: Clear forced window (return to normal operation)

**Response** (200 OK | 202 Accepted):
```json
{
  "status": "scheduled",  // or "applied" for stop
  "domain": "simulation_control",
  "action": "run",
  "until_tick": 5728,
  "duration_ticks": 50,
  "control_plane": { /* ControlPlaneState */ }
}
```

---

#### `POST /api/v1/intelligence/rollout`
**Purpose**: Trigger reinforcement learning rollout evaluation

**Request**:
```json
{
  "duration_ticks": 10
}
```

**Response** (202 Accepted):
```json
{
  "status": "scheduled",
  "domain": "intelligence_rollout",
  "until_tick": 5738,
  "duration_ticks": 10,
  "control_plane": { /* ControlPlaneState */ }
}
```

**Duration**: Bounded [1, 120] ticks

---

#### `POST /api/v1/alerts/ack`
**Purpose**: Acknowledge an alert/event

**Request**:
```json
{
  "alert_id": "evt-2024-04-19-001"
}
```

**Response** (200 OK):
```json
{
  "status": "applied",
  "domain": "alert_ack",
  "alert_id": "evt-2024-04-19-001",
  "acknowledged_alert_count": 3,
  "control_plane": { /* ControlPlaneState */ }
}
```

---

### 4. Data Retrieval

#### `GET /api/v1/snapshot`
**Purpose**: Retrieve last cached TickPayload as REST endpoint

**Response** (200 OK):
```json
{
  "type": "tick",
  "seq": 5678,
  "ts": "2024-04-19T12:34:56Z",
  "schema_version": 3,
  "bundles": { /* ServiceBundle map */ },
  "topology": { /* Topology */ },
  "objective": { /* ObjectiveScore */ },
  ... /* Full TickPayload schema */
}
```

**Error**:
- 503 Service Unavailable: No tick payload cached yet

---

#### `GET /health`
**Purpose**: Health check endpoint

**Response** (200 OK):
```json
{
  "status": "ok",
  "component": "api_headless",
  "clients": 3
}
```

---

### 5. WebSocket

#### `GET /ws`
**Purpose**: WebSocket upgrade to persistent real-time stream

**Protocol**: RFC 6455 compliant  
**Broadcast Frequency**: 10Hz (100ms interval)

**Connection Details**:
- Read buffer: 1KB
- Write buffer: 1KB
- Pong wait: 60s
- Ping interval: 15s
- Max clients: Configurable (default 50)

---

## Data Models & Types

### 1. Telemetry Input Model

#### `MetricPoint` (JSON ingestion)
```typescript
interface MetricPoint {
  service_id: string;           // Required
  timestamp: string;             // ISO 8601, optional (defaults to now)
  request_rate: number;          // requests/second
  error_rate: number;            // [0, 1] fraction
  latency: {
    p50: number;                 // ms
    p95: number;                 // ms
    p99: number;                 // ms
    mean: number;                // ms
  };
  cpu_usage: number;             // [0, 1] fraction or % 
  mem_usage: number;             // [0, 1] fraction or %
  active_conns: number;          // integer count
  queue_depth: number;           // integer count
  upstream_calls?: {             // Optional dependency tracking
    target_service_id: string;
    call_rate: number;
    error_rate: number;
    latency_mean: number;
  }[];
}
```

**Sanitization Rules**:
- Negative rates clamped to 0
- Error rate clamped to [0, 1]
- All latency values with mean < 0.1ms floored to 0.1ms (division protection)
- Services exceeding `MaxServices` config dropped (LRU replacement)

---

### 2. Aggregated Service Models

#### `ServiceWindow` (Computed per service)
```typescript
interface ServiceWindow {
  // Identity & timing
  service_id: string;
  computed_at: Date;
  last_observed_at: Date;
  sample_count: number;

  // Request statistics
  mean_request_rate: number;      // requests/sec
  std_request_rate: number;        // stddev
  last_request_rate: number;       // latest sample
  
  // Latency statistics
  mean_latency_ms: number;         // averaged
  max_latency_ms: number;          // peak observed
  last_latency_ms: number;         // latest sample
  last_p99_latency_ms: number;    // latest p99
  
  // Error & resource
  mean_error_rate: number;         // [0, 1]
  last_error_rate: number;         // [0, 1]
  mean_cpu: number;                // [0, 1]
  mean_mem: number;                // [0, 1]
  
  // Queue state
  mean_queue_depth: number;        // average queue length
  last_queue_depth: number;        // current queue
  mean_active_conns: number;       // average connections
  
  // Control signal
  applied_scale: number;           // Scaling factor from controller [0, ∞)
  
  // Signal quality assessment
  confidence_score: number;        // [0, 1] composite signal quality
  signal_quality: "good" | "degraded" | "sparse";
  
  // Physics engine state (injected from simulation)
  hazard: number;                  // Risk level
  reservoir: number;               // Stability buffer
  
  // Dependency tracking
  upstream_edges: {
    [target_service_id: string]: EdgeWindow;
  };
}

interface EdgeWindow {
  target_service_id: string;
  mean_call_rate: number;
  mean_error_rate: number;
  mean_latency_ms: number;
}
```

---

### 3. Service Bundle (Per-Service Multi-Model)

Complete state for a single service across 4 model layers:

#### `QueueModel`
```typescript
interface QueueModel {
  service_id: string;
  arrival_rate: number;            // λ (requests/sec)
  service_rate: number;            // μ (throughput/sec)
  concurrency: number;             // c (worker count)
  utilisation: number;             // ρ = λ/(c·μ) [0, 1]
  mean_queue_len: number;          // L_q (M/M/c formula)
  mean_wait_ms: number;            // W_q
  mean_sojourn_ms: number;         // W (system time)
  burst_factor: number;            // Burstiness multiplier
  adjusted_wait_ms: number;        // With bursts
  utilisation_trend: number;       // dρ/dt trend
  confidence: number;              // [0, 1] data quality
  upstream_pressure: number;       // Cascaded load from dependencies
  hazard: number;                  // Risk score from physics
  reservoir: number;               // Stability capacity
  last_p99_latency_ms: number;    // Worst-case latency SLA
}
```

#### `StabilityModel`
```typescript
interface StabilityModel {
  service_id: string;
  stability_margin: number;        // Distance to collapse (ms)
  collapse_risk: number;           // [0, 1] probability
  oscillation_risk: number;        // Queue oscillation propensity
  feedback_gain: number;           // Control loop gain
  is_unstable: boolean;            // Critical flag
  predicted_collapse_ms: number;  // Time to saturation
  cascade_amplification_score: number;  // Downstream risk
  collapse_zone: "safe" | "warning" | "collapse";
  trend_adjusted_margin: number;  // Margin accounting for trends
  stability_derivative: number;    // d(margin)/dt (drift)
}
```

#### `StochasticModel`
```typescript
interface StochasticModel {
  arrival_co_v: number;            // Coefficient of variation (burstiness)
  burst_amplification: number;    // Burst impact multiplier
  risk_propagation: number;       // Cascade risk factor
  confidence: number;             // [0, 1] model certainty
}
```

#### `SignalModel`
```typescript
interface SignalModel {
  fast_ewma: number;              // Fast exponential moving average
  slow_ewma: number;              // Slow exponential moving average
  ewma_variance: number;          // Signal variability
  cusum_pos: number;              // Positive cumulative sum (trend up)
  cusum_neg: number;              // Negative cumulative sum (trend down)
  spike_detected: boolean;        // Anomaly flag
}
```

#### Combined `ServiceBundle`
```typescript
interface ServiceBundle {
  queue: QueueModel;
  stability: StabilityModel;
  stochastic: StochasticModel;
  signal: SignalModel;
}
```

---

### 4. Control & Optimization

#### `ObjectiveScore` (System-wide health metric)
```typescript
interface ObjectiveScore {
  composite_score: number;            // [0, 1] overall health
  max_collapse_risk: number;          // [0, 1] max across services
  cascade_failure_probability: number; // [0, 1] systemic risk
  predicted_p99_latency_ms: number;  // Expected worst-case
  oscillation_risk: number;           // Queue oscillation risk
  risk_acceleration: number;          // d(risk)/dt
  trajectory_score: number;           // Policy trajectory quality
  latency_weight: number;             // Objective weighting
  utilisation_weight: number;         // Objective weighting
  risk_weight: number;                // Objective weighting
  reference_latency_ms: number;      // SLA baseline
  trend_stability_margin: number;    // Forward-looking margin
}
```

#### `ControlDirective` (Per-service actuation)
```typescript
interface ControlDirective {
  service_id?: string;
  computed_at?: string;
  
  // PID Control Output
  scale_factor: number;              // Actuation multiplier [0, ∞)
  target_utilisation: number;        // Setpoint ρ_target
  error: number;                     // ρ_current - ρ_target
  pid_output: number;                // PID controller output
  active: boolean;                   // Control enabled
  stability_margin: number;          // Safety threshold
  hysteresis_state?: string;         // Control state machine
  actuation_bound?: number;          // Clamp on scale_factor
  
  // Predictive Control (MPC)
  predictive_target?: number;        // Forward-looking setpoint
  mpc_predicted_rho?: number;       // Predicted ρ at horizon
  mpc_overshoot_risk?: boolean;     // Risk of overshooting
  mpc_underactuation_risk?: boolean; // Risk of under-actuation
  
  // Optimization details
  cost_gradient?: number;            // Objective gradient
  trajectory_cost_avg?: number;      // Average trajectory cost
  max_trajectory_cost?: number;      // Peak trajectory cost
  planner_scale_factor?: number;    // Planner recommended scale
  planner_convergent?: boolean;     // Convergence status
  planner_convex?: boolean;         // Problem convexity
  planner_probabilistic_score?: number;  // MC scenario score
}
```

#### `Event` (Reasoning & alerts)
```typescript
interface Event {
  id?: string;
  timestamp: string;
  category: string;                  // "cascade", "saturation", "anomaly", etc.
  description: string;
  severity: string;                  // "critical", "warning", "info"
  service_id?: string;
  evidence?: {
    utilisation: number;
    collapse_risk: number;
    oscillation_risk: number;
    queue_wait_ms: number;
    saturation_sec: number;
    burst_factor: number;
    cascade_risk: number;
    stability_margin: number;
    composite_score: number;
  };
  uncertainty_score?: number;        // [0, 1] confidence
}
```

---

### 5. Topology Models

#### `Node` (Service vertex)
```typescript
interface Node {
  service_id: string;
  normalised_load: number;          // [0, 1] relative load
  last_seen?: string;               // Timestamp
}
```

#### `Edge` (Service dependency)
```typescript
interface Edge {
  source: string;                    // Service A
  target: string;                    // Service B
  weight: number;                    // Coupling strength
  call_rate: number;                // requests/sec
  error_rate: number;               // [0, 1]
  latency_ms: number;               // ms
  last_updated?: string;            // Timestamp
}
```

#### `CriticalPath` (Cascade route)
```typescript
interface CriticalPath {
  nodes: string[];                  // Service sequence most at risk
  total_weight: number;             // Cumulative coupling weight
  cascade_risk: number;             // [0, 1] failure propagation risk
}
```

#### `Topology` (Full graph snapshot)
```typescript
interface Topology {
  captured_at: string;
  nodes: Node[];
  edges: Edge[];
  critical_path: CriticalPath;
}
```

#### `TopologyDiff` (Incremental updates)
```typescript
interface TopologyDiff {
  schema: number;
  is_full: boolean;                 // true = full snapshot, false = delta
  added_nodes?: Node[];
  removed_nodes?: string[];         // Service IDs
  updated_nodes?: Node[];
  added_edges?: Edge[];
  removed_edges?: string[];         // Edge IDs or descriptions
  updated_edges?: Edge[];
}
```

---

### 6. Risk & Prediction Models

#### `RiskQueueItem` (Priority-ordered risk)
```typescript
interface RiskQueueItem {
  service_id: string;
  urgency_score: number;            // [0, 1] composite urgency
  collapse_risk: number;            // [0, 1] collapse probability
  rho: number;                      // Current utilisation
  is_keystone: boolean;             // Critical dependency
  path_collapse_prob: number;       // Cascade failure probability
  urgency_class: "critical" | "warning" | "elevated" | "nominal";
}
```

#### `PredictionPoint` (Single point on utilisation curve)
```typescript
interface PredictionPoint {
  t: number;                        // Tick offset from now
  rho: number;                      // Predicted utilisation
  lo: number;                       // 95% CI lower bound
  hi: number;                       // 95% CI upper bound
}
```

#### `RiskTimelinePoint` (Per-service risk trajectory)
```typescript
interface RiskTimelinePoint {
  t: number;                        // Tick offset
  rho: number;                      // Predicted utilisation
  collapse_risk: number;            // [0, 1] risk at this tick
}
```

#### `PredictiveRiskTimeline` (Service → timeline map)
```typescript
type PredictiveRiskTimeline = {
  [service_id: string]: RiskTimelinePoint[];
};
```

---

### 7. Runtime State

#### `ControlPlaneState` (API-exposed orchestrator state)
```typescript
interface ControlPlaneState {
  tick: number;                     // Current tick counter
  actuation_enabled: boolean;       // Control on/off
  policy_preset: string;            // Current policy mode
  forced_sandbox_until: number;    // Sandbox forced until this tick
  forced_simulation_until: number;  // Simulation forced until this tick
  forced_intelligence_until: number; // RL forced until this tick
  simulation_reset_pending: boolean;
  acknowledged_alert_count: number;
}
```

#### `RuntimeMetrics` (Pipeline stage timings - EWMA smoothed)
```typescript
interface RuntimeMetrics {
  avg_prune_ms: number;            // Metric ring buffer pruning
  avg_windows_ms: number;          // Service window aggregation
  avg_topology_ms: number;         // Topology analysis
  avg_coupling_ms: number;         // Network coupling
  avg_modelling_ms: number;        // System identification
  avg_optimise_ms: number;         // Control optimization
  avg_sim_ms: number;              // Forward simulation
  avg_reasoning_ms: number;        // Reasoning engine (events)
  avg_broadcast_ms: number;        // WebSocket broadcast
  predicted_critical_ms: number;  // Predicted worst-case stage
}
```

---

## WebSocket Streaming

### Connection Lifecycle

```
Client                              Server
  |                                   |
  +-------- Upgrade Request -------->|
  |                                   |
  |<----- 101 Switching Protocols ----|
  |                                   |
  |<---- TickPayload (type: tick) ----|  10Hz
  |<---- TickPayload (type: tick) ----|
  |         ...                       |
  |                                   |
  |<---- Ping Frame ------------------|  Every 15s
  |---- Pong Frame ----------------->|
  |                                   |
  +------- Close Frame ------------->|
  |                                   |
```

### Message Format

#### Tick Message
```typescript
interface WebSocketMessage {
  type: "tick";
  seq: number;                      // Sequence number (monotonic)
  ts: string;                       // ISO 8601 timestamp
  schema_version: 3;                // Schema version (for breaking changes)
  
  // Core models
  bundles: {
    [serviceId: string]: ServiceBundle;
  };
  topology: Topology;
  topo_diff: TopologyDiff;
  objective: ObjectiveScore;
  directives: {
    [serviceId: string]: ControlDirective;
  };
  events: Event[];
  sim_result?: SimulationResult;
  
  // Control room overlay fields
  degraded_services: string[];      // Services in degraded state
  sat_countdowns: {                 // Saturation countdown (ticks)
    [serviceId: string]: number;
  };
  stability_zones: {                // Service stability classification
    [serviceId: string]: string;
  };
  prediction_horizon: {             // Prediction window per service
    [serviceId: string]: number;
  };
  
  // Prediction curves (N-tick forward projection)
  prediction_timeline: {
    [serviceId: string]: PredictionPoint[];
  };
  
  // Runtime health metrics
  tick_health_ms: number;           // Time to execute this tick
  degraded_fraction: number;        // % of services degraded
  safety_mode: boolean;             // Safety override active
  jitter_ms: number;                // Tick timing variance
  runtime_metrics: RuntimeMetrics;  // Stage timings
  control_plane: ControlPlaneState; // Current control state
  
  // Advanced analytics
  network_coupling: {               // Per-service equilibrium coupling
    [serviceId: string]: NetworkCouplingSnapshot;
  };
  network_equilibrium: NetworkEquilibriumSnapshot;
  topology_sensitivity: TopologySensitivitySnapshot;
  
  // Risk analysis
  priority_risk_queue: RiskQueueItem[];  // Ordered by urgency
  pressure_heatmap: {                    // Service pressure [0, 1]
    [serviceId: string]: number;
  };
  sim_overlay?: SimOverlayState;
  fixed_point_equilibrium: FixedPointSnapshot;
  scenario_comparison?: ScenarioComparisonSnapshot;
  risk_timeline: PredictiveRiskTimeline;
  stability_envelope: StabilityEnvelopeSnapshot;
}
```

#### Ping Message
```typescript
interface PingMessage {
  type: "ping";
}
```

### Broadcast Frequency & Characteristics

| Aspect | Value | Notes |
|--------|-------|-------|
| Base Frequency | 10 Hz | 100ms intervals |
| Message Size | ~50-150 KB | Varies with service count |
| Tick Sequence | Monotonic uint64 | Detects dropped messages |
| Client Limit | 50 (default) | Configurable, scales to ~200+ |
| Ping Interval | 15s | Keep-alive for NAT/proxies |
| Pong Wait | 60s | Connection timeout |
| Read Buffer | 1 KB | Per client |
| Write Buffer | 1 KB | Per client |

### Zero-Allocation Broadcast

- Latest `TickPayload` cached atomically in hub
- Pre-marshaled JSON serialized and cached
- Clients receive pre-serialized bytes (no per-client marshaling)
- Reduces memory allocation from O(n_clients) to O(1)

---

## Frontend Components

### 1. Layout Components

#### `DashboardShell` (Main layout container)
**File**: `dashboard/src/components/layout/DashboardShell.tsx`

**Exports**:
- `Sidebar()` - Left navigation panel
- `Header()` - Top status bar with clock
- `KpiBar()` - Key performance indicators
- `DashboardShell` - Main layout wrapper

**Features**:
- 3-column grid layout (sidebar, content, KPI bar)
- Real-time clock display (updates every 100ms)
- Neumorphic design system (Apple-inspired)
- Responsive breakpoints (xl: 1280px+)

**Props**:
```typescript
DashboardShell({ children: React.ReactNode })
```

---

### 2. Command Center (Main UI)

#### `CommandCenter` (Dashboard main component)
**File**: `dashboard/src/components/control-room/CommandCenter.tsx`

**Key Sections**:
1. **Stat Cards** - System health metrics
   - Composite objective score
   - Cascade failure probability
   - P99 latency prediction
   - System pressure

2. **Stage Pills** - Runtime pipeline timings (9 stages)
   - Prune, Windows, Topology, Coupling
   - Modelling, Optimise, Simulation, Reasoning, Broadcast

3. **Trend Chart** - 60-frame historical charts
   - Objective score trend
   - Cascade risk trend
   - Tick execution time trend

4. **Risk Bars** - Service urgency ranking
   - Top 3-5 services ordered by collapse risk

5. **Service List** - Per-service detailed metrics
   - Service ID, utilisation, confidence
   - Drift (d(margin)/dt), stability margin
   - Queue depth, signal quality

6. **Control Buttons** - Action triggers
   - Toggle (enable/disable control)
   - Chaos Run (disturbance)
   - Replay Burst (burst scenario)
   - Sandbox (what-if analysis)
   - Simulation Control (forward model)

7. **Alert Panel** - Event log with acknowledgment

**State Integration**:
```typescript
const store = useTelemetryStore();
tick = store.tick           // Latest TickPayload
history = store.history    // 60-point history
connected = store.connected
triggerAction(action)       // Trigger API call
```

---

### 3. Topology Visualization

#### `TopologyCanvas` (Real-time graph rendering)
**File**: `dashboard/src/components/control-room/TopologyModule.tsx`

**Technology**: Canvas 2D (60fps target)

**Features**:
- Force-directed layout (physics simulation)
- Real-time node/edge animation
- Service status coloring (nominal/warning/critical)
- Dependency edge weight visualization
- Mouse hover tooltips
- Zoom & pan support

**Rendering Loop**:
```typescript
requestAnimationFrame(render)  // 60fps, not locked to tick rate
```

**Node Rendering**:
- Circle positioned by physics simulation
- Color: cyan (nominal) → amber (warning) → red (critical)
- Size: Proportional to normalised_load
- Label: Service ID

**Edge Rendering**:
- Line thickness: Proportional to call_rate
- Color: Gray (nominal) → red (high error rate)
- Dashing: Indicates error state

---

### 4. UI Primitives

#### `HUD` Component Library
**File**: `dashboard/src/components/ui/HUD.tsx`

**Exports**:

##### `cn()` - Class merger
```typescript
cn(...inputs: ClassValue[]): string
// Combines clsx + tailwind-merge
```

##### `TacticalBox()` - Primary panel frame
```typescript
<TacticalBox
  title="System Health"
  badge="CRITICAL"
  status="critical"
  scan
>
  {children}
</TacticalBox>
```

**Props**:
- `title?: string` - Panel header text
- `badge?: string` - Badge label
- `status?: 'nominal' | 'warning' | 'critical' | 'alert'`
- `scan?: boolean` - Top scanline animation
- `className?: string` - Additional Tailwind classes

**Styling**:
- Neumorphic design with soft shadows
- Border accent line (top-left) in status color
- Rounded corners (16px)
- Padding: 14px outer, 12px inner

---

##### `StatusTube()` - Progress gauge
```typescript
<StatusTube
  label="System Utilisation"
  value="0.752"
  percent={0.752}
  status="warning"
  vertical={false}
/>
```

**Props**:
- `label: string`
- `value: string | number`
- `percent: number` - [0, 1]
- `vertical?: boolean` - Orientation
- `status?: 'nominal' | 'warning' | 'critical'`

---

##### `DigitalLog()` - Console output
```typescript
<DigitalLog lines={["[0000] System initialized", "[0001] Service discovered"]} />
```

**Features**:
- Monospace font (var(--font-data))
- Line numbers in gray
- Error highlighting (red for ERR/CRIT)
- Blinking cursor
- Auto-scroll to latest

---

##### `DataGridItem()` - Data cell
```typescript
<DataGridItem label="Requests/sec" value="1234.5" />
```

**Styling**: Compact label + value display

---

### 5. Component Tree

```
ControlRoom
└── CommandCenter
    ├── StatCard (x4)
    │   ├── Composite Score
    │   ├── Cascade Risk
    │   ├── P99 Latency
    │   └── System Pressure
    ├── StagePill (x9) → RuntimeMetrics
    ├── TrendChart
    │   ├── AreaChart (Objective)
    │   ├── AreaChart (Cascade)
    │   └── AreaChart (TickMs)
    ├── RiskBars
    ├── ServiceList
    │   └── Service items (signal quality, margin, etc.)
    ├── ControlButton (x5)
    │   ├── Toggle
    │   ├── ChaosRun
    │   ├── ReplayBurst
    │   ├── Sandbox
    │   └── SimControl
    ├── TopologyCanvas
    └── DigitalLog (events)
```

---

## State Management

### Zustand Store: `useTelemetryStore`

**File**: `dashboard/src/store/useTelemetryStore.ts`

#### State Interface
```typescript
interface TelemetryState {
  // Data
  tick: TickPayload | null;
  history: HistoryPoint[];         // Max 60 points
  
  // Connection
  connected: boolean;
  lastTickMs: number;              // Timestamp of last tick
  tickAge: number;                 // ms since last tick
  
  // Actions
  setTick(tick: TickPayload): void;
  setConnected(connected: boolean): void;
  reset(): void;
  triggerAction(action: string): Promise<void>;
  triggerDomain(domain: string, payload?: DomainPayload): Promise<ApiResult>;
}
```

#### `HistoryPoint` Structure
```typescript
interface HistoryPoint {
  seq: number;
  obj: number;                    // Objective score
  casc: number;                   // Cascade probability
  p99: number;                    // P99 latency
  rhoMean: number;                // Mean utilisation
  tickMs: number;                 // Tick execution time
  throughput: number;             // Total requests/sec
  queueDepth: number;             // Total queue depth
  workers: number;                // Total workers
}
```

#### Computed from ServiceBundle
```typescript
// Derived on each tick update
throughput = sum(bundle.queue.arrival_rate)
queueDepth = sum(bundle.queue.mean_queue_len)
workers = sum(bundle.queue.concurrency)
```

#### `setTick()` Logic
```typescript
setTick: (tick) => {
  if (store.tick?.seq === tick.seq) return; // Skip duplicate
  
  const now = Date.now();
  
  // Extract metrics from bundles
  let throughput = 0, queueDepth = 0, workers = 0;
  for (const serviceId in tick.bundles) {
    const queue = tick.bundles[serviceId]?.queue;
    if (queue) {
      throughput += queue.arrival_rate ?? 0;
      queueDepth += queue.mean_queue_len ?? 0;
      workers += queue.concurrency ?? 0;
    }
  }
  
  // Create history point
  const historyPoint: HistoryPoint = {
    seq: tick.seq,
    obj: tick.objective?.composite_score ?? 0,
    casc: tick.objective?.cascade_failure_probability ?? 0,
    p99: tick.objective?.predicted_p99_latency_ms ?? 0,
    rhoMean: tick.network_equilibrium?.system_rho_mean ?? 0,
    tickMs: tick.tick_health_ms ?? 0,
    throughput,
    queueDepth,
    workers,
  };
  
  // Append to history (limit 60 points)
  const newHistory = [...state.history, historyPoint].slice(-60);
  
  return {
    tick,
    history: newHistory,
    lastTickMs: now,
    tickAge: 0,
  };
};
```

#### `triggerAction()` Implementation
```typescript
triggerAction: async (action: string) => {
  try {
    const res = await fetch(`${API_BASE_URL}/api/v1/control/${action}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
    });
    // ... handle response
  } catch (error) {
    // ... error handling
  }
};
```

#### `triggerDomain()` Implementation
```typescript
triggerDomain: async (domain: string, payload?: DomainPayload) => {
  try {
    const res = await fetch(`${API_BASE_URL}/api/v1/${domain}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: payload ? JSON.stringify(payload) : undefined,
    });
    const data = await res.json();
    const error = responseError(data);
    return error ? { ok: false, error } : { ok: true, data };
  } catch (error) {
    return { ok: false, error: String(error) };
  }
};
```

---

## Configuration & Environment

### Backend Configuration

**File**: `internal/config/config.go`

```go
type Config struct {
  // Networking
  ListenAddr string = ":8080"
  
  // Tick timing
  TickInterval time.Duration = 100ms    // 10Hz
  TickDeadline time.Duration = 500ms    // Max execution time
  
  // Telemetry buffering
  RingBufferDepth int = 512             // Metric history per service
  MaxServices int = 500                 // Max concurrent services
  StaleServiceAge time.Duration = 5m    // Prune inactive services
  
  // Persistence
  DatabaseURL string                    // Optional PostgreSQL
  PersistInterval time.Duration = 1s
  
  // Simulation
  SimBudget time.Duration = 50ms        // CPU budget per tick
  SimHorizonMs float64 = 5000           // 5-second projection
  SimShockFactor float64 = 2.0          // Disturbance amplitude
  SimAsyncBuffer int = 10               // Async simulation queue
  
  // Control tuning
  UtilisationSetpoint float64 = 0.7    // Target ρ
  CollapseThreshold float64 = 0.95     // Critical threshold
  
  // Signal processing
  WindowFraction float64 = 0.2
  EWMAFastAlpha float64 = 0.1
  EWMASlowAlpha float64 = 0.01
  SpikeZScore float64 = 2.5
  
  // PID control gains
  PIDKp float64 = 0.2
  PIDKi float64 = 0.05
  PIDKd float64 = 0.1
  PIDDeadband float64 = 0.05
  PIDIntegralMax float64 = 0.5
  
  // WebSocket
  MaxStreamClients int = 50
  
  // Algorithm selection
  ArrivalEstimatorMode string = "ewma"         // or "median"
  PredictiveHorizonTicks int = 50
  MaxReasoningCooldowns int = 1000
  SimStochasticMode string = "exponential"     // or "pareto"
}
```

### Environment Variables

```bash
# API
LISTEN_ADDR=:8080

# Tick timing
TICK_INTERVAL_MS=100
TICK_DEADLINE_MS=500

# Telemetry
RING_BUFFER_DEPTH=512
MAX_SERVICES=500
STALE_SERVICE_AGE_SEC=300

# Persistence
DATABASE_URL=postgresql://...

# Control
UTILISATION_SETPOINT=0.7
COLLAPSE_THRESHOLD=0.95

# Actuator backends
ACTUATOR_HTTP_ENDPOINT=http://load-balancer:8000
ACTUATOR_HTTP_SERVICES=api-server,payment-service

# Security
INGEST_TOKEN=secret-token-12345

# WebSocket
MAX_STREAM_CLIENTS=50

# Frontend
NEXT_PUBLIC_API_BASE_URL=http://localhost:8080
NEXT_PUBLIC_WS_URL=ws://localhost:8080/ws
```

### Frontend Configuration

**File**: `dashboard/src/lib/config.ts`

```typescript
const DEFAULT_API_BASE_URL = 'http://localhost:8080';

export const API_BASE_URL = trimTrailingSlash(
  process.env.NEXT_PUBLIC_API_BASE_URL || DEFAULT_API_BASE_URL
);

export const WS_URL = trimTrailingSlash(
  process.env.NEXT_PUBLIC_WS_URL || toWebSocketUrl(API_BASE_URL)
);

// Auto-converts:
// http://localhost:8080 → ws://localhost:8080/ws
// https://example.com → wss://example.com/ws
```

---

## File Organization

### Backend Structure

```
internal/
├── api/
│   └── server.go                   # HTTP server + routing
├── ws/
│   └── websocket.go               # RFC 6455 implementation
├── streaming/
│   ├── hub.go                     # WebSocket broadcast hub
│   ├── types.go                   # TickPayload schema (v3)
│   ├── diff.go                    # Topology diff generation
│   └── predict.go                 # Prediction curves
├── telemetry/
│   ├── store.go                   # Ring buffer + ingestion
│   ├── types.go                   # MetricPoint, ServiceWindow
│   ├── ringbuffer.go              # Circular buffer impl
│   └── store_test.go
├── runtime/
│   ├── orchestrator.go            # 10Hz tick loop
│   ├── phase_runtime.go           # Pipeline stages
│   ├── simulation_bundles.go      # Simulation state
│   └── orchestrator_race_test.go
├── config/
│   └── config.go                  # Configuration loading
├── control/                        # MPC + optimization
│   ├── policy_controller.go
│   ├── bundle_generator.go
│   ├── horizon_cost.go
│   ├── coordinated_optimizer.go
│   └── ...
├── topology/                       # Graph analysis
├── dynamics/                       # Queueing theory
├── simulation/                     # Forward simulation
├── intelligence/                   # Reinforcement learning
├── autopilot/                      # Stability control
├── reasoning/                      # Event generation
├── modelling/                      # System identification
├── optimisation/                   # Numerical optimization
├── scenario/                       # Scenario engine
├── persistence/                    # Database layer
├── actuator/                       # Control actuation
├── debug/                          # Diagnostics
└── integration/                    # Integration tests

cmd/
└── loadequilibrium/
    └── main.go                     # Entry point
```

### Frontend Structure

```
dashboard/
├── src/
│   ├── app/
│   │   ├── page.tsx              # Root page
│   │   ├── layout.tsx            # App shell + metadata
│   │   └── globals.css           # Global styles
│   ├── components/
│   │   ├── control-room/
│   │   │   ├── CommandCenter.tsx # Main dashboard
│   │   │   ├── TopologyModule.tsx # Graph visualization
│   │   │   └── ControlRoom.tsx   # Wrapper
│   │   ├── layout/
│   │   │   └── DashboardShell.tsx # Layout primitives
│   │   └── ui/
│   │       └── HUD.tsx           # UI component library
│   ├── store/
│   │   └── useTelemetryStore.ts  # Zustand state
│   ├── types/
│   │   └── backend.ts            # TypeScript definitions
│   ├── lib/
│   │   ├── config.ts             # API/WS URLs
│   │   └── telemetry.ts          # Data normalization
│   └── hooks/                     # Custom React hooks
├── public/                        # Static assets
├── package.json
├── tsconfig.json
├── tailwind.config.ts
├── next.config.ts
├── postcss.config.mjs
└── eslint.config.mjs
```

---

## Data Flow Diagrams

### 1. Telemetry Ingestion Flow

```
Observability Agents (Prometheus, Datadog, Custom)
         |
         | POST /api/v1/ingest
         v
    HTTP Server (port 8080)
         |
         | Validate & sanitize
         v
    Telemetry Store (Ring Buffer)
         | (per-service circular buffer)
         v
    ServiceWindow Aggregation (100ms window)
         |
         +-------> Telemetry State
         v
    Orchestrator (waits for next tick)
```

### 2. Tick Processing Pipeline (10Hz)

```
┌─ Tick Loop (100ms) ─────────────────────────────┐
│                                                  │
├─ 1. Prune stale services                        │
│     └─> Prune ring buffers                      │
│                                                  │
├─ 2. Aggregate windows                           │
│     └─> ServiceWindow per service               │
│                                                  │
├─ 3. Topology analysis                           │
│     └─> Critical path, graph stability          │
│                                                  │
├─ 4. Network coupling                            │
│     └─> Service interdependencies               │
│                                                  │
├─ 5. System modelling                            │
│     └─> ServiceBundle (queue, stability, etc.)  │
│                                                  │
├─ 6. Optimization                                │
│     └─> Compute control directives              │
│                                                  │
├─ 7. Forward simulation                          │
│     └─> Predict trajectories                    │
│                                                  │
├─ 8. Reasoning engine                            │
│     └─> Generate events/alerts                  │
│                                                  │
├─ 9. Broadcast                                   │
│     ├─ Serialize TickPayload to JSON            │
│     └─ Send to all WS clients                   │
│                                                  │
└─ Next tick after 100ms ────────────────────────┘
```

### 3. WebSocket Broadcast Architecture

```
Runtime Orchestrator
         |
         | (every 10Hz)
         v
    Streaming Hub
         | (atomic store latest payload)
         v
    Pre-serialize to JSON bytes
         | (zero-allocation)
         |
    ┌────┴────┬──────┬──────┐
    v         v      v      v
  Client1  Client2 Client3 ClientN
    |        |      |       |
    +------- Atomic read ---+
             (same bytes)
```

### 4. Frontend State Update Flow

```
WebSocket.onmessage (TickPayload)
         |
         v
useTelemetryStore.setTick()
         |
    ┌────┴────────────────────┐
    v                          v
Extract metrics            Update history
(throughput, queue, etc.)   (slice to 60 points)
         |                   |
         +─────────┬─────────+
                   v
            Store state update
                   |
         ┌─────────┴──────────────┐
         v                        v
React Component                 Dashboard
Re-renders                       Updates
         |                        |
    ┌────┴────────────────────┐   |
    v                         v   v
StatCard    Trend Chart   RiskBars
```

### 5. Control Action Flow

```
Dashboard Button (e.g., "Chaos Run")
         |
         v
useTelemetryStore.triggerAction()
         |
         | POST /api/v1/control/chaos-run
         v
HTTP Server Handler
         |
    ┌────┴───────────────────────┐
    v                            v
Validate payload         Scenario engine
Bounded parameters           |
    |                        v
    +──> Set scenario overlay
         |
         v
Orchestrator
    |
    ├─ On next tick:
    │  ├─ Apply windowed disturbance
    │  ├─ Multiply load factors
    │  ├─ Trigger simulation
    │  └─ Broadcast updated state
    |
    v
WebSocket clients receive updated
     TickPayload with new predictions
```

---

## Summary: Key Integration Points for UI Redesign

### Data Sources
1. **WebSocket (Primary)** - `ws://localhost:8080/ws` (10Hz broadcast)
   - Fetch latest state: `store.tick`
   - History: `store.history` (60 points)

2. **REST Endpoints (Secondary)** - For discrete actions
   - Control actions: `/api/v1/control/*`
   - Domain APIs: `/api/v1/{policy,runtime,sandbox,simulation,intelligence,alerts}/*`
   - Snapshot fallback: `GET /api/v1/snapshot`

### Rendering Targets

| Component | Source | Update Frequency | Primary Metric |
|-----------|--------|------------------|-----------------|
| Stat Cards | `tick.objective` | 10Hz | composite_score, cascade_risk |
| Stage Pills | `tick.runtime_metrics` | 10Hz | Avg stage timings (ms) |
| Trend Chart | `history[*]` | 10Hz | obj, casc, tickMs |
| Risk Bars | `tick.priority_risk_queue` | 10Hz | urgency_score per service |
| Service List | `tick.bundles` | 10Hz | Per-service queue + stability |
| Topology | `tick.topology`, `tick.topo_diff` | 10Hz | Nodes, edges, critical path |
| Alert Panel | `tick.events` | 10Hz | Event description + severity |

### Control Points

| Action | Endpoint | State Updated |
|--------|----------|---------------|
| Toggle control | `POST /api/v1/control/toggle` | `control_plane.actuation_enabled` |
| Chaos run | `POST /api/v1/control/chaos-run` | Scenario overlay scheduled |
| Replay burst | `POST /api/v1/control/replay-burst` | Scenario overlay scheduled |
| Policy update | `POST /api/v1/policy/update` | `control_plane.policy_preset` |
| Runtime step | `POST /api/v1/runtime/step` | `control_plane.tick` |
| Sandbox trigger | `POST /api/v1/sandbox/trigger` | `control_plane.forced_sandbox_until` |
| Simulation control | `POST /api/v1/simulation/control` | `control_plane.forced_simulation_until` |
| Intelligence rollout | `POST /api/v1/intelligence/rollout` | `control_plane.forced_intelligence_until` |
| Alert acknowledge | `POST /api/v1/alerts/ack` | Event removed from `tick.events` |

### Performance Considerations
- **Message size**: 50-150 KB per tick (tunable via service count)
- **Network bandwidth**: ~5-15 Mbps @ 100 concurrent clients
- **Client memory**: ~3-5 MB per client (60-point history + current state)
- **Canvas rendering**: 60fps target with ~2000+ nodes/edges possible
- **CPU overhead**: Negligible for JSON parsing + React reconciliation on modern hardware

---

**End of Project Exploration Document**

Generated for: Complete UI Redesign Planning  
Schema Version: 3  
Backend Version: Go 1.22+  
Frontend Version: Next.js 16.2.2 + React 19.2.4
