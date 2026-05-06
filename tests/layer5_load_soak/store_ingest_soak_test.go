package layer5


// RUN: go test ./tests/layer5_load_soak/ -run TestL5_TEL -count=1 -timeout=3600s -v

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/streaming"
	"github.com/loadequilibrium/loadequilibrium/internal/telemetry"
)


// L5-TEL-001 — Store.Ingest sustained throughput: 10,000 events/s for 30 minutes
//
// AIM:   Ingest 10,000 MetricPoints/s across 10 rotating services for 30 min.
//        - All 10 services remain registered (cardinality not exceeded)
//        - Heap growth factor <= 1.5x baseline (no memory leak)
//        - AllWindows() p99 latency < 5ms (concurrent reads stay fast)
//        - Ring buffer SampleCount stays <= bufCap per service
//
// THRESHOLD: heap_growth_factor <= 1.5
// ON EXCEED: Store retains MetricPoints beyond ring capacity → OOM kill.

func TestL5_TEL_001_StoreIngestSoak(t *testing.T) {
	if testing.Short() {
		t.Skip("L5-TEL-001: skipped in short mode — requires 30 minutes")
	}

	start := time.Now()

	const (
		targetEventsPerSec = 10_000
		soakDuration       = 30 * time.Minute
		serviceCount       = 10
		bufCap             = 1000
		maxSvc             = 20
		sampleIntervalS    = 30
		windowReadWorkers  = 5
		p99WindowMs        = 5.0
	)

	store := telemetry.NewStore(bufCap, maxSvc, 5*time.Minute)

	heapBaseline := heapBytes()
	t.Logf("L5-TEL-001 baseline heap: %dKB", heapBaseline/1024)

	var (
		ingestsTotal     int64
		windowReadsTotal int64
		windowReadLatMs  []float64
		windowReadMu     sync.Mutex
		pruneCount       int64
	)

	type heapSample struct {
		elapsedS float64
		heapKB   uint64
	}
	var heapSamples []heapSample

	ctx, ingestCancel := testContextWithTimeout(soakDuration)
	defer ingestCancel()

	var wg sync.WaitGroup

	// ── Ingest goroutine: 10 events per 1ms tick = 10,000/s 
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		seq := int64(0)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				for batch := 0; batch < 10; batch++ {
					seq++
					svcID := fmt.Sprintf("svc-%02d", seq%int64(serviceCount))
					store.Ingest(&telemetry.MetricPoint{
						ServiceID:   svcID,
						Timestamp:   time.Now(),
						RequestRate: float64(seq%1000) + 100.0,
						ErrorRate:   0.005,
						Latency: telemetry.LatencyStats{
							Mean: 50.0 + float64(seq%20),
							P50:  45.0,
							P95:  80.0,
							P99:  100.0,
						},
						ActiveConns: int64(seq % 50),
						QueueDepth:  int64(seq % 20),
					})
					atomic.AddInt64(&ingestsTotal, 1)
				}
			}
		}
	}()

	// ── Concurrent AllWindows readers 
	for w := 0; w < windowReadWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				t0 := time.Now()
				windows := store.AllWindows(100, 30*time.Second)
				latMs := float64(time.Since(t0).Microseconds()) / 1000.0
				_ = windows
				atomic.AddInt64(&windowReadsTotal, 1)
				windowReadMu.Lock()
				windowReadLatMs = append(windowReadLatMs, latMs)
				windowReadMu.Unlock()
				time.Sleep(10 * time.Millisecond)
			}
		}()
	}

	// ── Periodic pruner 
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pruned := store.Prune(time.Now())
				atomic.AddInt64(&pruneCount, int64(len(pruned)))
			}
		}
	}()

	// ── Heap sampler 
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(time.Duration(sampleIntervalS) * time.Second)
		defer ticker.Stop()
		elapsed := 0.0
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				elapsed += float64(sampleIntervalS)
				runtime.GC()
				var ms runtime.MemStats
				runtime.ReadMemStats(&ms)
				heapSamples = append(heapSamples, heapSample{
					elapsedS: elapsed,
					heapKB:   ms.HeapInuse / 1024,
				})
				t.Logf("L5-TEL-001 t=%.0fs: heap=%dKB services=%d ingests=%d",
					elapsed, ms.HeapInuse/1024,
					len(store.ServiceIDs()),
					atomic.LoadInt64(&ingestsTotal),
				)
			}
		}
	}()

	wg.Wait()

	// ── Final measurements 
	heapFinal := heapBytes()
	heapGrowth := float64(heapFinal) / float64(heapBaseline+1)

	windowReadMu.Lock()
	lats := make([]float64, len(windowReadLatMs))
	copy(lats, windowReadLatMs)
	windowReadMu.Unlock()

	windowPct := computePercentiles(lats)
	finalServiceCount := len(store.ServiceIDs())
	finalIngests := atomic.LoadInt64(&ingestsTotal)

	var overflowedServices []string
	for _, svcID := range store.ServiceIDs() {
		w := store.Window(svcID, bufCap+1, time.Minute)
		if w != nil && w.SampleCount > bufCap {
			overflowedServices = append(overflowedServices,
				fmt.Sprintf("%s=%d", svcID, w.SampleCount))
		}
	}

	passed := heapGrowth <= 1.5 &&
		windowPct.P99Ms < p99WindowMs &&
		finalServiceCount == serviceCount &&
		len(overflowedServices) == 0

	durationMs := time.Since(start).Milliseconds()

	var errMsgs []string
	errMsgs = append(errMsgs, fmt.Sprintf(
		"ingests=%d heap_baseline=%dKB heap_final=%dKB growth=%.4fx services=%d(expected %d)",
		finalIngests, heapBaseline/1024, heapFinal/1024, heapGrowth, finalServiceCount, serviceCount,
	))
	errMsgs = append(errMsgs, fmt.Sprintf(
		"window_reads=%d p50=%.2fms p99=%.2fms(threshold=%.0fms)",
		atomic.LoadInt64(&windowReadsTotal), windowPct.P50Ms, windowPct.P99Ms, p99WindowMs,
	))
	if len(overflowedServices) > 0 {
		errMsgs = append(errMsgs, fmt.Sprintf("ring_overflow: %v", overflowedServices))
	}
	for _, s := range heapSamples {
		errMsgs = append(errMsgs, fmt.Sprintf("t=%.0fs heap=%dKB", s.elapsedS, s.heapKB))
	}

	writeL5Result(L5Record{
		TestID: "L5-TEL-001",
		Layer:  5,
		Name:   fmt.Sprintf("Store.Ingest soak: %d events/s for %s", targetEventsPerSec, soakDuration),
		Aim: fmt.Sprintf(
			"%d events/s for %s: heap growth <=1.5x, AllWindows p99 <%.0fms, ring bounded at %d",
			targetEventsPerSec, soakDuration, p99WindowMs, bufCap,
		),
		PackagesInvolved: []string{"internal/telemetry"},
		FunctionsTested: []string{
			"telemetry.NewStore", "(*Store).Ingest", "(*Store).AllWindows",
			"(*Store).Window", "(*Store).Prune", "(*Store).ServiceIDs",
		},
		Threshold: L5Threshold{
			Metric:    "heap_growth_factor",
			Operator:  "<=",
			Value:     1.5,
			Unit:      "ratio",
			Rationale: "Fixed-size ring buffers must not accumulate memory beyond capacity",
		},
		Result: L5ResultData{
			Status:        l5Status(passed),
			ActualValue:   heapGrowth,
			ActualUnit:    "heap_growth_factor",
			SampleCount:   int(finalIngests),
			Percentiles:   &windowPct,
			ThroughputRps: float64(finalIngests) / (float64(durationMs) / 1000.0),
			DurationMs:    durationMs,
			ErrorMessages: errMsgs,
		},
		OnExceed: "Store retains MetricPoints beyond ring buffer capacity → heap grows with ingestion volume → OOM kill",
		Questions: L5Questions{
			WhatWasTested:       fmt.Sprintf("Store.Ingest at %d events/s for %s, %d concurrent AllWindows readers", targetEventsPerSec, soakDuration, windowReadWorkers),
			WhyThisThreshold:    "1.5x: ring buffers fixed-size, heap stabilises after warm-up; >1.5x means entries retained beyond capacity",
			WhatHappensIfFails:  "Memory grows proportional to ingestion volume → OOM kill in production",
			HowLoadWasGenerated: fmt.Sprintf("1ms ticker × 10 events/tick = %d events/s across %d services", targetEventsPerSec, serviceCount),
			HowMetricsMeasured:  "runtime.MemStats.HeapInuse sampled every 30s with forced GC",
			WorstCaseDescription: fmt.Sprintf("heap_growth=%.4fx window_p99=%.2fms", heapGrowth, windowPct.P99Ms),
		},
		RunAt: l5Now(), GoVersion: l5GoVer(),
	})

	if !passed {
		t.Fatalf(
			"L5-TEL-001 FAILED: heap_growth=%.4fx window_p99=%.2fms services=%d overflow=%v\n"+
				"FIX heap: RingBuffer.Push must overwrite r.buf[r.head], not append. File: internal/telemetry/ringbuffer.go\n"+
				"FIX window: check RLock contention in Store.Window. File: internal/telemetry/store.go",
			heapGrowth, windowPct.P99Ms, finalServiceCount, overflowedServices,
		)
	}
	t.Logf("L5-TEL-001 PASS | ingests=%d heap_growth=%.4fx window_p99=%.2fms",
		finalIngests, heapGrowth, windowPct.P99Ms)
}


// L5-TEL-002 — Hub.GetLastPayload latency under 100 concurrent readers + 10Hz broadcast
//
// AIM:   100 goroutines tight-loop GetLastPayload for 60 seconds while
//        Broadcast runs at 10Hz. Read p99 must stay < 1ms.
//        This simulates 100 dashboard clients polling /api/v1/snapshot.
//
// THRESHOLD: read_p99_ms < 1.0
// ON EXCEED: /snapshot endpoint blocks → dashboard first-load hangs.

func TestL5_TEL_002_SnapshotReadLatencyUnderConcurrentBroadcast(t *testing.T) {
	start := time.Now()

	const (
		readers       = 100
		broadcastHz   = 10
		testDurationS = 60
		p99Threshold  = 1.0
	)

	hub := streaming.NewHub()
	hub.SetMaxClients(0)

	hub.Broadcast(&streaming.TickPayload{
		Type:         streaming.MsgTick,
		TickHealthMs: 1.0,
	})

	var (
		readLatenciesMs []float64
		latMu           sync.Mutex
		readsDone       int64
		nilReads        int64
		panics          int64
	)

	ctx, cancel := testContextWithTimeout(time.Duration(testDurationS) * time.Second)
	defer cancel()

	var wg sync.WaitGroup

	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func(rid int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					atomic.AddInt64(&panics, 1)
					t.Errorf("L5-TEL-002 PANIC in reader %d: %v", rid, r)
				}
			}()
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				t0 := time.Now()
				p := hub.GetLastPayload()
				latMs := float64(time.Since(t0).Microseconds()) / 1000.0
				atomic.AddInt64(&readsDone, 1)
				if p == nil {
					atomic.AddInt64(&nilReads, 1)
				}
				latMu.Lock()
				readLatenciesMs = append(readLatenciesMs, latMs)
				latMu.Unlock()
			}
		}(r)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(time.Duration(1000/broadcastHz) * time.Millisecond)
		defer ticker.Stop()
		seq := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				seq++
				hub.Broadcast(&streaming.TickPayload{
					Type:         streaming.MsgTick,
					TickHealthMs: float64(seq) * 0.1,
				})
			}
		}
	}()

	wg.Wait()

	latMu.Lock()
	lats := make([]float64, len(readLatenciesMs))
	copy(lats, readLatenciesMs)
	latMu.Unlock()

	pct := computePercentiles(lats)
	passed := pct.P99Ms < p99Threshold && panics == 0
	durationMs := time.Since(start).Milliseconds()

	writeL5Result(L5Record{
		TestID: "L5-TEL-002",
		Layer:  5,
		Name:   fmt.Sprintf("Hub.GetLastPayload: %d readers + %dHz broadcast for %ds", readers, broadcastHz, testDurationS),
		Aim:    fmt.Sprintf("%d concurrent GetLastPayload callers while Broadcast at %dHz: p99 < %.0fms", readers, broadcastHz, p99Threshold),
		PackagesInvolved: []string{"internal/streaming"},
		FunctionsTested:  []string{"(*Hub).GetLastPayload", "(*Hub).Broadcast"},
		Threshold: L5Threshold{
			Metric: "read_p99_ms", Operator: "<", Value: p99Threshold, Unit: "ms",
			Rationale: "/api/v1/snapshot calls GetLastPayload — must be fast for dashboard first-load",
		},
		Result: L5ResultData{
			Status:        l5Status(passed),
			ActualValue:   pct.P99Ms,
			ActualUnit:    "p99_ms",
			SampleCount:   int(readsDone),
			Percentiles:   &pct,
			ErrorCount:    panics,
			DurationMs:    durationMs,
			ErrorMessages: []string{fmt.Sprintf(
				"reads=%d nil=%d panics=%d p50=%.3fms p99=%.3fms p100=%.3fms",
				readsDone, nilReads, panics, pct.P50Ms, pct.P99Ms, pct.P100Ms,
			)},
		},
		OnExceed: "GetLastPayload blocks → /snapshot HTTP responses pile up → dashboard first-load timeout",
		Questions: L5Questions{
			WhatWasTested:       fmt.Sprintf("%d goroutines tight-looping GetLastPayload for %ds while 1 broadcaster at %dHz", readers, testDurationS, broadcastHz),
			WhyThisThreshold:    "1ms p99: RLock on a struct field is sub-microsecond; 1ms is extremely generous",
			WhatHappensIfFails:  "REST /snapshot endpoint latency > 1ms → dashboard HTTP timeout on initial load",
			HowLoadWasGenerated: fmt.Sprintf("%d readers in tight loop + %dHz broadcaster", readers, broadcastHz),
			HowMetricsMeasured:  "time.Since(t0) per GetLastPayload call; p99 from sorted slice",
			WorstCaseDescription: fmt.Sprintf("p99=%.3fms reads=%d", pct.P99Ms, readsDone),
		},
		RunAt: l5Now(), GoVersion: l5GoVer(),
	})

	if !passed {
		t.Fatalf(
			"L5-TEL-002 FAILED: p99=%.3fms (threshold=%.0fms) panics=%d\n"+
				"FIX: json.Marshal(p) in Broadcast must happen OUTSIDE h.mu.Lock().\n"+
				"     Move marshal before Lock, assign data, then Lock → assign lastPayload → Unlock.\n"+
				"     File: internal/streaming/hub.go",
			pct.P99Ms, p99Threshold, panics,
		)
	}
	t.Logf("L5-TEL-002 PASS | reads=%d p50=%.3fms p99=%.3fms", readsDone, pct.P50Ms, pct.P99Ms)
}