# LoadEquilibrium

**A predictive infrastructure telemetry runtime for computing queue saturation horizons and preemptive load-shedding control trajectories.**

LoadEquilibrium is an early research-grade discrete-event simulation engine built to model dependency failure in microservice topologies. It ingests out-of-band traffic telemetry, mathematically reconstructs the underlying queue depths, and computes the probability of cascade failure across the dependency graph.

It is designed to feed forward-looking saturation metrics to edge actuators (like Envoy or Kubernetes HPA) *before* downstream latencies collapse, solving the lag inherent in standard reactive CPU/error-rate autoscaling.

## Architecture

LoadEquilibrium consists of four primary subsystems running in a continuous real-time loop:

1. **Topology & Ingestion Engine:** Accepts structured JSON telemetry (arrival rates, latency percentiles, active connections, edges). Inverts these raw metrics into fluid approximations of system load.
2. **Queueing Mathematics Solver:** Uses M/M/c Erlang-C formulas paired with M/G/1 Pollaczek-Khinchine variance corrections to map observed latency distributions to theoretical waiting times and internal queue depths.
3. **Budget-Bounded Discrete-Event Simulator (DES):** Executes continuous Monte Carlo "shock" simulations across the dependency graph to discover head-of-line blocking and dependency saturation horizons. Bound by strict computational wall-clock limits to guarantee sub-second tick determinism.
4. **Model Predictive Controller (MPC):** Calculates a risk-weighted trajectory cost—balancing the latency cost of queueing against the risk of systemic collapse—to output a theoretical scaling or shedding factor for an external actuator.

## Modelling Philosophy

- **Queues, over Counters:** CPU and memory are trailing indicators. LoadEquilibrium models services as concurrent queues (c-servers).
- **Non-Linear Degradation:** A system at 80% utilisation behaves differently than one at 95%. Latency inflates asymptotically. The engine captures this physics natively.
- **Delayed Metric Inference:** Telemetry is often stale. If signal confidence drops, the model extrapolates gracefully using recent acceleration derivatives, penalized by an exponential decay function.
- **Probabilistic Shock Propagation:** Load shocks don't propagate equally. Downstream cascade risk is treated as a branching probability heavily influenced by edge weights and current downstream queue utilisation.

## Example Workflow: Preemptive Edge Shedding

Consider an API Gateway (`edge-proxy`) calling a `billing-service` which relies on a shared PostgreSQL pool.

1. **Telemetry Ingest:** The services emit high-frequency telemetry (every 2s).
2. **Detection:** The engine detects a traffic spike at `edge-proxy`. It calculates that the `billing-service` arrival rate variance (CoV) is increasing.
3. **Simulation:** The DES runs 400,000 events locally, forecasting that `billing-service` will exhaust its connection pool (Queue > 100) in exactly 12.4 seconds due to incoming acceleration.
4. **Actuation Signal:** The MPC module outputs an aggressive `ScaleFactor < 1.0` (shed) for requests bound to `billing-service`, propagating to an Envoy WASM plugin.
5. **Result:** Envoy sheds 15% of traffic hitting the billing route. The PostgreSQL pool avoids lock-contention collapse, preserving the critical path.

## Limitations

- **Actuation Void:** The current runtime computes a mathematically sound control trajectory but lacks native integrations to enforce it (e.g., standard Envoy xDS endpoints or Kubernetes custom metrics APIs).
- **Computational Overhead:** The continuous DES places a heavy CPU burden on the control plane, making it difficult to scale to topologies with thousands of dense edges without horizontal sharding.
- **Model Rigidity:** The engine assumes exponential service times (M/M/x) unless heavy-tailed P99 data is provided, which can misrepresent real-world multimodal workloads (like GC pauses or batch database queries).

## Future Roadmap

1. **eBPF Telemetry Sink:** Remove the need for bespoke JSON payloads by digesting standard eBPF network socket metrics.
2. **Actuator Integrations:** Native export of scaling/shedding signals to KEDA, Kubernetes HPA, and Envoy Rate Limit APIs.
3. **Empirical Validation Suite:** A rigorous chaos-testing framework to quantify the Saturation Prediction Error against a real high-throughput proxy testbed.
