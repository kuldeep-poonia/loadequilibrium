package collector

import (
	"testing"
	"time"
)

func TestNormalizerBuildsExistingIngestSchema(t *testing.T) {
	n := NewNormalizer()
	target := ServiceTarget{
		ServiceID:        "checkout",
		MemoryLimitBytes: 1024,
	}

	first := []Sample{
		{Name: "http_requests_total", Labels: map[string]string{"status": "200"}, Value: 100},
		{Name: "http_requests_total", Labels: map[string]string{"status": "500"}, Value: 2},
		{Name: "http_request_duration_seconds_sum", Value: 10},
		{Name: "http_request_duration_seconds_count", Value: 102},
		{Name: "http_client_requests_total", Labels: map[string]string{"peer_service": "payment", "status": "200"}, Value: 50},
		{Name: "queue_depth", Value: 3},
		{Name: "active_connections", Value: 7},
		{Name: "process_resident_memory_bytes", Value: 512},
	}
	second := []Sample{
		{Name: "http_requests_total", Labels: map[string]string{"status": "200"}, Value: 200},
		{Name: "http_requests_total", Labels: map[string]string{"status": "500"}, Value: 7},
		{Name: "http_request_duration_seconds_sum", Value: 25},
		{Name: "http_request_duration_seconds_count", Value: 207},
		{Name: "http_client_requests_total", Labels: map[string]string{"peer_service": "payment", "status": "200"}, Value: 80},
		{Name: "queue_depth", Value: 9},
		{Name: "active_connections", Value: 11},
		{Name: "process_resident_memory_bytes", Value: 512},
	}

	now := time.Unix(1000, 0)
	if _, ok := n.Build(target, first, now); !ok {
		t.Fatal("first scrape should still produce gauge-derived telemetry")
	}

	point, ok := n.Build(target, second, now.Add(10*time.Second))
	if !ok {
		t.Fatal("second scrape did not produce telemetry")
	}
	if point.ServiceID != "checkout" {
		t.Fatalf("unexpected service id %q", point.ServiceID)
	}
	if point.RequestRate < 10 || point.RequestRate > 11 {
		t.Fatalf("unexpected request rate %.3f", point.RequestRate)
	}
	if point.ErrorRate <= 0 {
		t.Fatalf("expected error rate, got %.3f", point.ErrorRate)
	}
	if point.Latency.Mean <= 0 {
		t.Fatalf("expected latency mean, got %.3f", point.Latency.Mean)
	}
	if point.QueueDepth != 9 {
		t.Fatalf("unexpected queue depth %d", point.QueueDepth)
	}
	if point.ActiveConns != 11 {
		t.Fatalf("unexpected active conns %d", point.ActiveConns)
	}
	if len(point.UpstreamCalls) != 1 || point.UpstreamCalls[0].TargetServiceID != "payment" {
		t.Fatalf("expected payment upstream, got %#v", point.UpstreamCalls)
	}
}
