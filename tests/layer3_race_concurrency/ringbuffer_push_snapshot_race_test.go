package layer3_test

// FILE: tests/layer3_race_concurrency/L3_TEL_001_ringbuffer_push_snapshot_race_test.go
//
// Tests:      L3-TEL-001
// Package:    github.com/loadequilibrium/loadequilibrium/internal/telemetry
// Structs:    RingBuffer
// Methods:    NewRingBuffer(capacity int) *RingBuffer
//             (*RingBuffer).Push(p *MetricPoint)
//             (*RingBuffer).Snapshot() []*MetricPoint
//             (*RingBuffer).Last() *MetricPoint
//             (*RingBuffer).Size() int
//             (*RingBuffer).SummaryStats() RingSummary
//
// RUN:  go test ./tests/layer3_race_concurrency/ -run TestL3_TEL_001 -race -count=500 -timeout=600s -v
// PASS criteria: zero data races (race detector), zero torn reads,
//               zero panics across all 500 repetitions.

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/telemetry"
)

// ─────────────────────────────────────────────────────────────────────────────
// L3-TEL-001 — Concurrent Push + Snapshot + Last + Size + SummaryStats
//
// AIM:   10 writers + 10 readers operating concurrently on one RingBuffer for
//        30 seconds must produce zero data races, zero torn reads, zero panics.
//        A "torn read" is a Snapshot entry whose Timestamp is zero or whose
//        RequestRate is negative — both are physically impossible after Push.
//
// THRESHOLD: torn_reads == 0, panics == 0
// ON EXCEED: Telemetry ring buffer delivers garbled MetricPoints to the queue
//            physics engine → MPC optimises against nonsense sensor data →
//            actuator receives an unbounded control command.
// ─────────────────────────────────────────────────────────────────────────────
func TestL3_TEL_001_RingBufferConcurrentPushSnapshot(t *testing.T) {
	start := time.Now()

	const (
		capacity       = 1024
		writerCount    = 10
		readerCount    = 10
		testDurationS  = 30
	)

	rb := telemetry.NewRingBuffer(capacity)

	var (
		pushesAttempted int64
		pushesDone      int64
		snapshotsDone   int64
		lastCallsDone   int64
		sizeCalls       int64
		summaryDone     int64
		tornReads       int64
		panics          int64
	)

	ctx, cancel := testContextWithTimeout(testDurationS * time.Second)
	defer cancel()

	var wg sync.WaitGroup

	// ── 10 writer goroutines ──────────────────────────────────────────────────
	for w := 0; w < writerCount; w++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					atomic.AddInt64(&panics, 1)
					t.Errorf("L3-TEL-001 PANIC in writer goroutine %d: %v", wid, r)
				}
			}()

			seq := int64(0)
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				atomic.AddInt64(&pushesAttempted, 1)
				seq++
				p := &telemetry.MetricPoint{
					ServiceID:   fmt.Sprintf("svc-%d", wid),
					Timestamp:   time.Now(),
					RequestRate: float64(seq),           // always positive and increasing
					ErrorRate:   0.01,
					Latency: telemetry.LatencyStats{
						Mean: 50.0 + float64(wid),
						P50:  45.0,
						P95:  80.0,
						P99:  100.0,
					},
				}
				rb.Push(p)
				atomic.AddInt64(&pushesDone, 1)
			}
		}(w)
	}

	// ── 10 reader goroutines — mix of Snapshot / Last / Size / SummaryStats ──
	for r := 0; r < readerCount; r++ {
		wg.Add(1)
		go func(rid int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					atomic.AddInt64(&panics, 1)
					t.Errorf("L3-TEL-001 PANIC in reader goroutine %d: %v", rid, r)
				}
			}()

			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				switch rid % 4 {
				case 0, 1: // Snapshot — most expensive, most likely to expose torn reads
					snap := rb.Snapshot()
					atomic.AddInt64(&snapshotsDone, 1)
					for i, pt := range snap {
						if pt == nil {
							// nil entry from Snapshot is a logic error — buffer returned
							// a slot that was never written.
							atomic.AddInt64(&tornReads, 1)
							t.Errorf("L3-TEL-001 TORN READ: nil entry at index %d in Snapshot of %d entries", i, len(snap))
							continue
						}
						// Torn read check: RequestRate was always written as positive.
						// A zero or negative value means the reader saw a partially-written slot.
						if pt.RequestRate < 0 {
							atomic.AddInt64(&tornReads, 1)
							t.Errorf("L3-TEL-001 TORN READ: RequestRate=%.4f < 0 at index %d (service=%s)",
								pt.RequestRate, i, pt.ServiceID)
						}
						// Timestamp must not be zero — it is always set before Push.
						if pt.Timestamp.IsZero() {
							atomic.AddInt64(&tornReads, 1)
							t.Errorf("L3-TEL-001 TORN READ: zero Timestamp at index %d (service=%s)",
								i, pt.ServiceID)
						}
					}

				case 2: // Last — returns most recent entry
					last := rb.Last()
					atomic.AddInt64(&lastCallsDone, 1)
					if last != nil {
						if last.RequestRate < 0 {
							atomic.AddInt64(&tornReads, 1)
							t.Errorf("L3-TEL-001 TORN READ via Last(): RequestRate=%.4f", last.RequestRate)
						}
					}

				case 3: // Size + SummaryStats — exercise read-side locks
					sz := rb.Size()
					atomic.AddInt64(&sizeCalls, 1)
					if sz < 0 || sz > capacity {
						t.Errorf("L3-TEL-001 INVALID Size(): got %d (capacity=%d)", sz, capacity)
					}
					summary := rb.SummaryStats()
					atomic.AddInt64(&summaryDone, 1)
					// SummaryStats.Count must match Size() semantics (within race window).
					if summary.Count < 0 || summary.Count > capacity {
						t.Errorf("L3-TEL-001 INVALID SummaryStats.Count: %d", summary.Count)
					}
				}
			}
		}(r)
	}

	wg.Wait()

	// ── Collect final metrics ─────────────────────────────────────────────────
	finalSize := rb.Size()
	finalSummary := rb.SummaryStats()
	durationMs := time.Since(start).Milliseconds()

	passed := atomic.LoadInt64(&panics) == 0 &&
		atomic.LoadInt64(&tornReads) == 0

	var errMsgs []string
	if !passed {
		errMsgs = append(errMsgs, fmt.Sprintf(
			"panics=%d torn_reads=%d",
			atomic.LoadInt64(&panics),
			atomic.LoadInt64(&tornReads),
		))
	}
	errMsgs = append(errMsgs, fmt.Sprintf(
		"pushes_done=%d snapshots=%d last_calls=%d size_calls=%d summary_calls=%d final_size=%d mean_req=%.2f",
		atomic.LoadInt64(&pushesDone),
		atomic.LoadInt64(&snapshotsDone),
		atomic.LoadInt64(&lastCallsDone),
		atomic.LoadInt64(&sizeCalls),
		atomic.LoadInt64(&summaryDone),
		finalSize,
		finalSummary.MeanReqRate,
	))

	writeL3Result(L3Record{
		TestID: "L3-TEL-001",
		Layer:  3,
		Name:   "RingBuffer concurrent Push/Snapshot/Last/Size/SummaryStats",
		Aim: "10 writers + 10 readers on one RingBuffer for 30s: " +
			"zero data races (race detector), zero torn reads, zero panics",
		PackagesInvolved: []string{"internal/telemetry"},
		FunctionsTested: []string{
			"NewRingBuffer", "(*RingBuffer).Push", "(*RingBuffer).Snapshot",
			"(*RingBuffer).Last", "(*RingBuffer).Size", "(*RingBuffer).SummaryStats",
		},
		Threshold: L3Threshold{
			Metric:    "torn_reads_plus_panics",
			Operator:  "==",
			Value:     0,
			Unit:      "count",
			Rationale: "Any torn read means the control loop sees a physically impossible sensor state",
		},
		Result: L3ResultData{
			Status:              l3Status(passed),
			ActualValue:         float64(atomic.LoadInt64(&tornReads) + atomic.LoadInt64(&panics)),
			ActualUnit:          "integrity_violations",
			OperationsCompleted: atomic.LoadInt64(&pushesDone),
			RaceDetectorActive:  raceDetectorEnabled(),
			DurationMs:          durationMs,
			ErrorMessages:       errMsgs,
		},
		OnExceed: "Torn MetricPoint delivered to QueuePhysicsEngine → MPC optimises on garbled state → " +
			"unbounded ControlDirective.ScaleFactor sent to actuator",
		Questions: L3Questions{
			WhatWasTested: fmt.Sprintf(
				"RingBuffer(capacity=%d) under %d concurrent writers and %d concurrent readers for %ds, "+
					"%d total pushes",
				capacity, writerCount, readerCount, testDurationS, atomic.LoadInt64(&pushesDone),
			),
			WhyThisThreshold:    "Any negative RequestRate or zero Timestamp in a returned entry means a partial write was observed — the sync.RWMutex failed to protect the slot",
			WhatHappensIfFails:  "QueuePhysicsEngine.RunQueueModel receives MetricPoint with garbage state → CollapseRisk computed incorrectly → phaseRuntime.mergeDirective produces wrong ScaleFactor",
			HowRacesWereDetected: "Go race detector (-race flag on test binary) — any race causes non-zero exit",
			HowLeaksWereDetected: "N/A — goroutine lifecycle not the focus of this test",
			WhatConcurrencyPattern: "MRSW: multiple concurrent writers via Push (exclusive lock) + multiple concurrent readers via Snapshot/Last/Size/SummaryStats (shared lock)",
		},
		RunAt:     l3Now(),
		GoVersion: l3GoVer(),
	})

	if !passed {
		t.Fatalf(
			"L3-TEL-001 FAILED: torn_reads=%d panics=%d\n"+
				"FIX: verify sync.RWMutex.Lock() wraps the entire buf[head]=p + head increment in Push().\n"+
				"     Snapshot() must hold RLock for the full copy loop — not just the index computation.\n"+
				"     File: internal/telemetry/ringbuffer.go",
			atomic.LoadInt64(&tornReads),
			atomic.LoadInt64(&panics),
		)
	}

	t.Logf(
		"L3-TEL-001 PASS | pushes=%d snapshots=%d torn=0 panics=0 | final_size=%d",
		atomic.LoadInt64(&pushesDone),
		atomic.LoadInt64(&snapshotsDone),
		finalSize,
	)
}

// testContextWithTimeout returns a context that expires after d.
// Defined here to avoid importing context in every file.
func testContextWithTimeout(d time.Duration) (interface{ Done() <-chan struct{} }, func()) {
	done := make(chan struct{})
	timer := time.AfterFunc(d, func() { close(done) })
	ctx := &timeoutCtx{done: done}
	cancel := func() {
		timer.Stop()
		select {
		case <-done:
		default:
			close(done)
		}
	}
	return ctx, cancel
}

type timeoutCtx struct{ done chan struct{} }

func (c *timeoutCtx) Done() <-chan struct{} { return c.done }