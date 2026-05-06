import os
import time
from typing import Dict, List

import requests


PROMETHEUS_URL = os.getenv("PROMETHEUS_URL", "http://localhost:9090")
INGEST_URL = os.getenv("INGEST_URL", "http://localhost:8080/api/v1/ingest")
INGEST_TOKEN = os.getenv("INGEST_TOKEN", "changeme-set-in-production")
POLL_INTERVAL = float(os.getenv("COLLECTOR_INTERVAL_SEC", "2"))
TIMEOUT_SEC = float(os.getenv("COLLECTOR_TIMEOUT_SEC", "5"))

QUERIES = {
    "request_rate": "request_rate",
    "error_rate": "error_rate",
    "latency_p50_ms": "latency_p50_ms",
    "latency_p95_ms": "latency_p95_ms",
    "latency_p99_ms": "latency_p99_ms",
    "latency_mean_ms": "latency_mean_ms",
    "active_conns": "active_conns",
    "queue_depth": "queue_depth",
    "cpu_usage": "cpu_usage",
    "mem_usage": "mem_usage",
}


def query_instant(expr: str) -> Dict[str, float]:
    response = requests.get(
        f"{PROMETHEUS_URL}/api/v1/query",
        params={"query": expr},
        timeout=TIMEOUT_SEC,
    )
    response.raise_for_status()
    payload = response.json()
    if payload.get("status") != "success":
        raise RuntimeError(f"prometheus query failed: {expr}")

    out: Dict[str, float] = {}
    for item in payload["data"]["result"]:
        service_id = item.get("metric", {}).get("service_id")
        if not service_id:
            continue
        out[service_id] = float(item["value"][1])
    return out


def collect_services() -> Dict[str, Dict[str, float]]:
    services: Dict[str, Dict[str, float]] = {}
    for field, expr in QUERIES.items():
        for service_id, value in query_instant(expr).items():
            services.setdefault(service_id, {})[field] = value
    return services


def build_payload(services: Dict[str, Dict[str, float]]) -> List[Dict[str, object]]:
    points: List[Dict[str, object]] = []
    payment = services.get("payment", {})

    for service_id, metrics in services.items():
        point = {
            "service_id": service_id,
            "request_rate": metrics.get("request_rate", 0.0),
            "error_rate": metrics.get("error_rate", 0.0),
            "latency": {
                "p50": metrics.get("latency_p50_ms", 0.0),
                "p95": metrics.get("latency_p95_ms", 0.0),
                "p99": metrics.get("latency_p99_ms", 0.0),
                "mean": metrics.get("latency_mean_ms", 0.0),
            },
            "cpu_usage": metrics.get("cpu_usage", 0.0),
            "mem_usage": metrics.get("mem_usage", 0.0),
            "active_conns": int(round(metrics.get("active_conns", 0.0))),
            "queue_depth": int(round(metrics.get("queue_depth", 0.0))),
        }

        if service_id == "checkout" and payment:
            point["upstream_calls"] = [
                {
                    "target_service_id": "payment",
                    "call_rate": max(metrics.get("request_rate", 0.0) * 0.7, 1.0),
                    "error_rate": payment.get("error_rate", 0.0),
                    "latency_mean": payment.get("latency_mean_ms", 0.0),
                }
            ]

        points.append(point)

    return points


def post_payload(points: List[Dict[str, object]]) -> None:
    headers = {"Content-Type": "application/json"}
    if INGEST_TOKEN:
        headers["X-Ingest-Token"] = INGEST_TOKEN

    response = requests.post(
        INGEST_URL,
        json=points,
        headers=headers,
        timeout=TIMEOUT_SEC,
    )
    response.raise_for_status()
    print(f"[collector] posted points={len(points)} status={response.status_code}", flush=True)


def main() -> None:
    print(f"[collector] prometheus={PROMETHEUS_URL} ingest={INGEST_URL}", flush=True)
    while True:
        try:
            services = collect_services()
            if not services:
                print("[collector] waiting for Prometheus samples", flush=True)
            else:
                post_payload(build_payload(services))
        except Exception as exc:
            print(f"[collector] error={exc}", flush=True)
        time.sleep(POLL_INTERVAL)


if __name__ == "__main__":
    main()
