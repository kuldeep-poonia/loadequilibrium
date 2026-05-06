# LOADEQUILIBRIUM: System Architecture & Design

> **Enterprise-grade autonomous distributed system stability engine**

## Table of Contents
1. [Executive Summary](#executive-summary)
2. [Core Concepts & Theory](#core-concepts--theory)
3. [System Architecture](#system-architecture)
4. [Data Model](#data-model)
5. [Control Algorithms](#control-algorithms)
6. [API Reference](#api-reference)
7. [Deployment Guide](#deployment-guide)

---

## Executive Summary

**LoadEquilibrium** is a real-time distributed system control plane that:

- **Predicts cascade failures** 60 seconds ahead using Monte-Carlo topology analysis
- **Maintains network equilibrium** via Gauss-Seidel algorithm solving coupled service utilization equations
- **Controls via MPC** (Model Predictive Control) with safety-constrained RL policy
- **10Hz responsiveness** enabling sub-100ms reaction to degradation events
- **Zero-copy WebSocket streaming** of 50-150KB TickPayload to dashboard every 100ms

**Why it matters**: Traditional monitoring is reactive (alerts after failure). LoadEquilibrium is predictive (prevents failure before it cascades across topology).

---

## Core Concepts & Theory

### 1. Queue Theory Foundation (M/M/c Model)

Every service is modeled as an M/M/c queue:

```
λ (arrival_rate) → [Service with c concurrency] → μ (service_rate) → Output
                   ↓ (queue accumulation)
                   L_q = utilization² / (1 - utilization)
```

**Key Metric: Utilisation ρ**
```
ρ = λ / (μ × c)    where c = concurrency

ρ < 0.70  → Safe zone (stability guaranteed)
0.70 ≤ ρ < 0.85  → Warning zone (degradation risk starts)
ρ ≥ 0.85  → Critical zone (cascade failure imminent)
ρ ≥ 1.0   → Overloaded (queue grows unbounded)
```

**Queue Depth**: L_q = (ρ²) / (1 - ρ)
- At ρ=0.70: L_q ≈ 1.6 requests queued
- At ρ=0.80: L_q ≈ 3.2 requests queued  
- At ρ=0.90: L_q ≈ 8.1 requests queued (exponential explosion)

**Latency Percentiles**:
```
P50 (median) = (1/μ) + L_q/λ
P95          = measured at 95th percentile
P99          = measured at 99th percentile (SLA critical)
```

### 2. Stability Analysis

Each service computes 4 stability metrics:

```
collapse_risk ∈ [0, 1]
  = f(ρ, oscillation_risk, burst_amplification)
  = How likely this service will degrade in next 60s
  
margin_to_saturation ∈ [0, 1]
  = (1 - ρ_effective)
  = Headroom before overload
  = Used for: SLA buffer calculation, traffic headroom
  
oscillation_risk ∈ [0, 1]
  = variance(queue_depth) / mean(queue_depth)
  = Indicates unstable feedback loops
  
spike_detection ∈ boolean
  = CUSUM anomaly detector
  = Detects sudden latency increases
```

### 3. Cascade Failure Prediction

**3-Layer Detection Model**:

**Layer 1: Per-Service Stability**
- Each service independently computes collapse_risk based on queue theory

**Layer 2: Topology Coupling**
- Service A's high queue depth → increases arrival rate for Service B
- Create dependency graph: A → B means A's queue feeds B's arrivals
- Compute "risk_propagation" = how much upstream risk affects this service

**Layer 3: Network-Level Cascade**
```
cascade_probability = compound_failure_probability(all_services)
  = 1 - ∏(1 - collapse_risk_i) × topology_amplification_factor
  
When cascade_probability > 0.70:
  → System enters CRITICAL state
  → Trigger: traffic shifting, auto-scaling, emergency throttling
```

### 4. Network Equilibrium (Gauss-Seidel)

Services are coupled: A's queue affects B's arrivals. Need GLOBAL solution.

```
ρ_i = λ_i / (μ_i × c_i)

But λ_i is not just external traffic:
λ_i = external_i + ∑(requests_from_upstream_j)

Solve iteratively:
  ρ_1 := λ_external_1 / (μ_1 × c_1)
  ρ_2 := (λ_external_2 + effect_from_ρ_1) / (μ_2 × c_2)
  ρ_3 := (λ_external_3 + effect_from_ρ_2) / (μ_3 × c_3)
  ... repeat until convergence
```

Result: Accurate system-wide utilization + latency predictions.

### 5. Control Target

**Primary Control Variable**: ρ_target = 0.70

- **Lower** (0.50): Too wasteful, unused capacity
- **0.70**: Optimal (queue theory sweet spot)
  - P99 latency is low but predictable
  - Cascades are highly unlikely
  - Enough capacity for transient spikes
- **Higher** (0.85+): Dangerous
  - Exponential latency growth
  - Cascade risk rises sharply
  - Single service failure collapses entire system

---

## System Architecture

### Component Breakdown

```
┌─────────────────────────────────────────────────────────────┐
│ DASHBOARD (Next.js React 19.2.4)                            │
│ ├─ SystemStatusBar: Live health, cascade risk, P99         │
│ ├─ ObjectiveMetricsPanel: Composite score, SLA violations  │
│ ├─ MetricsChart: 6-second health timeline (Recharts)       │
│ ├─ BundleMetricsPanel: Top 20 services by collapse_risk    │
│ ├─ TopologyInsightsPanel: Critical path, fragility         │
│ ├─ CascadePredictionPanel: 60-second forecast              │
│ ├─ AlertsQueue: Dynamic alert generation                   │
│ └─ ControlPanel: Action buttons (chaos, scaling, etc)      │
└────────────┬─────────────────────────────────────────────────┘
             │ WebSocket @10Hz (100ms)
             │ GET /api/v1/...
             │ POST /api/v1/control/...
             ▼
┌─────────────────────────────────────────────────────────────┐
│ API SERVER (Go net/http)                                    │
│ Port: 8080                                                  │
│ ├─ REST Endpoints (13 total)                               │
│ ├─ WebSocket Hub (/ws endpoint)                            │
│ ├─ CORS configured                                         │
│ └─ Zero-alloc broadcast to all clients                     │
└────────────┬─────────────────────────────────────────────────┘
             │ TickPayload (50-150KB)
             │ 40+ fields: bundles, objective, topology, predictions
             ▼
┌─────────────────────────────────────────────────────────────┐
│ RUNTIME ORCHESTRATOR (10Hz)                                │
│ ├─ Observation: Ingest telemetry from services             │
│ ├─ Modelling: Compute bundles (queue, stability, signal)   │
│ ├─ Topology: Build dependency graph, detect criticality    │
│ ├─ Prediction: Monte-Carlo cascade forecast (60s)          │
│ ├─ Intelligence: Evaluate RL policy + MPC control          │
│ ├─ Control: Send commands to actuators                     │
│ └─ Streaming: Broadcast TickPayload to WebSocket           │
└────────────┬─────────────────────────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────────────────────────┐
│ MODELLING SUBSYSTEMS (14 modules)                          │
│ ├─ Queue Theory: λ, μ, ρ, L_q, W, latencies              │
│ ├─ Stability Analysis: collapse_risk, oscillation, margin  │
│ ├─ Signal Processing: EWMA, CUSUM, spike detection        │
│ ├─ Stochastic: burst_amplification, risk_propagation      │
│ ├─ Network Equilibrium: Gauss-Seidel solver               │
│ ├─ Hazard Physics: exponential failure distributions       │
│ ├─ Reservoir Physics: buffer utilization modeling          │
│ └─ ... (more in internal/modelling/)                      │
└─────────────────────────────────────────────────────────────┘
```

### Data Flow per Tick (10Hz = 100ms)

```
T=0ms:    External telemetry arrives (service metrics)
          ↓
T=5ms:    Modelling: Compute M/M/c bundles
          ↓
T=15ms:   Topology: Build graph, critical path
          ↓
T=25ms:   Prediction: Monte-Carlo cascade forecast
          ↓
T=35ms:   Intelligence: Evaluate control policy
          ↓
T=45ms:   Control: Send commands to actuators
          ↓
T=50ms:   Streaming: Broadcast TickPayload via WebSocket
          ↓
T=100ms:  Next tick begins
```

---

## Data Model

### TickPayload (WebSocket message every 100ms)

```typescript
type TickPayload = {
  seq: number;                    // Message sequence number
  timestamp_ms: number;           // When tick was computed
  
  // System-level metrics
  objective: {
    composite_score: number;           // [0, 100] overall health
    cascade_failure_probability: number; // [0, 1]
    predicted_p99_latency_ms: number;   // Forecasted worst-case
    predicted_sla_violations: number;   // Count next 60s
    stability_envelope: number;         // [0, 1] prediction confidence
  };
  
  // Per-service metrics (dictionary)
  bundles: {
    [service_id]: {
      queue: {
        arrival_rate: number;        // λ (requests/sec)
        service_rate: number;        // μ (1/service_time)
        concurrency: number;         // c (parallel workers)
        utilisation: number;         // ρ = λ/(μ*c)
        queue_depth: number;         // L_q
        wait_time_ms: number;        // W (avg latency)
        p50_latency_ms: number;
        p95_latency_ms: number;
        p99_latency_ms: number;
      };
      stability: {
        collapse_risk: number;           // [0, 1]
        oscillation_risk: number;        // [0, 1]
        margin_to_saturation: number;    // (1 - ρ)
        last_spike: {
          magnitude: number;
          duration_ms: number;
          timestamp_ms: number;
        };
      };
      stochastic: {
        burst_amplification: number;    // How much bursts amplify
        risk_propagation: number;       // Upstream risk effect
        sla_violations_predicted: number;
      };
      signal: {
        ewma: number;                   // Exponential moving average
        cusum: number;                  // Cumulative sum anomaly
        spike_detected: boolean;
        trend: 'rising' | 'stable' | 'falling';
      };
    }
  };
  
  // Topology graph
  topology: {
    nodes: { id: string; risk_level: string }[];
    edges: { from: string; to: string; error_rate: number; latency_ms: number }[];
    critical_path: string[];           // Service IDs on critical path
    fragility_index: number;           // [0, 1] how fragile topology is
  };
  
  // Predictions
  predictions: {
    cascade_trajectory: [
      { time_ahead_ms: number; probability: number; affected_services: number }
    ];
    sla_risk_next_60s: number;
  };
  
  // Control state
  control: {
    policy_active: boolean;
    chaos_mode: boolean;
    last_action: string;
  };
};
```

### TickPayload Size
- **Typical**: 50-150 KB per message
- **Frequency**: 10 Hz (every 100ms)
- **Bandwidth**: ~500 KB/s per client
- **Scaling**: WebSocket hub broadcasts to all connected dashboards simultaneously

---

## Control Algorithms

### 1. Model Predictive Control (MPC)

```
For each upcoming time step (0 to 60s):
  1. Simulate forward with current policy
  2. Predict cascade probability at each step
  3. Evaluate control actions:
     - Scale services (increase c)
     - Shift traffic (reduce λ)
     - Increase service rate (upgrade μ)
  4. Choose action that minimizes cascade probability
  5. Constrain actions to maintain SLA
  6. Apply best action immediately
```

### 2. Reinforcement Learning Policy

- **State**: Service bundles + topology
- **Action Space**: Scale/traffic-shift/upgrade decisions
- **Reward**: Negative cascade_probability + positive efficiency
- **Constraints**: Can't violate SLA, resource limits, cost budgets

### 3. Safety Projection

```
If proposed_action violates_sla:
  → Project action to nearest safe boundary
  → Use backup control (emergency throttling)
  
If cascade_probability > 0.85:
  → Override RL policy
  → Execute emergency sequence:
    1. Drop lowest-priority requests
    2. Shift traffic to backup regions
    3. Trigger auto-scaling
    4. Alert operations team
```

---

## API Reference

### REST Endpoints

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/health` | GET | Health check (200 OK if running) |
| `/snapshot` | GET | Current system state snapshot |
| `/api/v1/ingest` | POST | Submit telemetry data |
| `/api/v1/policy/toggle` | POST | Enable/disable RL policy |
| `/api/v1/policy/update` | POST | Update policy weights |
| `/api/v1/control/chaos` | POST | Trigger chaos test |
| `/api/v1/control/replay-burst` | POST | Replay stored burst |
| `/api/v1/control/toggle-policy` | POST | Switch active policy |
| `/api/v1/runtime/step` | POST | Advance simulation one tick |
| `/api/v1/simulation/control` | POST | Set simulation parameters |
| `/api/v1/intelligence/rollout` | POST | Trigger policy rollout |
| `/api/v1/alerts/ack` | POST | Acknowledge alert |
| `/api/v1/domain/{domain}/trigger` | POST | Trigger domain-specific action |

### WebSocket Endpoint

```
ws://localhost:8080/ws

Connection: 
  1. Browser opens WebSocket to /ws
  2. Server sends TickPayload every 100ms
  3. Browser stores in Zustand store (60-point history)
  4. Components re-render reactively
  5. On disconnect: exponential backoff reconnect (1s→30s)
```

---

## Deployment Guide

### Local Development

```bash
# Terminal 1: Backend
cd completeproject
go run cmd/loadequilibrium/main.go

# Terminal 2: Dashboard
cd completeproject/dashboard
npm install
npm run dev
```

Open: http://localhost:3000

### Docker Deployment

```bash
# Build image
docker build -t loadequilibrium:latest .

# Run container
docker run -e PORT=8080 -e SCENARIO_MODE=on -p 8080:8080 loadequilibrium:latest
```

### Docker Compose (Full Stack)

```bash
docker-compose up
```

Includes:
- LoadEquilibrium backend (port 8080)
- Dashboard (port 3000)
- Prometheus metrics scraper
- Grafana dashboards (optional)

### Environment Variables

```bash
# Backend
PORT=8080                              # API server port
SCENARIO_MODE=off                      # on/off scenario engine
DATABASE_URL=postgres://...            # Optional persistence
MAX_SERVICES=200                       # Max service capacity
RING_BUFFER_DEPTH=1000                 # History buffer size
STALE_SERVICE_AGE=30s                  # Service TTL

# Dashboard
NEXT_PUBLIC_BACKEND_WS=ws://localhost:8080/ws
NEXT_PUBLIC_BACKEND_API=http://localhost:8080/api/v1
```

---

## Performance Characteristics

| Metric | Value |
|--------|-------|
| Tick Frequency | 10 Hz (100ms) |
| TickPayload Size | 50-150 KB |
| WebSocket Broadcast | <50ms |
| REST Response Time | <200ms |
| Monte-Carlo Simulation | <30ms (60-step horizon) |
| Network Equilibrium Solve | <15ms (Gauss-Seidel) |
| Memory per Service | ~5 KB (bundle + history) |
| Throughput @ 200 services | 50-100 Mbps bandwidth |

---

## Monitoring & Observability

### Prometheus Metrics
- `loadequilibrium_cascade_probability` - System-wide cascade risk
- `loadequilibrium_service_utilisation` - Per-service ρ
- `loadequilibrium_sla_violations` - Predicted violations
- `loadequilibrium_control_actions` - Actions executed

### Dashboard Alerts
- Cascade probability > 0.70 → CRITICAL
- Service utilisation > 0.85 → WARNING
- SLA violations predicted > 5 → WARNING
- Topology fragility > 0.8 → INFO

### Logs
- `[engine]` - 10Hz tick events
- `[api]` - REST/WebSocket activity
- `[control]` - Policy decisions
- `[topology]` - Graph analysis

---

## Technical Specifications

### Backend (Go)
- **Language**: Go 1.21+
- **Framework**: net/http (no external HTTP dependencies)
- **Concurrency**: goroutines per WebSocket client
- **Memory**: O(n_services) where n ≤ 200
- **Dependencies**: Standard library only (minimal)

### Frontend (Next.js)
- **Framework**: Next.js 16.2.2
- **UI Library**: React 19.2.4
- **State Management**: Zustand 5.0.12
- **Charts**: Recharts 3.8.1
- **Styling**: Tailwind CSS v4.2.2
- **Build**: Turbopack (fast incremental builds)

---

## Troubleshooting

| Issue | Cause | Solution |
|-------|-------|----------|
| Dashboard shows DISCONNECTED | WebSocket not connecting | Check `ws://localhost:8080/ws` is reachable |
| No service data in dashboard | Backend not running | Run `go run cmd/loadequilibrium/main.go` |
| Cascade probability always 0 | Scenario mode off | Set `SCENARIO_MODE=on` to generate test data |
| High P99 latency | Utilisation > 0.85 | Increase concurrency or reduce load |
| Memory usage high | Too many services tracked | Reduce `MAX_SERVICES` or increase `STALE_SERVICE_AGE` |

---

## License

Commercial. See LICENSE file.
