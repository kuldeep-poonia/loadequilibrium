# LE Collector Production Integration Plan

## 1. Architecture Plan

LE Collector is an additive telemetry bridge. It does not replace the existing LoadEquilibrium ingest API, runtime orchestrator, topology engine, modelling pipeline, actuator, persistence, WebSocket stream, or dashboard.

The control plane still receives `telemetry.MetricPoint` data through `POST /api/v1/ingest`. The collector discovers labelled Docker containers, scrapes their Prometheus-compatible `/metrics` endpoints, normalizes those samples into the existing ingest schema, and batches them into the current API.

Runtime ownership remains:

- LoadEquilibrium: modelling, topology, optimization, intelligence, actuation, streaming.
- LE Collector: service discovery, Prometheus scraping, metric normalization, topology signal extraction, ingest delivery.

## 2. Collector Subsystem Design

New code lives under:

- `cmd/le-collector`: collector binary entrypoint.
- `internal/collector`: Docker discovery, Prometheus parser, normalizer, ingest client, health/metrics server.
- `Dockerfile.collector`: production container build.

The collector is intentionally out-of-process. That keeps collector failures from destabilizing the control loop and lets future deployments scale discovery/scraping separately.

## 3. Docker Discovery Implementation Plan

The collector uses the Docker Engine HTTP API over `unix:///var/run/docker.sock`.

Required service label:

```yaml
labels:
  le.enable: "true"
```

Optional labels:

```yaml
labels:
  le.service: "checkout-api"
  le.metrics_path: "/metrics"
  le.environment: "prod"
  le.team: "payments"
```

Additional production labels supported by the implementation:

```yaml
labels:
  le.metrics_port: "8080"
  le.metrics_scheme: "http"
  le.metrics_host: "checkout-api"
  le.metrics_url: "http://checkout-api:8080/metrics"
```

Discovery inspects running containers, checks `le.enable`, resolves service identity, detects exposed ports, builds candidate metrics URLs, and refreshes on `LE_DISCOVERY_INTERVAL`.

## 4. Prometheus Scraping Architecture

The scraper performs real HTTP GET requests against discovered endpoints and parses Prometheus text exposition format.

It supports:

- counters
- gauges
- histograms
- labelled samples
- bounded response bodies
- connection pooling
- scrape timeouts
- concurrent scraping with a configurable limit

## 5. Telemetry Normalization Layer

The normalizer converts Prometheus samples into the existing `telemetry.MetricPoint` shape:

- request rate from request counters or request-rate gauges
- latency from histogram `_bucket`, `_sum`, `_count`, or latency gauges
- error rate from status/code/outcome/error labels
- queue pressure from queue depth/size/lag gauges
- active connections from active/inflight gauges
- CPU from cumulative CPU counters
- memory fraction from process memory divided by Docker memory limit when available

No telemetry schema replacement is required.

## 6. Topology Inference Pipeline

Topology is inferred from real outbound/client/dependency metrics. The collector recognizes client-side counters and histograms with labels such as:

- `peer_service`
- `peer.service`
- `target_service`
- `destination_service`
- `net_peer_name`
- `net.peer.name`
- `dependency`
- `db_system`
- `db.system`
- `le_target`

Those are emitted as `upstream_calls[]` in the existing ingest payload, which feeds the current topology graph unchanged.

## 7. Runtime Integration Strategy

The only runtime integration point is the existing ingest endpoint:

```text
LE Collector -> POST /api/v1/ingest -> telemetry.Store -> runtime.Orchestrator
```

Manual ingest remains supported. Users can mix collector-fed telemetry and direct application POSTs.

## 8. Production Hardening Checklist

Implemented:

- Docker socket discovery
- bounded scrape body
- HTTP timeouts
- connection pooling
- concurrent scrape limit
- bounded ingest queue
- batching
- exponential backoff
- retry budget
- circuit breaker
- rate-limited ingest requests
- structured logs
- `/health`
- `/ready`
- `/targets`
- `/metrics`
- graceful shutdown

Still recommended before broad production rollout:

- TLS/mTLS for remote Docker or remote metrics endpoints
- auth for collector `/targets` if exposed outside a private network
- Kubernetes discovery in a separate collector mode
- native OTLP receiver or OpenTelemetry Collector exporter
- service-mesh metric profiles for Istio/Linkerd/Envoy

## 9. Docker Compose Onboarding Flow

Start the platform:

```bash
docker compose up -d
```

Add labels to a service in the same Compose project or a network reachable by the collector:

```yaml
services:
  checkout-api:
    image: example/checkout-api
    labels:
      le.enable: "true"
      le.service: "checkout-api"
      le.metrics_path: "/metrics"
```

The collector discovers the container, scrapes metrics, posts normalized telemetry to LoadEquilibrium, and the dashboard updates through the existing WebSocket stream.

## 10. New Directories And Files

- `cmd/le-collector/main.go`
- `internal/collector/config.go`
- `internal/collector/types.go`
- `internal/collector/docker.go`
- `internal/collector/prometheus.go`
- `internal/collector/normalizer.go`
- `internal/collector/ingest.go`
- `internal/collector/collector.go`
- `internal/collector/http.go`
- `internal/collector/*_test.go`
- `Dockerfile.collector`
- `docs/LE_COLLECTOR.md`

## 11. Minimal-Risk Migration Plan

1. Deploy collector with no labelled services.
2. Confirm `/health`, `/metrics`, and `/targets`.
3. Label one low-risk service with `le.enable=true`.
4. Verify collector `/targets` includes that service.
5. Verify LoadEquilibrium dashboard receives a service bundle.
6. Add labels to more services gradually.
7. Enable actuator HTTP backend only after recommendations are trusted.

## 12. End-To-End Telemetry Flow

1. Service exposes Prometheus `/metrics`.
2. Docker label `le.enable=true` marks it for discovery.
3. Collector inspects Docker, resolves service ID and metrics URL.
4. Collector scrapes Prometheus samples.
5. Normalizer computes request rate, latency, errors, queue pressure, resource usage, and dependencies.
6. Ingest client batches points and POSTs to `/api/v1/ingest`.
7. Existing telemetry store builds service windows.
8. Existing topology/runtime/modelling/optimization pipeline runs on the next tick.
9. Existing WebSocket stream sends dashboard state.
10. Existing actuator path can emit recommendations or real scaling calls.

## 13. Actual Code Changes

The implementation is real and functional. It does not create a mock adapter or simulated data source. Data only flows when labelled Docker containers expose real Prometheus metrics.

## 14. Testing Strategy

Implemented tests cover:

- Prometheus text parsing
- Counter-rate normalization
- latency extraction
- error-rate extraction
- queue and active connection gauges
- upstream call inference
- conversion into the existing ingest schema

Recommended integration test:

1. Start `docker compose up -d`.
2. Start a labelled fixture service exposing `/metrics`.
3. Check `curl localhost:9091/targets`.
4. Check `curl localhost:8080/api/v1/snapshot`.
5. Confirm the dashboard shows the service and topology edge.

## 15. Failure-Mode Analysis

- Docker socket unavailable: collector logs discovery failure, health stays reachable, no telemetry is emitted.
- Service has label but no reachable metrics endpoint: target remains discovered, scrapes fail, ingest is not polluted with fake data.
- Metrics endpoint slow: scrape timeout protects collector worker pool.
- Metrics endpoint too large: body limit prevents memory blowup.
- LoadEquilibrium ingest unavailable: retry/backoff runs, circuit opens after repeated failures, queue is bounded.
- Queue full: points are dropped and counted in collector metrics.
- Counter reset: negative deltas are ignored for that scrape.
- Missing topology metrics: service telemetry still flows; dependency edges appear only when real client/dependency metrics exist.
