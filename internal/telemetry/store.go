package telemetry

import (
	"math"
	"sync"
	"time"
)

// Store is the central in-memory registry of all service telemetry.
// It maintains one ring buffer per service, enforces a cardinality cap,
// and prunes services that have not reported within the stale age.
type Store struct {
	mu       sync.RWMutex
	buffers  map[string]*RingBuffer
	lastSeen map[string]time.Time
	bufCap   int
	maxSvc   int
	staleAge time.Duration
}

func NewStore(bufferCapacity, maxServices int, staleAge time.Duration) *Store {
	return &Store{
		buffers:  make(map[string]*RingBuffer),
		lastSeen: make(map[string]time.Time),
		bufCap:   bufferCapacity,
		maxSvc:   maxServices,
		staleAge: staleAge,
	}
}

func sanitizePoint(p *MetricPoint) bool {
	if p.ServiceID == "" {
		return false
	}
	// Clamp to physically meaningful ranges.
	if p.RequestRate < 0 {
		p.RequestRate = 0
	}
	if p.ErrorRate < 0 {
		p.ErrorRate = 0
	}
	if p.ErrorRate > 1 {
		p.ErrorRate = 1 // error rate is a fraction [0,1]
	}
	// Latency values of 0 cause division-by-zero in service rate formula.
	// Floor at 0.1ms — if a service truly responds in <0.1ms, physics is fine.
	const minLatencyMs = 0.1
	if p.Latency.Mean < 0 {
		p.Latency.Mean = 0
	}
	if p.Latency.P50 < 0 {
		p.Latency.P50 = 0
	}
	if p.Latency.P95 < 0 {
		p.Latency.P95 = 0
	}
	if p.Latency.P99 < 0 {
		p.Latency.P99 = 0
	}
	// A mean of 0 with any other latency percentile set is suspicious but legal.
	// We only floor when ALL are zero to avoid masking real ultra-fast services.
	if p.Latency.Mean == 0 && p.Latency.P50 == 0 && p.Latency.P95 == 0 {
		p.Latency.Mean = minLatencyMs
	}
	if p.ActiveConns < 0 {
		p.ActiveConns = 0
	}
	if p.QueueDepth < 0 {
		p.QueueDepth = 0
	}
	return true
}

// Ingest appends a MetricPoint. New services are admitted up to maxSvc.
func (s *Store) Ingest(p *MetricPoint) {
	if !sanitizePoint(p) {
		return
	}
	if p.Timestamp.IsZero() {
		p.Timestamp = time.Now()
	}

	s.mu.RLock()
	buf, ok := s.buffers[p.ServiceID]
	s.mu.RUnlock()

	if !ok {
		s.mu.Lock()
		if buf, ok = s.buffers[p.ServiceID]; !ok {
			if len(s.buffers) >= s.maxSvc {
				s.mu.Unlock()
				return // cardinality cap — drop new service
			}
			buf = NewRingBuffer(s.bufCap)
			s.buffers[p.ServiceID] = buf
		}
		s.mu.Unlock()
	}

	buf.Push(p)

	s.mu.Lock()
	s.lastSeen[p.ServiceID] = p.Timestamp
	s.mu.Unlock()
}

// Prune removes services not seen within staleAge. Call from tick loop.
func (s *Store) Prune(now time.Time) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var pruned []string
	for id, t := range s.lastSeen {
		if now.Sub(t) > s.staleAge {
			delete(s.buffers, id)
			delete(s.lastSeen, id)
			pruned = append(pruned, id)
		}
	}
	return pruned
}

func (s *Store) ServiceIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.buffers))
	for id := range s.buffers {
		ids = append(ids, id)
	}
	return ids
}

// Window computes a ServiceWindow over the most recent n points.
// Returns nil if the service is unknown or has no data.
// freshnessCutoff: if the newest point is older than this, returns nil (stale).
func (s *Store) Window(serviceID string, n int, freshnessCutoff time.Duration) *ServiceWindow {
	s.mu.RLock()
	buf, ok := s.buffers[serviceID]
	s.mu.RUnlock()
	if !ok {
		return nil
	}
	points := buf.Snapshot()
	if len(points) == 0 {
		return nil
	}
	// Freshness check.
	newest := points[len(points)-1]
	if freshnessCutoff > 0 && time.Since(newest.Timestamp) > freshnessCutoff {
		return nil
	}
	if n > 0 && len(points) > n {
		points = points[len(points)-n:]
	}
	return computeWindow(serviceID, points)
}

// AllWindows returns windows for every known service.
func (s *Store) AllWindows(n int, freshnessCutoff time.Duration) map[string]*ServiceWindow {
	ids := s.ServiceIDs()
	out := make(map[string]*ServiceWindow, len(ids))
	for _, id := range ids {
		if w := s.Window(id, n, freshnessCutoff); w != nil {
			out[id] = w
		}
	}
	return out
}

func computeWindow(serviceID string, pts []*MetricPoint) *ServiceWindow {
	n := float64(len(pts))
	var sumReq, sumErr, sumLat, maxLat, sumCPU, sumMem, sumQueue, sumConns float64
	edgeSums := make(map[string]*edgeAccum)

	for _, p := range pts {
		sumReq += p.RequestRate
		sumErr += p.ErrorRate
		sumLat += p.Latency.Mean
		if p.Latency.Mean > maxLat {
			maxLat = p.Latency.Mean
		}
		sumCPU += p.CPUUsage
		sumMem += p.MemUsage
		sumQueue += float64(p.QueueDepth)
		sumConns += float64(p.ActiveConns)
		for _, uc := range p.UpstreamCalls {
			acc, exists := edgeSums[uc.TargetServiceID]
			if !exists {
				acc = &edgeAccum{target: uc.TargetServiceID}
				edgeSums[uc.TargetServiceID] = acc
			}
			acc.sumCallRate += uc.CallRate
			acc.sumErrRate += uc.ErrorRate
			acc.sumLatency += uc.LatencyMean
			acc.count++
		}
	}

	last := pts[len(pts)-1]

	// Compute meanReq and stdReq first — used by both inference and window.
	meanReq := sumReq / n
	var sumSqDiff float64
	for _, p := range pts {
		d := p.RequestRate - meanReq
		sumSqDiff += d * d
	}
	stdReq := 0.0
	if n > 1 {
		stdReq = math.Sqrt(sumSqDiff / (n - 1))
	}

	// ── Missing metric inference ──────────────────────────────────────────────
	// When a metric is absent or zero, infer it from related signals rather than
	// leaving it at zero (which would cause downstream models to produce wrong results).

	// P99 latency inference: use Last→P95→Mean progression if P99 is absent.
	lastP99 := last.Latency.P99
	if lastP99 <= 0 && last.Latency.P95 > 0 {
		// P99 ≈ P95 × 1.20 (heuristic from exponential tail shape)
		lastP99 = last.Latency.P95 * 1.20
	}
	if lastP99 <= 0 && last.Latency.Mean > 0 {
		// P99 ≈ Mean × 2.5 (exponential distribution approximation)
		lastP99 = last.Latency.Mean * 2.5
	}

	// Active connections inference: when zero but request rate is non-zero,
	// estimate from Little's Law: L ≈ λ × E[S] where E[S] ≈ mean latency.
	meanConns := sumConns / n
	if meanConns < 1.0 && meanReq > 0 && sumLat > 0 {
		// E[conns] = λ × E[latency_sec] (Little's Law steady-state)
		inferredConns := meanReq * (sumLat / n / 1000.0)
		if inferredConns >= 1.0 {
			meanConns = inferredConns
		} else {
			meanConns = 1.0 // floor — at least one connection when there is traffic
		}
	}

	// Queue depth inference: when missing but utilisation is estimable.
	// If MeanLatencyMs is significantly above a baseline (2× P50), queue is likely non-zero.
	lastQueueDepth := float64(last.QueueDepth)
	meanQueueDepth := sumQueue / n
	if lastQueueDepth == 0 && last.Latency.P50 > 0 && last.Latency.Mean > last.Latency.P50*1.5 {
		// Latency is inflated above median → queue is likely building.
		// Estimate: excess latency fraction × mean active connections.
		excessFrac := (last.Latency.Mean - last.Latency.P50) / last.Latency.Mean
		lastQueueDepth = excessFrac * meanConns
	}

	// Signal confidence: product of three quality dimensions.
	// 1) Sample adequacy: saturates at 1.0 for ≥30 samples.
	sampleConf := 1.0 - math.Exp(-float64(len(pts))/15.0)
	// 2) Arrival stability: high CoV → lower confidence.
	cov := 0.0
	if meanReq > 0 {
		cov = stdReq / meanReq
	}
	stabilityConf := math.Exp(-cov * 0.5)
	// 3) Freshness: age since last observation relative to 2s nominal tick.
	ageSec := time.Since(last.Timestamp).Seconds()
	freshnessConf := math.Exp(-ageSec / 6.0) // half-life = ~4s
	confidence := sampleConf * stabilityConf * freshnessConf

	quality := "good"
	switch {
	case len(pts) < 3 || confidence < 0.3:
		quality = "sparse"
	case confidence < 0.65:
		quality = "degraded"
	}

	edges := make(map[string]EdgeWindow, len(edgeSums))
	for tid, acc := range edgeSums {
		c := float64(acc.count)
		edges[tid] = EdgeWindow{
			TargetServiceID: tid,
			MeanCallRate:    acc.sumCallRate / c,
			MeanErrorRate:   acc.sumErrRate / c,
			MeanLatencyMs:   acc.sumLatency / c,
		}
	}

	return &ServiceWindow{
		ServiceID:        serviceID,
		ComputedAt:       time.Now(),
		SampleCount:      len(pts),
		MeanRequestRate:  meanReq,
		StdRequestRate:   stdReq,
		LastRequestRate:  last.RequestRate,
		MeanLatencyMs:    sumLat / n,
		MaxLatencyMs:     maxLat,
		LastLatencyMs:    last.Latency.Mean,
		LastP99LatencyMs: lastP99,
		MeanErrorRate:    sumErr / n,
		LastErrorRate:    last.ErrorRate,
		MeanCPU:          sumCPU / n,
		MeanMem:          sumMem / n,
		MeanQueueDepth:   meanQueueDepth,
		LastQueueDepth:   lastQueueDepth,
		MeanActiveConns:  meanConns,
		UpstreamEdges:    edges,
		LastObservedAt:   last.Timestamp,
		ConfidenceScore:  confidence,
		SignalQuality:    quality,
	}
}

type edgeAccum struct {
	target      string
	sumCallRate float64
	sumErrRate  float64
	sumLatency  float64
	count       int
}
