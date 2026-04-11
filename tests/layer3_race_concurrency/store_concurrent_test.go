package layer3_test

// FILE: tests/layer3_race_concurrency/L3_TEL_003_store_concurrent_test.go
//
// Tests:   L3-TEL-003, L3-TEL-004
// Package: github.com/loadequilibrium/loadequilibrium/internal/telemetry
// Struct:  Store
// Methods: NewStore(bufferCapacity, maxServices int, staleAge time.Duration) *Store
//          (*Store).Ingest(p *MetricPoint)
//          (*Store).Prune(now time.Time) []string
//          (*Store).AllWindows(n int, freshnessCutoff time.Duration) map[string]*ServiceWindow
//          (*Store).Window(serviceID string, n int, freshnessCutoff time.Duration) *ServiceWindow
//          (*Store).ServiceIDs() []string
//
// RUN: go test ./tests/layer3_race_concurrency/ -run TestL3_TEL_003 -race -count=500 -timeout=600s -v
//      go test ./tests/layer3_race_concurrency/ -run TestL3_TEL_004 -race -count=1  -timeout=60s  -v

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/telemetry"
)

// ─────────────────────────────────────────────────────────────────────────────
// L3-TEL-003 — Store: concurrent Ingest + Prune + AllWindows + ServiceIDs
//
// AIM:   20 ingester goroutines, 5 pruner goroutines, 5 reader goroutines
//        operating concurrently on one Store for 30 seconds must produce zero
//        data races, zero panics, and zero nil-pointer dereferences.
//
// THRESHOLD: panics == 0, nil_window_deref == 0
// ON EXCEED: Race on Store.buffers map → concurrent map read/write panic →
//            entire orchestrator tick goroutine crashes → control loop halts.
// ─────────────────────────────────────────────────────────────────────────────
func TestL3_TEL_003_StoreConcurrentIngestPruneAllWindows(t *testing.T) {
	start := time.Now()

	const (
		bufCap      = 256
		maxSvc      = 50
		staleAge    = 2 * time.Second
		ingesters   = 20
		pruners     = 5
		readers     = 5
		serviceCount = 10 // rotate across N service IDs to create delete/re-add cycles
		durationS   = 30
	)

	store := telemetry.NewStore(bufCap, maxSvc, staleAge)

	var (
		ingestsDone int64
		prunesDone  int64
		readsDone   int64
		panics      int64
		nilDeref    int64
	)

	ctx, cancel := testContextWithTimeout(durationS * time.Second)
	defer cancel()

	var wg sync.WaitGroup

	// ── 20 ingester goroutines ────────────────────────────────────────────────
	// Each ingester rotates through serviceCount service IDs so that some
	// services are continuously created, pruned, and re-created — exercising
	// the double-checked locking in Store.Ingest.
	for i := 0; i < ingesters; i++ {
		wg.Add(1)
		go func(iid int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					atomic.AddInt64(&panics, 1)
					t.Errorf("L3-TEL-003 PANIC in ingester %d: %v", iid, r)
				}
			}()

			seq := int64(0)
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				seq++
				svcID := fmt.Sprintf("svc-%03d", (iid*serviceCount+int(seq))%serviceCount)
				store.Ingest(&telemetry.MetricPoint{
					ServiceID:   svcID,
					Timestamp:   time.Now(),
					RequestRate: float64(seq % 1000),
					ErrorRate:   0.01,
					Latency: telemetry.LatencyStats{
						Mean: 50.0 + float64(iid),
						P50:  45.0,
						P95:  80.0,
						P99:  100.0,
					},
					ActiveConns: int64(seq % 100),
					QueueDepth:  int64(seq % 50),
				})
				atomic.AddInt64(&ingestsDone, 1)
			}
		}(i)
	}

	// ── 5 pruner goroutines ───────────────────────────────────────────────────
	// Pruners call Store.Prune with a very aggressive threshold so that services
	// get deleted while ingesters are simultaneously creating them.
	// This exercises the concurrent map delete + RLock path.
	for p := 0; p < pruners; p++ {
		wg.Add(1)
		go func(pid int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					atomic.AddInt64(&panics, 1)
					t.Errorf("L3-TEL-003 PANIC in pruner %d: %v", pid, r)
				}
			}()

			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				// Prune with time.Now() — removes services not seen within staleAge.
				_ = store.Prune(time.Now())
				atomic.AddInt64(&prunesDone, 1)
				time.Sleep(100 * time.Millisecond)
			}
		}(p)
	}

	// ── 5 reader goroutines ───────────────────────────────────────────────────
	// Readers call AllWindows and Window — both acquire RLock while pruners
	// and ingesters hold or wait for the write lock.
	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func(rid int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					atomic.AddInt64(&panics, 1)
					t.Errorf("L3-TEL-003 PANIC in reader %d: %v", rid, r)
				}
			}()

			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				windows := store.AllWindows(64, 5*time.Second)
				atomic.AddInt64(&readsDone, 1)

				// Dereference every returned window to verify it is not
				// a dangling pointer from a concurrent Prune.
				for svcID, w := range windows {
					if w == nil {
						atomic.AddInt64(&nilDeref, 1)
						t.Errorf("L3-TEL-003 NIL WINDOW: AllWindows returned nil for service %q", svcID)
						continue
					}
					// Verify the window fields are self-consistent.
					if w.MeanRequestRate < 0 {
						t.Errorf("L3-TEL-003 INVALID WINDOW: MeanRequestRate=%.4f < 0 for %q",
							w.MeanRequestRate, svcID)
					}
					if w.SampleCount < 0 {
						t.Errorf("L3-TEL-003 INVALID WINDOW: SampleCount=%d < 0 for %q",
							w.SampleCount, svcID)
					}
					if w.ConfidenceScore < 0 || w.ConfidenceScore > 1.0001 {
						t.Errorf("L3-TEL-003 INVALID WINDOW: ConfidenceScore=%.6f outside [0,1] for %q",
							w.ConfidenceScore, svcID)
					}
				}

				// Also exercise the single-service Window path.
				svcID := fmt.Sprintf("svc-%03d", rid%serviceCount)
				w := store.Window(svcID, 32, 5*time.Second)
				// Window returning nil is legal (service may have been pruned).
				// But if it returns non-nil, the fields must be valid.
				if w != nil && w.SampleCount <= 0 {
					t.Errorf("L3-TEL-003 INVALID Window: SampleCount=%d for %q", w.SampleCount, svcID)
				}

				// Exercise ServiceIDs — returned slice must have no duplicates.
				ids := store.ServiceIDs()
				seen := make(map[string]struct{}, len(ids))
				for _, id := range ids {
					if _, dup := seen[id]; dup {
						t.Errorf("L3-TEL-003 DUPLICATE in ServiceIDs(): %q", id)
					}
					seen[id] = struct{}{}
				}
			}
		}(r)
	}

	wg.Wait()

	durationMs := time.Since(start).Milliseconds()
	passed := atomic.LoadInt64(&panics) == 0 && atomic.LoadInt64(&nilDeref) == 0

	writeL3Result(L3Record{
		TestID: "L3-TEL-003",
		Layer:  3,
		Name:   "Store concurrent Ingest/Prune/AllWindows/Window/ServiceIDs",
		Aim: fmt.Sprintf(
			"%d ingesters + %d pruners + %d readers on Store for %ds: zero races, zero panics, zero nil windows",
			ingesters, pruners, readers, durationS,
		),
		PackagesInvolved: []string{"internal/telemetry"},
		FunctionsTested: []string{
			"NewStore", "(*Store).Ingest", "(*Store).Prune",
			"(*Store).AllWindows", "(*Store).Window", "(*Store).ServiceIDs",
		},
		Threshold: L3Threshold{
			Metric:    "panics_plus_nil_windows",
			Operator:  "==",
			Value:     0,
			Unit:      "count",
			Rationale: "Concurrent map access without proper locking causes panic in Go runtime",
		},
		Result: L3ResultData{
			Status:              l3Status(passed),
			ActualValue:         float64(atomic.LoadInt64(&panics) + atomic.LoadInt64(&nilDeref)),
			ActualUnit:          "panics+nil_deref",
			OperationsCompleted: atomic.LoadInt64(&ingestsDone),
			RaceDetectorActive:  raceDetectorEnabled(),
			DurationMs:          durationMs,
			ErrorMessages: []string{fmt.Sprintf(
				"ingests=%d prunes=%d reads=%d panics=%d nil_windows=%d",
				atomic.LoadInt64(&ingestsDone),
				atomic.LoadInt64(&prunesDone),
				atomic.LoadInt64(&readsDone),
				atomic.LoadInt64(&panics),
				atomic.LoadInt64(&nilDeref),
			)},
		},
		OnExceed: "Race on Store.buffers map → 'concurrent map read and map write' panic → " +
			"Orchestrator.tick goroutine crashes → control loop halts → system goes open-loop",
		Questions: L3Questions{
			WhatWasTested: fmt.Sprintf(
				"Store(bufCap=%d maxSvc=%d staleAge=%s) under %d ingesters + %d pruners + %d readers for %ds",
				bufCap, maxSvc, staleAge, ingesters, pruners, readers, durationS,
			),
			WhyThisThreshold:    "Go's map is not concurrency-safe; unguarded concurrent access panics the runtime — not a data race, an outright crash",
			WhatHappensIfFails:  "The Orchestrator tick goroutine panics on map access → entire control loop stops → system runs open-loop indefinitely",
			HowRacesWereDetected: "Go race detector on test binary + explicit panic recovery in each goroutine",
			HowLeaksWereDetected: "N/A — not the focus of this test",
			WhatConcurrencyPattern: "Double-checked locking (RLock for read, upgrade to Lock for write) + delete-under-lock in Prune racing with Ingest",
		},
		RunAt:     l3Now(),
		GoVersion: l3GoVer(),
	})

	if !passed {
		t.Fatalf(
			"L3-TEL-003 FAILED: panics=%d nil_windows=%d\n"+
				"FIX: Store.Ingest must use double-checked locking:\n"+
				"     1. RLock → check if buf exists → RUnlock\n"+
				"     2. Lock  → check again → create if still absent → Unlock\n"+
				"     Store.Prune must hold full Lock when deleting from both maps.\n"+
				"     File: internal/telemetry/store.go",
			atomic.LoadInt64(&panics),
			atomic.LoadInt64(&nilDeref),
		)
	}

	t.Logf(
		"L3-TEL-003 PASS | ingests=%d prunes=%d reads=%d panics=0 nil=0",
		atomic.LoadInt64(&ingestsDone),
		atomic.LoadInt64(&prunesDone),
		atomic.LoadInt64(&readsDone),
	)
}

// ─────────────────────────────────────────────────────────────────────────────
// L3-TEL-004 — Store cardinality cap enforced under concurrent ingestion
//
// AIM:   When maxServices=10 and 50 goroutines try to register new services
//        simultaneously, Store.Ingest must never hold more than 10 services.
//        The cap must be enforced without data races.
//
// THRESHOLD: max_services_at_any_point <= maxSvcCap
// ON EXCEED: Unbounded cardinality → unbounded heap growth →
//            OOM kill in high-cardinality production environments.
// ─────────────────────────────────────────────────────────────────────────────
func TestL3_TEL_004_StoreCardinalityCapConcurrent(t *testing.T) {
	start := time.Now()

	const (
		bufCap     = 64
		maxSvcCap  = 10  // hard cardinality cap
		goroutines = 50  // all try to register unique services simultaneously
		iterations = 500 // each goroutine injects this many points
	)

	store := telemetry.NewStore(bufCap, maxSvcCap, 24*time.Hour) // long staleAge — no pruning

	var (
		ingestsDone   int64
		capViolations int64
		panics        int64
	)

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					atomic.AddInt64(&panics, 1)
					t.Errorf("L3-TEL-004 PANIC in goroutine %d: %v", gid, r)
				}
			}()

			for i := 0; i < iterations; i++ {
				// Each goroutine uses its own unique service ID — they all compete
				// to be admitted into the cardinality-capped store.
				store.Ingest(&telemetry.MetricPoint{
					ServiceID:   fmt.Sprintf("svc-unique-%03d", gid),
					Timestamp:   time.Now(),
					RequestRate: float64(i + 1),
					ErrorRate:   0.01,
					Latency: telemetry.LatencyStats{Mean: 50.0, P50: 45.0, P95: 80.0, P99: 100.0},
				})
				atomic.AddInt64(&ingestsDone, 1)

				// Poll cardinality after every 10 injects.
				if i%10 == 0 {
					count := len(store.ServiceIDs())
					if count > maxSvcCap {
						atomic.AddInt64(&capViolations, 1)
						t.Errorf(
							"L3-TEL-004 CAP VIOLATION: ServiceIDs count=%d > maxSvc=%d (goroutine=%d iter=%d)",
							count, maxSvcCap, gid, i,
						)
					}
				}
			}
		}(g)
	}
	wg.Wait()

	// Final check: after all goroutines finish, count must still be <= cap.
	finalCount := len(store.ServiceIDs())
	if finalCount > maxSvcCap {
		atomic.AddInt64(&capViolations, 1)
		t.Errorf("L3-TEL-004 CAP VIOLATION at end: count=%d > maxSvc=%d", finalCount, maxSvcCap)
	}

	passed := atomic.LoadInt64(&panics) == 0 && atomic.LoadInt64(&capViolations) == 0
	durationMs := time.Since(start).Milliseconds()

	writeL3Result(L3Record{
		TestID: "L3-TEL-004",
		Layer:  3,
		Name:   "Store cardinality cap enforced under concurrent ingestion",
		Aim: fmt.Sprintf(
			"%d goroutines each registering unique services against Store(maxSvc=%d): count must never exceed cap",
			goroutines, maxSvcCap,
		),
		PackagesInvolved: []string{"internal/telemetry"},
		FunctionsTested:  []string{"NewStore", "(*Store).Ingest", "(*Store).ServiceIDs"},
		Threshold: L3Threshold{
			Metric:    "cardinality_cap_violations",
			Operator:  "==",
			Value:     0,
			Unit:      "count",
			Rationale: "Exceeding maxServices allows unbounded map growth → OOM in high-cardinality environments",
		},
		Result: L3ResultData{
			Status:              l3Status(passed),
			ActualValue:         float64(finalCount),
			ActualUnit:          "final_service_count",
			OperationsCompleted: atomic.LoadInt64(&ingestsDone),
			RaceDetectorActive:  raceDetectorEnabled(),
			DurationMs:          durationMs,
			ErrorMessages: []string{fmt.Sprintf(
				"goroutines=%d iterations_each=%d total_ingests=%d violations=%d panics=%d final_count=%d max_allowed=%d",
				goroutines, iterations, atomic.LoadInt64(&ingestsDone),
				atomic.LoadInt64(&capViolations), atomic.LoadInt64(&panics),
				finalCount, maxSvcCap,
			)},
		},
		OnExceed: "More than maxServices entries in the Store → heap grows unboundedly → " +
			"Kubernetes OOM killer terminates the pod",
		Questions: L3Questions{
			WhatWasTested: fmt.Sprintf(
				"%d goroutines trying to register %d unique services against Store(maxSvc=%d) simultaneously",
				goroutines, goroutines, maxSvcCap,
			),
			WhyThisThreshold:    "Cap must be a hard guarantee; exceeding it by even one service breaks the memory budget calculation",
			WhatHappensIfFails:  "Production system with high service cardinality fills memory → OOM kill → control loop restarts → brief open-loop operation",
			HowRacesWereDetected: "Go race detector on test binary detects race on len(s.buffers) check vs map write",
			HowLeaksWereDetected: "N/A",
			WhatConcurrencyPattern: "TOCTOU (Time-of-Check-Time-of-Use) race: check len(s.buffers) then add — must be guarded by single Lock across both operations",
		},
		RunAt:     l3Now(),
		GoVersion: l3GoVer(),
	})

	if !passed {
		t.Fatalf(
			"L3-TEL-004 FAILED: violations=%d panics=%d final_count=%d (max=%d)\n"+
				"FIX: The len(s.buffers) >= s.maxSvc check and the map insertion MUST both happen\n"+
				"     inside the same Lock() region in Store.Ingest. Currently the check uses RLock\n"+
				"     and the insert uses a separate Lock — two goroutines can both pass the check\n"+
				"     simultaneously before either inserts, admitting N > maxSvc services.\n"+
				"     File: internal/telemetry/store.go",
			atomic.LoadInt64(&capViolations), atomic.LoadInt64(&panics), finalCount, maxSvcCap,
		)
	}

	t.Logf(
		"L3-TEL-004 PASS | ingests=%d final_count=%d max_allowed=%d violations=0",
		atomic.LoadInt64(&ingestsDone), finalCount, maxSvcCap,
	)
}