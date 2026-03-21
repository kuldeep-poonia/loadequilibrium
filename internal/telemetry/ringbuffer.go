package telemetry

import (
	"math"
	"sync"
	"time"
)

// RingBuffer is a fixed-capacity circular buffer of MetricPoints.
// Writes overwrite the oldest entry when full.
// All operations are goroutine-safe.
type RingBuffer struct {
	mu   sync.RWMutex
	buf  []*MetricPoint
	head int // index of the next write slot
	size int // number of valid entries currently stored
	cap  int
}

// NewRingBuffer allocates a RingBuffer of the given capacity.
func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		buf: make([]*MetricPoint, capacity),
		cap: capacity,
	}
}

// Push appends a MetricPoint, evicting the oldest if the buffer is full.
func (r *RingBuffer) Push(p *MetricPoint) {
	r.mu.Lock()
	r.buf[r.head] = p
	r.head = (r.head + 1) % r.cap
	if r.size < r.cap {
		r.size++
	}
	r.mu.Unlock()
}

// Snapshot returns a copy of all valid entries in chronological order
// (oldest first). The returned slice is a new allocation — safe to read
// without holding the lock.
func (r *RingBuffer) Snapshot() []*MetricPoint {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.size == 0 {
		return nil
	}

	out := make([]*MetricPoint, r.size)
	start := (r.head - r.size + r.cap) % r.cap
	for i := 0; i < r.size; i++ {
		out[i] = r.buf[(start+i)%r.cap]
	}
	return out
}

// Last returns the most recently pushed point, or nil if empty.
func (r *RingBuffer) Last() *MetricPoint {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.size == 0 {
		return nil
	}
	idx := (r.head - 1 + r.cap) % r.cap
	return r.buf[idx]
}

// Size returns the current number of stored entries.
func (r *RingBuffer) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.size
}

// RingSummary is a compact statistical summary over the full ring buffer history.
// It avoids allocating the full point slice, making it safe for frequent calls
// on large buffers without GC pressure.
type RingSummary struct {
	Count          int
	MeanReqRate    float64
	StdReqRate     float64
	MeanLatencyMs  float64
	MaxLatencyMs   float64
	MeanErrorRate  float64
	OldestAt       time.Time
	NewestAt       time.Time
}

// SummaryStats computes a compact statistical summary over all stored points.
// This is used for historical telemetry summarisation without returning the
// full point slice.
func (r *RingBuffer) SummaryStats() RingSummary {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.size == 0 {
		return RingSummary{}
	}

	var sumReq, sumReqSq, sumLat, maxLat, sumErr float64
	start := (r.head - r.size + r.cap) % r.cap
	oldest := r.buf[start]
	newest := r.buf[(r.head-1+r.cap)%r.cap]

	for i := 0; i < r.size; i++ {
		p := r.buf[(start+i)%r.cap]
		if p == nil {
			continue
		}
		sumReq += p.RequestRate
		sumReqSq += p.RequestRate * p.RequestRate
		sumLat += p.Latency.Mean
		if p.Latency.Mean > maxLat {
			maxLat = p.Latency.Mean
		}
		sumErr += p.ErrorRate
	}

	n := float64(r.size)
	mean := sumReq / n
	variance := sumReqSq/n - mean*mean
	if variance < 0 {
		variance = 0
	}

	oldestAt := time.Time{}
	newestAt := time.Time{}
	if oldest != nil {
		oldestAt = oldest.Timestamp
	}
	if newest != nil {
		newestAt = newest.Timestamp
	}

	return RingSummary{
		Count:         r.size,
		MeanReqRate:   mean,
		StdReqRate:    math.Sqrt(variance),
		MeanLatencyMs: sumLat / n,
		MaxLatencyMs:  maxLat,
		MeanErrorRate: sumErr / n,
		OldestAt:      oldestAt,
		NewestAt:      newestAt,
	}
}
