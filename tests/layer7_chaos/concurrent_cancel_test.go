//go:build legacy
// +build legacy


package layer7



import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/optimisation"
	"github.com/loadequilibrium/loadequilibrium/internal/streaming"
)

// 
// L7-CHAOS-004 — Concurrent cancel race: zero panics across all operations
//
// AIM:
//   10 goroutines each performing a different operation concurrently:
//     0: Hub.Broadcast (real streaming.TickPayload)
//     1: Store.Ingest
//     2: Store.AllWindows + Store.Prune
//     3: Act.Dispatch
//     4: Hub.GetLastPayload + HTTP /health
//   Context cancelled after 300ms while all goroutines are active.
//   Zero panics required.
//
// THRESHOLD: panics == 0
// ON EXCEED: Race condition during concurrent cancel causes panic → crash.
// 


func TestL7_CHAOS_004_ConcurrentCancelRace_ORIG(t *testing.T) {
	start := time.Now()

	const (
		tickMs        = 100
		goroutines    = 10
		cancelAfterMs = 300
	)

	ts := newTestSystem(tickMs)

	// Seed telemetry.
	for i := 0; i < 5; i++ {
		ts.InjectTelemetry(fmt.Sprintf("race-svc-%02d", i), 100.0, 30.0)
	}
	ts.WaitForTicks(3, 5*time.Second)

	var panics int64
	done := make(chan struct{})

	for g := 0; g < goroutines; g++ {
		g := g
		go func() {
			defer func() {
				if r := recover(); r != nil {
					atomic.AddInt64(&panics, 1)
					t.Errorf("L7-CHAOS-004 PANIC goroutine %d: %v", g, r)
				}
			}()
			seq := uint64(g * 100_000)
			for {
				select {
				case <-done:
					return
				default:
				}

				switch g % 5 {
				case 0:
					// Real Hub.Broadcast with real TickPayload.
					ts.Hub.Broadcast(&streaming.TickPayload{
						Type:         streaming.MsgTick,
						TickHealthMs: float64(g) * 0.1,
						RuntimeMetrics: streaming.RuntimeMetrics{
							AvgPruneMs: float64(g),
						},
					})
				case 1:
					ts.InjectTelemetry(
						fmt.Sprintf("race-svc-%02d", g%5),
						100.0+float64(g), 30.0,
					)
				case 2:
					_ = ts.Store.AllWindows(32, 30*time.Second)
					_ = ts.Store.Prune(time.Now())
				case 3:
					seq++
					ts.Act.Dispatch(seq, map[string]optimisation.ControlDirective{
						fmt.Sprintf("race-svc-%02d", g%5): {
							ServiceID:   fmt.Sprintf("race-svc-%02d", g%5),
							ScaleFactor: 1.0,
							Active:      true,
						},
					})
				case 4:
					_ = ts.Hub.GetLastPayload()
					_, _ = httpGet(ts.HTTPSrv.URL + "/health")
				}
			}
		}()
	}

	// Cancel after 300ms.
	time.Sleep(time.Duration(cancelAfterMs) * time.Millisecond)
	close(done)
	ts.Shutdown()

	// Let goroutines exit.
	time.Sleep(500 * time.Millisecond)

	p := atomic.LoadInt64(&panics)
	passed := p == 0

	t.Logf("L7-CHAOS-004 panics=%d", p)

	writeL7Result(L7Record{
		TestID: "L7-CHAOS-004", Layer: 7,
		Name: fmt.Sprintf("%d concurrent goroutines (Broadcast/Ingest/Prune/Dispatch/HTTP) + cancel", goroutines),
		Aim:  "Concurrent cancel after 300ms while all ops active: zero panics",
		PackagesInvolved: []string{
			"internal/runtime", "internal/streaming", "internal/telemetry",
			"internal/actuator", "internal/api",
		},
		Threshold: L7Threshold{
			Metric: "panics", Operator: "==", Value: 0, Unit: "count",
			Rationale: "No concurrent cancel pattern must cause panic",
		},
		Result: L7ResultData{
			Status:         l7Status(passed),
			ActualValue:    float64(p),
			ActualUnit:     "panics",
			ChaosInjected:  fmt.Sprintf("%d goroutines concurrent with cancel after %dms", goroutines, cancelAfterMs),
			PanicsDetected: p,
			DurationMs:     time.Since(start).Milliseconds(),
			ErrorMessages:  []string{fmt.Sprintf("panics=%d", p)},
		},
		OnExceed: "Race condition during shutdown causes panic → process crash → Kubernetes restart loop",
		Questions: L7Questions{
			WhatChaosWasInjected: fmt.Sprintf("%d goroutines with mixed ops cancelled after %dms", goroutines, cancelAfterMs),
			WhyThisThreshold:     "Zero panics: send-on-closed-channel or nil dereference during cancel is a bug",
			WhatHappensIfFails:   "Shutdown races cause panics → unreliable graceful termination",
			HowChaosWasInjected:  "10 goroutines tight-looping; cancel via done channel + ts.Shutdown() simultaneously",
			HowRecoveryVerified:  "N/A — concurrent safety test",
			ProductionEquivalent: "Dashboard clients + active control + SIGTERM simultaneously",
		},
		RunAt: l7Now(), GoVersion: l7GoVer(),
	})

	if !passed {
		t.Fatalf(
			"L7-CHAOS-004 FAILED: panics=%d\n"+
				"Run with: go test ./tests/layer7_chaos/ -run TestL7_CHAOS_004 -race -v\n"+
				"The race detector will identify which lines have the concurrent access issue.",
			p,
		)
	}
	t.Logf("L7-CHAOS-004 PASS | panics=0 across %d goroutines", goroutines)
}