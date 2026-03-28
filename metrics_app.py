import math
import time

from prometheus_client import Counter, Gauge, start_http_server


REQUESTS_TOTAL = Counter("requests_total", "Total synthetic requests", ["service_id"])
REQUEST_RATE = Gauge("request_rate", "Synthetic request rate", ["service_id"])
ERROR_RATE = Gauge("error_rate", "Synthetic error rate", ["service_id"])
LATENCY_P50_MS = Gauge("latency_p50_ms", "Synthetic p50 latency", ["service_id"])
LATENCY_P95_MS = Gauge("latency_p95_ms", "Synthetic p95 latency", ["service_id"])
LATENCY_P99_MS = Gauge("latency_p99_ms", "Synthetic p99 latency", ["service_id"])
LATENCY_MEAN_MS = Gauge("latency_mean_ms", "Synthetic mean latency", ["service_id"])
ACTIVE_CONNS = Gauge("active_conns", "Synthetic active connections", ["service_id"])
QUEUE_DEPTH = Gauge("queue_depth", "Synthetic queue depth", ["service_id"])
CPU_USAGE = Gauge("cpu_usage", "Synthetic cpu usage", ["service_id"])
MEM_USAGE = Gauge("mem_usage", "Synthetic memory usage", ["service_id"])


SERVICES = {
    "checkout": {
        "base_rate": 280.0,
        "rate_amp": 90.0,
        "queue_base": 90.0,
        "queue_amp": 70.0,
        "lat_base": 320.0,
        "lat_amp": 180.0,
        "err_base": 0.03,
        "conn_base": 45.0,
        "conn_amp": 25.0,
        "cpu_base": 0.72,
        "mem_base": 0.66,
    },
    "payment": {
        "base_rate": 170.0,
        "rate_amp": 60.0,
        "queue_base": 60.0,
        "queue_amp": 40.0,
        "lat_base": 220.0,
        "lat_amp": 120.0,
        "err_base": 0.015,
        "conn_base": 30.0,
        "conn_amp": 18.0,
        "cpu_base": 0.61,
        "mem_base": 0.58,
    },
}


def set_service_metrics(tick: int) -> None:
    for service_id, cfg in SERVICES.items():
        rate_wave = 0.5 + 0.5 * math.sin((tick / 3.0) + (0.4 if service_id == "payment" else 0.0))
        queue_wave = 0.5 + 0.5 * math.cos((tick / 4.0) + (0.8 if service_id == "payment" else 0.0))
        latency_wave = 0.5 + 0.5 * math.sin((tick / 5.0) + (1.0 if service_id == "payment" else 0.0))

        request_rate = cfg["base_rate"] + cfg["rate_amp"] * rate_wave
        queue_depth = cfg["queue_base"] + cfg["queue_amp"] * queue_wave
        latency_mean = cfg["lat_base"] + cfg["lat_amp"] * latency_wave
        latency_p50 = max(10.0, latency_mean * 0.45)
        latency_p95 = latency_mean * 1.45
        latency_p99 = latency_mean * 1.85
        error_rate = min(0.25, cfg["err_base"] + 0.05 * latency_wave)
        active_conns = cfg["conn_base"] + cfg["conn_amp"] * rate_wave
        cpu_usage = min(0.98, cfg["cpu_base"] + 0.18 * rate_wave)
        mem_usage = min(0.98, cfg["mem_base"] + 0.12 * queue_wave)

        REQUESTS_TOTAL.labels(service_id=service_id).inc(request_rate)
        REQUEST_RATE.labels(service_id=service_id).set(request_rate)
        ERROR_RATE.labels(service_id=service_id).set(error_rate)
        LATENCY_P50_MS.labels(service_id=service_id).set(latency_p50)
        LATENCY_P95_MS.labels(service_id=service_id).set(latency_p95)
        LATENCY_P99_MS.labels(service_id=service_id).set(latency_p99)
        LATENCY_MEAN_MS.labels(service_id=service_id).set(latency_mean)
        ACTIVE_CONNS.labels(service_id=service_id).set(active_conns)
        QUEUE_DEPTH.labels(service_id=service_id).set(queue_depth)
        CPU_USAGE.labels(service_id=service_id).set(cpu_usage)
        MEM_USAGE.labels(service_id=service_id).set(mem_usage)


def main() -> None:
    start_http_server(8000)
    print("[metrics_app] serving synthetic metrics on :8000", flush=True)

    tick = 0
    while True:
        tick += 1
        set_service_metrics(tick)
        print(f"[metrics_app] updated tick={tick}", flush=True)
        time.sleep(1)


if __name__ == "__main__":
    main()
