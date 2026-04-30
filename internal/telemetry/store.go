package telemetry

import (
	"math"
	"sync"
	"time"
)

type Store struct {
	mu       sync.RWMutex
	buffers  map[string]*RingBuffer
	lastSeen map[string]time.Time
	bufCap   int
	maxSvc   int
	staleAge time.Duration
}

func finiteOrZero(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
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

// StaleAge returns the prune threshold configured for this Store.
func (s *Store) StaleAge() time.Duration {
	return s.staleAge
}

func sanitizePoint(p *MetricPoint) bool {
	if p.ServiceID == "" {
		return false
	}
	// Clamp to physically meaningful ranges.
	p.RequestRate = finiteOrZero(p.RequestRate)
	p.ErrorRate = finiteOrZero(p.ErrorRate)
	p.Latency.Mean = finiteOrZero(p.Latency.Mean)
	p.Latency.P50 = finiteOrZero(p.Latency.P50)
	p.Latency.P95 = finiteOrZero(p.Latency.P95)
	p.Latency.P99 = finiteOrZero(p.Latency.P99)
	p.CPUUsage = finiteOrZero(p.CPUUsage)
	p.MemUsage = finiteOrZero(p.MemUsage)
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

	s.mu.Lock()
	buf, ok := s.buffers[p.ServiceID]
	if !ok {
		if len(s.buffers) >= s.maxSvc {
			s.mu.Unlock()
			return
		}
		buf = NewRingBuffer(s.bufCap)
		s.buffers[p.ServiceID] = buf
	}
	s.lastSeen[p.ServiceID] = p.Timestamp
	s.mu.Unlock()

	// Push uses the RingBuffer's internal lock, not the Store's lock
	buf.Push(p)
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

// HasServices returns true if the store has any buffered services.
// Zero allocation, O(1) — safe to call in hot paths (e.g. adaptInterval).
func (s *Store) HasServices() bool {
	s.mu.RLock()
	n := len(s.buffers)
	s.mu.RUnlock()
	return n > 0
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

	last := buf.Last()
	if last.Timestamp.IsZero() {
		return nil
	}
	if freshnessCutoff > 0 && time.Since(last.Timestamp) > freshnessCutoff {
		return nil
	}

	points := buf.Snapshot(n)
	if len(points) == 0 {
		return nil
	}

	return computeWindow(serviceID, points)
}

// AllWindows returns windows for every known service.
func (s *Store) AllWindows(n int, freshnessCutoff time.Duration) map[string]*ServiceWindow {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make(map[string]*ServiceWindow, len(s.buffers))
	for id, buf := range s.buffers {
		last := buf.Last()
		if last.Timestamp.IsZero() {
			continue
		}
		if freshnessCutoff > 0 && time.Since(last.Timestamp) > freshnessCutoff {
			continue
		}

		points := buf.Snapshot(n)
		if len(points) == 0 {
			continue
		}

		w := computeWindow(id, points)
		if w != nil {
			out[id] = w
		}
	}
	return out
}

func computeWindow(serviceID string, pts []MetricPoint) *ServiceWindow {
	n := float64(len(pts))
	var sumReq, sumErr, sumLat, maxLat, sumCPU, sumMem, sumQueue, sumConns float64
	var edgeSums map[string]*edgeAccum

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
			uc.CallRate = finiteOrZero(uc.CallRate)
			uc.ErrorRate = finiteOrZero(uc.ErrorRate)
			uc.LatencyMean = finiteOrZero(uc.LatencyMean)
			if edgeSums == nil {
				edgeSums = make(map[string]*edgeAccum)
			}
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
		if meanQueueDepth == 0 {
			meanQueueDepth = lastQueueDepth / n
		}
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
