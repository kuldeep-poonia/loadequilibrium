package collector

import "testing"

func TestParsePrometheusText(t *testing.T) {
	body := []byte(`
# HELP http_requests_total Total requests.
# TYPE http_requests_total counter
http_requests_total{method="GET",status="200",route="/checkout"} 12
http_request_duration_seconds_bucket{le="0.1"} 10
http_request_duration_seconds_bucket{le="+Inf"} 12
http_request_duration_seconds_sum 0.42
http_request_duration_seconds_count 12
`)

	samples, err := ParsePrometheusText(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 5 {
		t.Fatalf("expected 5 samples, got %d", len(samples))
	}
	if samples[0].Name != "http_requests_total" {
		t.Fatalf("unexpected first metric %q", samples[0].Name)
	}
	if samples[0].Labels["status"] != "200" {
		t.Fatalf("status label not parsed: %#v", samples[0].Labels)
	}
	if samples[1].Labels["le"] != "0.1" {
		t.Fatalf("bucket label not parsed: %#v", samples[1].Labels)
	}
}
