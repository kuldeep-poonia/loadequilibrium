package layer3

// FILE: tests/layer3_race_concurrency/L3_TEL_002_ringbuffer_memory_stability_test.go
//
// Tests:   L3-TEL-002
// Package: github.com/loadequilibrium/loadequilibrium/internal/telemetry
// Methods: NewRingBuffer(capacity int) *RingBuffer
//          (*RingBuffer).Push(p *MetricPoint)
//
// RUN: go test ./tests/layer3_race_concurrency/ -run TestL3_TEL_002 -count=1 -timeout=300s -v
// NOTE: Do NOT run this with -race; memory profiling and race detector interact poorly.
//       Run separately: go test ... -run TestL3_TEL_002 -count=1

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/telemetry"
)

// ─────────────────────────────────────────────────────────────────────────────
// L3-TEL-002 — RingBuffer memory stability under 10 million Push calls
//
// AIM:   A RingBuffer with capacity=1000 must not grow heap beyond 2× its
//        initial allocation when 10,000,000 entries are pushed.  The buffer is
//        fixed-size by design — old entries must be overwritten, never retained.
//
// THRESHOLD: heap_growth_factor <= 2.0
// ON EXCEED: Buffer retains all pushed entries → heap grows O(N) with pushes →
//            OOM kill in Kubernetes after sustained telemetry ingestion →
//            CrashLoopBackOff on the loadequilibrium pod.
// ─────────────────────────────────────────────────────────────────────────────
func TestL3_TEL_002_RingBufferMemoryStability(t *testing.T) {
	if testing.Short() {
		t.Skip("L3-TEL-002: skipped in short mode (requires ~10M Push calls)")
	}

	start := time.Now()

	const (
		capacity = 1000           // ring size — must stay fixed
		entries  = 10_000_000     // total pushes
		gcEvery  = 1_000_000      // force GC every million pushes to get accurate heap snapshot
	)

	rb := telemetry.NewRingBuffer(capacity)

	// Warm up: fill the buffer once so GC pressure from the initial allocation
	// is already paid before we take the baseline measurement.
	for i := 0; i < capacity; i++ {
		rb.Push(&telemetry.MetricPoint{
			ServiceID:   "warmup",
			Timestamp:   time.Now(),
			RequestRate: float64(i),
		})
	}
	runtime.GC()
	runtime.GC() // two GC passes to collect any finalizer queue

	// ── Baseline ──────────────────────────────────────────────────────────────
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)
	heapBefore := memBefore.HeapInuse // HeapInuse is more stable than HeapAlloc

	// ── Push 10 million entries ───────────────────────────────────────────────
	type memorySample struct {
		afterN    int
		heapInuse uint64
	}
	samples := make([]memorySample, 0, entries/gcEvery)

	for i := 0; i < entries; i++ {
		rb.Push(&telemetry.MetricPoint{
			ServiceID:   "svc-stability",
			Timestamp:   time.Now(),
			RequestRate: float64(i % 10000),
			ErrorRate:   0.01,
			Latency: telemetry.LatencyStats{
				Mean: 50.0,
				P50:  45.0,
				P95:  80.0,
				P99:  100.0,
			},
			ActiveConns: 10,
			QueueDepth:  5,
		})

		if (i+1)%gcEvery == 0 {
			runtime.GC()
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			samples = append(samples, memorySample{
				afterN:    i + 1,
				heapInuse: ms.HeapInuse,
			})
		}
	}

	// ── Final measurement ─────────────────────────────────────────────────────
	runtime.GC()
	runtime.GC()
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)
	heapAfter := memAfter.HeapInuse

	// Growth factor relative to post-warmup baseline.
	// Add 1 to heapBefore to prevent division by zero on fresh allocators.
	growthFactor := float64(heapAfter) / float64(heapBefore+1)

	// Verify ring buffer is still at capacity (last Size() call).
	finalSize := rb.Size()
	sizeCorrect := finalSize == capacity

	passed := growthFactor <= 2.0 && sizeCorrect

	// Build sample string for results.
	sampleDescs := make([]string, 0, len(samples))
	for _, s := range samples {
		sampleDescs = append(sampleDescs, fmt.Sprintf(
			"after_%dM: heap=%dKB", s.afterN/1_000_000, s.heapInuse/1024,
		))
	}

	durationMs := time.Since(start).Milliseconds()

	writeL3Result(L3Record{
		TestID: "L3-TEL-002",
		Layer:  3,
		Name:   "RingBuffer memory stability under 10M pushes",
		Aim: fmt.Sprintf(
			"Pushing %d entries into RingBuffer(capacity=%d) must not grow HeapInuse beyond 2× baseline",
			entries, capacity,
		),
		PackagesInvolved: []string{"internal/telemetry"},
		FunctionsTested:  []string{"NewRingBuffer", "(*RingBuffer).Push", "(*RingBuffer).Size"},
		Threshold: L3Threshold{
			Metric:    "heap_inuse_growth_factor",
			Operator:  "<=",
			Value:     2.0,
			Unit:      "ratio",
			Rationale: "Fixed-size ring buffer must overwrite old slots; heap growth > 2× means entries are being retained",
		},
		Result: L3ResultData{
			Status:              l3Status(passed),
			ActualValue:         growthFactor,
			ActualUnit:          "growth_factor_x",
			OperationsCompleted: entries,
			RaceDetectorActive:  false, // intentionally run without -race for memory test
			DurationMs:          durationMs,
			ErrorMessages: append(sampleDescs, fmt.Sprintf(
				"heap_before=%dKB heap_after=%dKB growth=%.4fx final_size=%d(expected %d)",
				heapBefore/1024, heapAfter/1024, growthFactor, finalSize, capacity,
			)),
		},
		OnExceed: "RingBuffer retains pushed MetricPoints beyond its capacity → heap grows O(N) → " +
			"Kubernetes OOM kill → pod CrashLoopBackOff → total telemetry loss",
		Questions: L3Questions{
			WhatWasTested: fmt.Sprintf(
				"RingBuffer(capacity=%d).Push called %d times; HeapInuse sampled every %d pushes with forced GC",
				capacity, entries, gcEvery,
			),
			WhyThisThreshold:    "2× allows for GC overhead and Go runtime internals; any larger growth means entries are escaping the fixed-size ring",
			WhatHappensIfFails:  "Memory grows proportionally to ingestion volume → OOM kill in production after hours of operation",
			HowRacesWereDetected: "N/A — single-goroutine memory test; run TestL3_TEL_001 for concurrent safety",
			HowLeaksWereDetected: "runtime.MemStats.HeapInuse before and after 10M pushes with GC forced between samples",
			WhatConcurrencyPattern: "Single-writer sequential push to verify allocation semantics, not concurrent safety",
		},
		RunAt:     l3Now(),
		GoVersion: l3GoVer(),
	})

	if !passed {
		t.Fatalf(
			"L3-TEL-002 FAILED: growth=%.4fx (threshold=2.0x) heap_before=%dKB heap_after=%dKB size=%d(expected %d)\n"+
				"FIX: RingBuffer.Push must overwrite r.buf[r.head] with the new pointer and advance head.\n"+
				"     If r.buf[r.head] is not overwritten but appended, the slice grows → O(N) heap.\n"+
				"     Verify: r.buf[r.head] = p  (not append)  in internal/telemetry/ringbuffer.go",
			growthFactor, heapBefore/1024, heapAfter/1024, finalSize, capacity,
		)
	}

	t.Logf(
		"L3-TEL-002 PASS | entries=%d growth=%.4fx heap_before=%dKB heap_after=%dKB size=%d",
		entries, growthFactor, heapBefore/1024, heapAfter/1024, finalSize,
	)
}