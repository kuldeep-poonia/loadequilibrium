package collector

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/telemetry"
)

type Normalizer struct {
	mu   sync.Mutex
	prev map[string]counterPoint
}

type counterPoint struct {
	value float64
	seen  time.Time
}

type upstreamAccum struct {
	callRate    float64
	errorRate   float64
	errorCalls  float64
	latencySum  float64
	latencyRate float64
}

type histogramAccum struct {
	buckets map[float64]float64
	sumRate float64
	cntRate float64
}

func NewNormalizer() *Normalizer {
	return &Normalizer{prev: make(map[string]counterPoint)}
}

func (n *Normalizer) Build(target ServiceTarget, samples []Sample, now time.Time) (telemetry.MetricPoint, bool) {
	if now.IsZero() {
		now = time.Now()
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	var explicitRateGauge, serverCounterRate, serverHistCountRate float64
	var serverErrorCalls, cpuRate, memBytes float64
	var activeConns, queueDepth float64
	serverLatency := make(map[string]*histogramAccum)
	upstreams := make(map[string]*upstreamAccum)
	clientLatency := make(map[string]*histogramAccum)

	for _, s := range samples {
		name := strings.ToLower(s.Name)
		if isRateGauge(name) {
			explicitRateGauge += positive(s.Value)
		}
		if isQueueGauge(name) {
			queueDepth = math.Max(queueDepth, positive(s.Value))
		}
		if isActiveGauge(name) {
			activeConns = math.Max(activeConns, positive(s.Value))
		}
		if isMemoryGauge(name) {
			memBytes = math.Max(memBytes, positive(s.Value))
		}
		if isCPUCumulative(name) {
			if rate, ok := n.counterRate(target.ServiceID, s, now); ok {
				cpuRate += rate
			}
		}

		if isServerRequestCounter(name) {
			if rate, ok := n.counterRate(target.ServiceID, s, now); ok {
				serverCounterRate += rate
				if isErrorSample(s.Labels) {
					serverErrorCalls += rate
				}
			}
		}

		if isServerLatencyHistogram(name) {
			base, suffix := histogramBase(name)
			if base == "" {
				continue
			}
			acc := serverLatency[base]
			if acc == nil {
				acc = &histogramAccum{buckets: make(map[float64]float64)}
				serverLatency[base] = acc
			}
			switch suffix {
			case "bucket":
				le, ok := parseBucket(s.Labels["le"])
				if !ok {
					continue
				}
				if rate, ok := n.counterRate(target.ServiceID, s, now); ok {
					acc.buckets[le] += rate
				}
			case "sum":
				if rate, ok := n.counterRate(target.ServiceID, s, now); ok {
					acc.sumRate += rate
				}
			case "count":
				if rate, ok := n.counterRate(target.ServiceID, s, now); ok {
					acc.cntRate += rate
					serverHistCountRate += rate
					if isErrorSample(s.Labels) {
						serverErrorCalls += rate
					}
				}
			}
		}

		if isClientRequestCounter(name) {
			targetID := dependencyTarget(s.Labels)
			if targetID == "" || targetID == target.ServiceID {
				continue
			}
			if rate, ok := n.counterRate(target.ServiceID, s, now); ok {
				acc := upstreams[targetID]
				if acc == nil {
					acc = &upstreamAccum{}
					upstreams[targetID] = acc
				}
				acc.callRate += rate
				if isErrorSample(s.Labels) {
					acc.errorCalls += rate
				}
			}
		}

		if isClientLatencyHistogram(name) {
			targetID := dependencyTarget(s.Labels)
			if targetID == "" || targetID == target.ServiceID {
				continue
			}
			base, suffix := histogramBase(name)
			if base == "" {
				continue
			}
			key := targetID + "\xff" + base
			acc := clientLatency[key]
			if acc == nil {
				acc = &histogramAccum{buckets: make(map[float64]float64)}
				clientLatency[key] = acc
			}
			switch suffix {
			case "bucket":
				le, ok := parseBucket(s.Labels["le"])
				if !ok {
					continue
				}
				if rate, ok := n.counterRate(target.ServiceID, s, now); ok {
					acc.buckets[le] += rate
				}
			case "sum":
				if rate, ok := n.counterRate(target.ServiceID, s, now); ok {
					acc.sumRate += rate
				}
			case "count":
				if rate, ok := n.counterRate(target.ServiceID, s, now); ok {
					acc.cntRate += rate
				}
			}
		}
	}

	requestRate := explicitRateGauge
	if requestRate <= 0 {
		requestRate = serverCounterRate
	}
	if requestRate <= 0 {
		requestRate = serverHistCountRate
	}

	latency := latencyFromHistograms(serverLatency)
	if latency.Mean <= 0 {
		latency.Mean = firstGauge(samples, isLatencyMeanGauge)
	}
	if latency.P50 <= 0 {
		latency.P50 = firstGauge(samples, isLatencyP50Gauge)
	}
	if latency.P95 <= 0 {
		latency.P95 = firstGauge(samples, isLatencyP95Gauge)
	}
	if latency.P99 <= 0 {
		latency.P99 = firstGauge(samples, isLatencyP99Gauge)
	}

	if latency.Mean <= 0 && requestRate > 0 {
		latency.Mean = 0.1
	}
	if latency.P50 <= 0 && latency.Mean > 0 {
		latency.P50 = latency.Mean
	}
	if latency.P95 <= 0 && latency.Mean > 0 {
		latency.P95 = latency.Mean * 1.5
	}
	if latency.P99 <= 0 && latency.P95 > 0 {
		latency.P99 = latency.P95 * 1.2
	}

	totalForErrors := requestRate
	if totalForErrors <= 0 {
		totalForErrors = serverCounterRate + serverHistCountRate
	}
	errorRate := 0.0
	if totalForErrors > 0 {
		errorRate = clamp(serverErrorCalls/totalForErrors, 0, 1)
	}

	for key, hist := range clientLatency {
		targetID := strings.SplitN(key, "\xff", 2)[0]
		acc := upstreams[targetID]
		if acc == nil {
			acc = &upstreamAccum{}
			upstreams[targetID] = acc
		}
		if hist.cntRate > 0 && hist.sumRate > 0 {
			acc.latencySum += durationToMillis(key, hist.sumRate)
			acc.latencyRate += hist.cntRate
		}
	}

	upstreamCalls := make([]telemetry.UpstreamCall, 0, len(upstreams))
	for targetID, acc := range upstreams {
		if acc.callRate <= 0 {
			continue
		}
		meanLatency := 0.0
		if acc.latencyRate > 0 {
			meanLatency = acc.latencySum / acc.latencyRate
		}
		errRate := 0.0
		if acc.callRate > 0 {
			errRate = clamp(acc.errorCalls/acc.callRate, 0, 1)
		}
		upstreamCalls = append(upstreamCalls, telemetry.UpstreamCall{
			TargetServiceID: targetID,
			CallRate:        acc.callRate,
			ErrorRate:       errRate,
			LatencyMean:     meanLatency,
		})
	}
	sort.Slice(upstreamCalls, func(i, j int) bool {
		return upstreamCalls[i].TargetServiceID < upstreamCalls[j].TargetServiceID
	})

	memUsage := 0.0
	if target.MemoryLimitBytes > 0 && memBytes > 0 {
		memUsage = clamp(memBytes/float64(target.MemoryLimitBytes), 0, 1)
	}

	point := telemetry.MetricPoint{
		ServiceID:     target.ServiceID,
		Timestamp:     now,
		RequestRate:   positive(requestRate),
		ErrorRate:     errorRate,
		Latency:       latency,
		CPUUsage:      clamp(cpuRate, 0, 1),
		MemUsage:      memUsage,
		ActiveConns:   int64(math.Round(positive(activeConns))),
		QueueDepth:    int64(math.Round(positive(queueDepth))),
		UpstreamCalls: upstreamCalls,
	}

	return point, hasSignal(point)
}

func (n *Normalizer) counterRate(serviceID string, sample Sample, now time.Time) (float64, bool) {
	key := serviceID + "\xff" + sampleKey(sample)
	prev, ok := n.prev[key]
	n.prev[key] = counterPoint{value: sample.Value, seen: now}
	if !ok || prev.seen.IsZero() || !now.After(prev.seen) {
		return 0, false
	}
	if sample.Value < prev.value {
		return 0, false
	}
	dt := now.Sub(prev.seen).Seconds()
	if dt <= 0 {
		return 0, false
	}
	return (sample.Value - prev.value) / dt, true
}

func sampleKey(sample Sample) string {
	if len(sample.Labels) == 0 {
		return sample.Name
	}
	keys := make([]string, 0, len(sample.Labels))
	for key := range sample.Labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(sample.Name)
	b.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(sample.Labels[key])
	}
	b.WriteByte('}')
	return b.String()
}

func isRateGauge(name string) bool {
	return name == "request_rate" || name == "requests_per_second" || name == "http_requests_per_second"
}

func isQueueGauge(name string) bool {
	return strings.Contains(name, "queue_depth") ||
		strings.Contains(name, "queue_size") ||
		strings.Contains(name, "queued_jobs") ||
		strings.Contains(name, "jobs_queued") ||
		strings.Contains(name, "queue_messages") ||
		strings.Contains(name, "consumer_lag")
}

func isActiveGauge(name string) bool {
	return strings.Contains(name, "active_conns") ||
		strings.Contains(name, "active_connections") ||
		strings.Contains(name, "inflight_requests") ||
		strings.Contains(name, "in_flight_requests") ||
		strings.Contains(name, "active_requests")
}

func isMemoryGauge(name string) bool {
	return name == "process_resident_memory_bytes" ||
		name == "go_memstats_alloc_bytes" ||
		strings.Contains(name, "memory_usage_bytes")
}

func isCPUCumulative(name string) bool {
	return name == "process_cpu_seconds_total" || strings.Contains(name, "cpu_seconds_total")
}

func isServerRequestCounter(name string) bool {
	if strings.HasSuffix(name, "_bucket") || strings.HasSuffix(name, "_sum") || strings.HasSuffix(name, "_created") {
		return false
	}
	if strings.Contains(name, "client") || strings.Contains(name, "outbound") || strings.Contains(name, "dependency") {
		return false
	}
	return name == "http_requests_total" ||
		name == "requests_total" ||
		name == "http_server_requests_total" ||
		name == "grpc_server_handled_total" ||
		strings.HasSuffix(name, "_requests_total")
}

func isClientRequestCounter(name string) bool {
	if strings.HasSuffix(name, "_bucket") || strings.HasSuffix(name, "_sum") || strings.HasSuffix(name, "_created") {
		return false
	}
	if strings.Contains(name, "spanmetrics_calls_total") || name == "calls_total" {
		return true
	}
	return strings.Contains(name, "client") && strings.HasSuffix(name, "_total") ||
		strings.Contains(name, "outbound") && strings.HasSuffix(name, "_total") ||
		strings.Contains(name, "dependency") && strings.HasSuffix(name, "_total")
}

func isServerLatencyHistogram(name string) bool {
	if !isHistogramPart(name) {
		return false
	}
	if strings.Contains(name, "client") || strings.Contains(name, "outbound") || strings.Contains(name, "dependency") {
		return false
	}
	return strings.Contains(name, "duration") || strings.Contains(name, "latency")
}

func isClientLatencyHistogram(name string) bool {
	if !isHistogramPart(name) {
		return false
	}
	if strings.Contains(name, "spanmetrics_duration") {
		return true
	}
	return strings.Contains(name, "client") || strings.Contains(name, "outbound") || strings.Contains(name, "dependency")
}

func isHistogramPart(name string) bool {
	return strings.HasSuffix(name, "_bucket") || strings.HasSuffix(name, "_sum") || strings.HasSuffix(name, "_count")
}

func histogramBase(name string) (base, suffix string) {
	for _, suffix := range []string{"_bucket", "_sum", "_count"} {
		if strings.HasSuffix(name, suffix) {
			return strings.TrimSuffix(name, suffix), strings.TrimPrefix(suffix, "_")
		}
	}
	return "", ""
}

func parseBucket(value string) (float64, bool) {
	if value == "+Inf" || value == "Inf" || value == "inf" {
		return math.Inf(1), true
	}
	parsed, err := strconv.ParseFloat(value, 64)
	return parsed, err == nil
}

func latencyFromHistograms(histograms map[string]*histogramAccum) telemetry.LatencyStats {
	var out telemetry.LatencyStats
	var bestCount float64
	for base, hist := range histograms {
		if hist.cntRate <= 0 {
			continue
		}
		mean := 0.0
		if hist.sumRate > 0 {
			mean = durationToMillis(base, hist.sumRate/hist.cntRate)
		}
		p50 := histogramQuantile(0.50, hist.buckets)
		p95 := histogramQuantile(0.95, hist.buckets)
		p99 := histogramQuantile(0.99, hist.buckets)
		if p50 > 0 {
			p50 = durationToMillis(base, p50)
		}
		if p95 > 0 {
			p95 = durationToMillis(base, p95)
		}
		if p99 > 0 {
			p99 = durationToMillis(base, p99)
		}
		if hist.cntRate > bestCount {
			out = telemetry.LatencyStats{P50: p50, P95: p95, P99: p99, Mean: mean}
			bestCount = hist.cntRate
		}
	}
	return out
}

func histogramQuantile(q float64, buckets map[float64]float64) float64 {
	if len(buckets) == 0 {
		return 0
	}
	bounds := make([]float64, 0, len(buckets))
	for bound := range buckets {
		bounds = append(bounds, bound)
	}
	sort.Float64s(bounds)
	total := 0.0
	for _, bound := range bounds {
		if math.IsInf(bound, 1) {
			total = buckets[bound]
			break
		}
		total = math.Max(total, buckets[bound])
	}
	if total <= 0 {
		return 0
	}
	rank := q * total
	prevBound := 0.0
	prevCount := 0.0
	for _, bound := range bounds {
		count := buckets[bound]
		if count >= rank {
			if math.IsInf(bound, 1) {
				return prevBound
			}
			if count <= prevCount {
				return bound
			}
			fraction := (rank - prevCount) / (count - prevCount)
			return prevBound + fraction*(bound-prevBound)
		}
		if !math.IsInf(bound, 1) {
			prevBound = bound
		}
		prevCount = count
	}
	return prevBound
}

func durationToMillis(metricName string, value float64) float64 {
	name := strings.ToLower(metricName)
	switch {
	case strings.Contains(name, "milliseconds") || strings.Contains(name, "_ms"):
		return value
	case strings.Contains(name, "microseconds"):
		return value / 1000
	case strings.Contains(name, "nanoseconds"):
		return value / 1_000_000
	default:
		return value * 1000
	}
}

func isLatencyMeanGauge(name string) bool {
	return name == "latency_mean_ms" || name == "request_latency_mean_ms" || name == "http_latency_mean_ms"
}

func isLatencyP50Gauge(name string) bool {
	return name == "latency_p50_ms" || name == "request_latency_p50_ms" || strings.Contains(name, "p50_latency_ms")
}

func isLatencyP95Gauge(name string) bool {
	return name == "latency_p95_ms" || name == "request_latency_p95_ms" || strings.Contains(name, "p95_latency_ms")
}

func isLatencyP99Gauge(name string) bool {
	return name == "latency_p99_ms" || name == "request_latency_p99_ms" || strings.Contains(name, "p99_latency_ms")
}

func firstGauge(samples []Sample, predicate func(string) bool) float64 {
	for _, sample := range samples {
		if predicate(strings.ToLower(sample.Name)) && sample.Value > 0 {
			if strings.HasSuffix(strings.ToLower(sample.Name), "_seconds") {
				return sample.Value * 1000
			}
			return sample.Value
		}
	}
	return 0
}

func isErrorSample(labels map[string]string) bool {
	for _, key := range []string{"status", "code", "status_code", "http_status_code", "grpc_code", "outcome", "result", "error"} {
		value := strings.ToLower(strings.TrimSpace(labels[key]))
		if value == "" {
			continue
		}
		if value == "true" || value == "error" || value == "failed" || value == "failure" || value == "server_error" {
			return true
		}
		if strings.HasPrefix(value, "5") {
			return true
		}
		if key == "grpc_code" && value != "ok" && value != "0" {
			return true
		}
	}
	return false
}

func dependencyTarget(labels map[string]string) string {
	for _, key := range []string{
		"target_service", "peer_service", "peer.service", "server", "host",
		"net_peer_name", "net.peer.name", "remote_service", "destination_service",
		"destination", "dependency", "le_target", "db_system", "db.system",
	} {
		if value := sanitizeDependencyName(labels[key]); value != "" {
			return value
		}
	}
	return ""
}

func sanitizeDependencyName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "unknown" || value == "localhost" || value == "127.0.0.1" {
		return ""
	}
	value = strings.TrimPrefix(value, "http://")
	value = strings.TrimPrefix(value, "https://")
	if host, _, found := strings.Cut(value, ":"); found && host != "" {
		value = host
	}
	value = strings.Trim(value, "/")
	return value
}

func hasSignal(point telemetry.MetricPoint) bool {
	return point.RequestRate > 0 ||
		point.Latency.Mean > 0 ||
		point.CPUUsage > 0 ||
		point.MemUsage > 0 ||
		point.ActiveConns > 0 ||
		point.QueueDepth > 0 ||
		len(point.UpstreamCalls) > 0
}

func positive(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0
	}
	return v
}

func clamp(v, min, max float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return min
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
