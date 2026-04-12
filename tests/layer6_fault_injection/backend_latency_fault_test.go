package layer6

// FILE: tests/layer6_fault_injection/L6_NET_001_backend_latency_fault_test.go
//
// Tests:   L6-NET-001, L6-NET-002, L6-NET-003, L6-NET-004
// Real types used:
//   actuator.NewCoalescingActuator(feedbackBuf int, backend Backend) *CoalescingActuator
//   (*CoalescingActuator).Dispatch(tickIndex uint64, dirs map[string]optimisation.ControlDirective)
//   (*CoalescingActuator).Feedback() <-chan ActuationResult
//   (*CoalescingActuator).Close(ctx context.Context) error
//   actuator.ActuationResult{TickIndex, ServiceID, Success, Latency, Error}
//   actuator.NewRouterBackend(defaultBackend Backend) *RouterBackend
//   (*RouterBackend).AddRoute(serviceID string, backend Backend)
//   backends.NewHTTPBackend(endpoint string) *HTTPBackend
//   optimisation.ControlDirective{ServiceID, ScaleFactor, Active, ...}
//
// FAULT INJECTION: httptest.Server with controllable handler behaviour.
// No Toxiproxy binary required — fully in-process.

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/actuator"
	"github.com/loadequilibrium/loadequilibrium/internal/actuator/backends"
	"github.com/loadequilibrium/loadequilibrium/internal/optimisation"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func closeAct(a *actuator.CoalescingActuator) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = a.Close(ctx)
}

func ngoro() int { return runtime.NumGoroutine() }

func strhas(s, sub string) bool { return strings.Contains(s, sub) }

func l6max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// collectFeedback drains up to n results from ch within timeout.
func collectFeedback(ch <-chan actuator.ActuationResult, n int, timeout time.Duration) (ok, fail int64) {
	deadline := time.After(timeout)
	for i := 0; i < n; i++ {
		select {
		case r := <-ch:
			if r.Success {
				ok++
			} else {
				fail++
			}
		case <-deadline:
			return
		}
	}
	return
}

// ─────────────────────────────────────────────────────────────────────────────
// L6-NET-001 — +500ms backend latency: Dispatch non-blocking, coalescing works
//
// AIM:
//   Phase 1: 20 Dispatch() calls while backend takes 500ms → each Dispatch < 5ms.
//   Phase 2: Fewer than 20 HTTP requests made (coalescing_ratio >= 2.0).
//   Phase 3: After latency removed, recovery request completes < 100ms.
//
// THRESHOLD: max_dispatch_ms < 5, coalescing >= 2.0, recovery_ms < 100
// ON EXCEED: Control loop blocks on actuator → tick overruns cascade.
// ─────────────────────────────────────────────────────────────────────────────
func TestL6_NET_001_BackendLatencyCoalescingNonBlocking(t *testing.T) {
	start := time.Now()

	const (
		backendLatencyMs = 500
		dispatches       = 20
		svcID            = "svc-lat"
		feedbackBuf      = 128
		minCoalesce      = 2.0
	)

	var (
		reqs    int64
		latency int32 // 1=on 0=off
	)
	atomic.StoreInt32(&latency, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&reqs, 1)
		if atomic.LoadInt32(&latency) == 1 {
			time.Sleep(time.Duration(backendLatencyMs) * time.Millisecond)
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	act := actuator.NewCoalescingActuator(feedbackBuf, backends.NewHTTPBackend(srv.URL))
	defer closeAct(act)

	// Phase 1: measure Dispatch latency.
	var maxDispMs float64
	for i := 0; i < dispatches; i++ {
		t0 := time.Now()
		act.Dispatch(uint64(i+1), map[string]optimisation.ControlDirective{
			svcID: {ServiceID: svcID, ScaleFactor: 1.0 + float64(i)*0.01, Active: true},
		})
		ms := float64(time.Since(t0).Microseconds()) / 1000.0
		if ms > maxDispMs {
			maxDispMs = ms
		}
	}

	// Phase 2: wait and count HTTP requests.
	time.Sleep(time.Duration(backendLatencyMs+300) * time.Millisecond)
	httpReqs := atomic.LoadInt64(&reqs)
	ratio := float64(dispatches) / float64(l6max(httpReqs, 1))

	t.Logf("L6-NET-001 dispatched=%d http=%d ratio=%.2f max_disp=%.3fms", dispatches, httpReqs, ratio, maxDispMs)

	// Phase 3: remove latency, verify fast recovery.
	atomic.StoreInt32(&latency, 0)
	rStart := time.Now()
	act.Dispatch(999, map[string]optimisation.ControlDirective{
		svcID: {ServiceID: svcID, ScaleFactor: 1.0, Active: true},
	})

	var recMs int64
	var recOK bool
	select {
	case res := <-act.Feedback():
		if res.TickIndex == 999 {
			recMs = time.Since(rStart).Milliseconds()
			recOK = res.Success
		}
	case <-time.After(10 * time.Second):
		t.Log("L6-NET-001: recovery timeout")
	}

	passed := maxDispMs < 5.0 && ratio >= minCoalesce && recOK && recMs < 100

	writeL6Result(L6Record{
		TestID: "L6-NET-001", Layer: 6,
		Name: fmt.Sprintf("Backend +%dms latency: non-blocking Dispatch + coalescing + recovery", backendLatencyMs),
		Aim:  fmt.Sprintf("Dispatch<5ms; coalescing>=%.1f:1; recovery<100ms", minCoalesce),
		PackagesInvolved: []string{"internal/actuator", "internal/actuator/backends"},
		FunctionsTested:  []string{"CoalescingActuator.Dispatch", "CoalescingActuator.Feedback", "HTTPBackend.Execute"},
		Threshold: L6Threshold{
			Metric: "max_dispatch_latency_ms", Operator: "<", Value: 5, Unit: "ms",
			Rationale: "Dispatch must never block the orchestrator tick loop",
		},
		Result: L6ResultData{
			Status: l6Status(passed), ActualValue: maxDispMs, ActualUnit: "max_dispatch_ms",
			FaultInjected:   fmt.Sprintf("+%dms latency via httptest.Server sleep", backendLatencyMs),
			TimeToRecoverMs: recMs, CommandsSent: int64(dispatches) + 1,
			CommandsSucceeded: func() int64 { if recOK { return 1 }; return 0 }(),
			CommandsCoalesced: int64(dispatches) - httpReqs,
			DurationMs:        time.Since(start).Milliseconds(),
			ErrorMessages: []string{fmt.Sprintf(
				"max_disp=%.3fms dispatched=%d http=%d ratio=%.2f rec_ms=%d rec_ok=%v",
				maxDispMs, dispatches, httpReqs, ratio, recMs, recOK,
			)},
		},
		OnExceed: "CoalescingActuator blocks orchestrator tick → tick deadline exceeded → safety mode",
		Questions: L6Questions{
			WhatFaultWasInjected: fmt.Sprintf("+%dms sleep in httptest.Server handler", backendLatencyMs),
			WhyThisThreshold:     "Dispatch<5ms: non-blocking is the core contract of CoalescingActuator",
			WhatHappensIfFails:   "Tick loop blocked → consecutive overruns → safety level escalation",
			HowFaultWasInjected:  "atomic int32 controlling time.Sleep in handler; cleared for recovery phase",
			HowRecoveryVerified:  "Feedback.ActuationResult{Success:true, TickIndex:999} after fault cleared",
			WhatDegradedMeans:    "pending map coalesces by serviceID — latest directive wins; caller never blocks",
		},
		RunAt: l6Now(), GoVersion: l6GoVer(),
	})

	if !passed {
		t.Fatalf(
			"L6-NET-001 FAILED: max_disp=%.3fms ratio=%.2f rec_ms=%d rec_ok=%v\n"+
				"FIX: CoalescingActuator.Dispatch must only lock/unlock to update pending map — no Execute inline.\n"+
				"File: internal/actuator/actuator.go",
			maxDispMs, ratio, recMs, recOK,
		)
	}
	t.Logf("L6-NET-001 PASS | max_disp=%.3fms coalescing=%.2fx rec=%dms", maxDispMs, ratio, recMs)
}

// ─────────────────────────────────────────────────────────────────────────────
// L6-NET-002 — Backend disconnection: graceful failure, no panic
//
// AIM:   Close server before any request → all connections refused.
//        Panics==0, every dispatch delivers ActuationResult{Success:false},
//        goroutine_leak<=3.
//
// THRESHOLD: panics==0, failed==serviceCount
// ─────────────────────────────────────────────────────────────────────────────
func TestL6_NET_002_BackendDisconnectionGraceful(t *testing.T) {
	start := time.Now()

	const (
		svcCount    = 10
		feedbackBuf = 64
	)

	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	deadURL := dead.URL
	dead.Close() // immediately closed

	act := actuator.NewCoalescingActuator(feedbackBuf, backends.NewHTTPBackend(deadURL))
	defer closeAct(act)

	goroBefore := ngoro()

	var panics int64
	func() {
		defer func() {
			if r := recover(); r != nil {
				atomic.AddInt64(&panics, 1)
				t.Errorf("L6-NET-002 PANIC: %v", r)
			}
		}()
		dirs := make(map[string]optimisation.ControlDirective, svcCount)
		for i := 0; i < svcCount; i++ {
			id := fmt.Sprintf("svc-%02d", i)
			dirs[id] = optimisation.ControlDirective{ServiceID: id, ScaleFactor: 1.0, Active: true}
		}
		act.Dispatch(1, dirs)
	}()

	_, failed := collectFeedback(act.Feedback(), svcCount, 15*time.Second)

	time.Sleep(100 * time.Millisecond)
	goroAfter := ngoro()
	leaked := goroAfter - goroBefore
	if leaked < 0 {
		leaked = 0
	}

	t.Logf("L6-NET-002 panics=%d failed=%d leaked=%d", panics, failed, leaked)

	passed := panics == 0 && failed == int64(svcCount) && leaked <= 3

	writeL6Result(L6Record{
		TestID: "L6-NET-002", Layer: 6,
		Name: "Backend disconnection: graceful failure propagation",
		Aim:  fmt.Sprintf("All %d services get ActuationResult{Success:false}, 0 panics, goroutine_leak<=3", svcCount),
		PackagesInvolved: []string{"internal/actuator", "internal/actuator/backends"},
		FunctionsTested:  []string{"CoalescingActuator.Dispatch (closed server)", "CoalescingActuator.Feedback"},
		Threshold: L6Threshold{
			Metric: "panics", Operator: "==", Value: 0, Unit: "count",
			Rationale: "Connection error must be caught as error return, never re-panicked",
		},
		Result: L6ResultData{
			Status: l6Status(passed), ActualValue: float64(panics), ActualUnit: "panics",
			FaultInjected: "httptest.Server.Close() before any request",
			CommandsSent:  int64(svcCount), CommandsFailed: failed, Panics: panics,
			DurationMs:    time.Since(start).Milliseconds(),
			ErrorMessages: []string{fmt.Sprintf("panics=%d failed=%d(expected %d) leaked=%d", panics, failed, svcCount, leaked)},
		},
		OnExceed: "Actuator goroutine panics on connection error → control loop goroutine crashes → open-loop",
		Questions: L6Questions{
			WhatFaultWasInjected:  "httptest.NewServer + server.Close() before any Dispatch",
			WhyThisThreshold:      "Zero panics: network errors are expected in production",
			WhatHappensIfFails:    "CoalescingActuator goroutine panics → feedback channel never drained → orchestrator blocked",
			HowFaultWasInjected:   "server closed before HTTPBackend.Execute makes first TCP connection",
			HowRecoveryVerified:   "Not tested here — see L6-NET-001 for recovery",
			WhatDegradedMeans:     "All ActuationResult.Success=false with Error; orchestrator logs and continues",
		},
		RunAt: l6Now(), GoVersion: l6GoVer(),
	})

	if !passed {
		t.Fatalf(
			"L6-NET-002 FAILED: panics=%d failed=%d(expected %d) leaked=%d\n"+
				"FIX: processPending must catch err from Execute and assign to ActuationResult.Error.\n"+
				"File: internal/actuator/actuator.go",
			panics, failed, svcCount, leaked,
		)
	}
	t.Logf("L6-NET-002 PASS | panics=0 failed=%d leaked=%d", failed, leaked)
}

// ─────────────────────────────────────────────────────────────────────────────
// L6-NET-003 — RouterBackend partial failure isolation
//
// AIM:   svc-a → 5xx backend; svc-b → 200 backend.
//        svc-b success rate == 1.0; svc-a fail rate == 1.0.
//
// THRESHOLD: svc_b_success_rate == 1.0
// ─────────────────────────────────────────────────────────────────────────────
func TestL6_NET_003_RouterPartialFailureIsolation(t *testing.T) {
	start := time.Now()

	const rounds = 5

	var bHits, aHits int64

	goodSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&bHits, 1)
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer goodSrv.Close()

	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&aHits, 1)
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer badSrv.Close()

	router := actuator.NewRouterBackend(backends.NewHTTPBackend(goodSrv.URL))
	router.AddRoute("svc-a", backends.NewHTTPBackend(badSrv.URL))
	router.AddRoute("svc-b", backends.NewHTTPBackend(goodSrv.URL))

	act := actuator.NewCoalescingActuator(64, router)
	defer closeAct(act)

	for i := 0; i < rounds; i++ {
		act.Dispatch(uint64(i+1), map[string]optimisation.ControlDirective{
			"svc-a": {ServiceID: "svc-a", ScaleFactor: 1.0, Active: true},
			"svc-b": {ServiceID: "svc-b", ScaleFactor: 1.0, Active: true},
		})
		time.Sleep(60 * time.Millisecond)
	}

	var aOK, aFail, bOK, bFail int64
	deadline := time.After(30 * time.Second)
	for i := 0; i < rounds*2; i++ {
		select {
		case res := <-act.Feedback():
			switch res.ServiceID {
			case "svc-a":
				if res.Success {
					aOK++
				} else {
					aFail++
				}
			case "svc-b":
				if res.Success {
					bOK++
				} else {
					bFail++
				}
			}
		case <-deadline:
			t.Logf("L6-NET-003: timeout at %d results", i)
			goto done3
		}
	}
done3:

	bRate := float64(bOK) / float64(l6max(bOK+bFail, 1))
	aFailRate := float64(aFail) / float64(l6max(aOK+aFail, 1))
	passed := bRate == 1.0 && aFailRate == 1.0

	t.Logf("L6-NET-003 svc-b_rate=%.3f svc-a_fail_rate=%.3f", bRate, aFailRate)

	writeL6Result(L6Record{
		TestID: "L6-NET-003", Layer: 6,
		Name: "RouterBackend partial failure isolation",
		Aim:  "svc-a (5xx) fails 100%; svc-b (200) succeeds 100%; zero cross-contamination",
		PackagesInvolved: []string{"internal/actuator", "internal/actuator/backends"},
		FunctionsTested:  []string{"actuator.NewRouterBackend", "(*RouterBackend).AddRoute", "(*RouterBackend).Execute"},
		Threshold: L6Threshold{
			Metric: "svc_b_success_rate", Operator: "==", Value: 1.0, Unit: "ratio",
			Rationale: "A broken route for one service must not affect unrelated services",
		},
		Result: L6ResultData{
			Status: l6Status(passed), ActualValue: bRate, ActualUnit: "svc_b_success_rate",
			FaultInjected: "svc-a → HTTP 500; svc-b → HTTP 200",
			CommandsSent: int64(rounds * 2), CommandsSucceeded: bOK, CommandsFailed: aFail,
			DurationMs:   time.Since(start).Milliseconds(),
			ErrorMessages: []string{fmt.Sprintf(
				"svc-a: ok=%d fail=%d | svc-b: ok=%d fail=%d | b_rate=%.3f a_fail=%.3f",
				aOK, aFail, bOK, bFail, bRate, aFailRate,
			)},
		},
		OnExceed: "One broken route contaminates all routes — cascading failure instead of isolation",
		Questions: L6Questions{
			WhatFaultWasInjected:  "svc-a AddRoute'd to HTTP 500 server; svc-b AddRoute'd to HTTP 200 server",
			WhyThisThreshold:      "svc-b must be 100% unaffected — route isolation is the contract of RouterBackend",
			WhatHappensIfFails:    "A single degraded service's backend degrades control for ALL services",
			HowFaultWasInjected:   "router.AddRoute with independent broken and working backends",
			HowRecoveryVerified:   "N/A — steady-state isolation test",
			WhatDegradedMeans:     "svc-a commands fail with error; svc-b commands succeed normally",
		},
		RunAt: l6Now(), GoVersion: l6GoVer(),
	})

	if !passed {
		t.Fatalf(
			"L6-NET-003 FAILED: svc-b_rate=%.3f(need 1.0) svc-a_fail=%.3f(need 1.0)\n"+
				"svc-a: ok=%d fail=%d | svc-b: ok=%d fail=%d\n"+
				"FIX: RouterBackend.Execute calls route backends independently — errors must not propagate across routes.\n"+
				"File: internal/actuator/router.go",
			bRate, aFailRate, aOK, aFail, bOK, bFail,
		)
	}
	t.Logf("L6-NET-003 PASS | svc-b=1.0 svc-a_fail=1.0")
}

// ─────────────────────────────────────────────────────────────────────────────
// L6-NET-004 — Backend HTTP 503: error message contains status code, no panic
// ─────────────────────────────────────────────────────────────────────────────
func TestL6_NET_004_Backend5xxErrorFeedback(t *testing.T) {
	start := time.Now()

	errorSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer errorSrv.Close()

	act := actuator.NewCoalescingActuator(32, backends.NewHTTPBackend(errorSrv.URL))
	defer closeAct(act)

	var panics int64
	func() {
		defer func() {
			if r := recover(); r != nil {
				atomic.AddInt64(&panics, 1)
				t.Errorf("L6-NET-004 PANIC: %v", r)
			}
		}()
		act.Dispatch(1, map[string]optimisation.ControlDirective{
			"svc-5xx": {ServiceID: "svc-5xx", ScaleFactor: 1.5, Active: true},
		})
	}()

	var res actuator.ActuationResult
	select {
	case res = <-act.Feedback():
	case <-time.After(10 * time.Second):
		t.Fatal("L6-NET-004: timeout")
	}

	has503 := res.Error != nil && strhas(res.Error.Error(), "503")
	passed := panics == 0 && !res.Success && res.Error != nil && has503

	t.Logf("L6-NET-004 panics=%d success=%v err=%v has_503=%v", panics, res.Success, res.Error, has503)

	writeL6Result(L6Record{
		TestID: "L6-NET-004", Layer: 6,
		Name: "Backend HTTP 503: ActuationResult.Error contains status code",
		Aim:  "HTTP 503 → ActuationResult{Success:false, Error contains '503'}; zero panics",
		PackagesInvolved: []string{"internal/actuator", "internal/actuator/backends"},
		FunctionsTested:  []string{"backends.(*HTTPBackend).Execute (non-2xx path)"},
		Threshold: L6Threshold{
			Metric: "panics", Operator: "==", Value: 0, Unit: "count",
			Rationale: "5xx is expected in production — error, not panic",
		},
		Result: L6ResultData{
			Status: l6Status(passed), ActualValue: float64(panics), ActualUnit: "panics",
			FaultInjected:  "httptest.Server returns HTTP 503",
			CommandsSent:   1,
			CommandsFailed: func() int64 { if !res.Success { return 1 }; return 0 }(),
			Panics:         panics,
			DurationMs:     time.Since(start).Milliseconds(),
			ErrorMessages:  []string{fmt.Sprintf("panics=%d success=%v err=%v has_503=%v", panics, res.Success, res.Error, has503)},
		},
		OnExceed: "HTTPBackend panics on non-2xx → actuator goroutine crashes → actuator stops",
		Questions: L6Questions{
			WhatFaultWasInjected:  "httptest.Server writes StatusServiceUnavailable",
			WhyThisThreshold:      "503 is most common production backend error — must produce typed error",
			WhatHappensIfFails:    "Backend 5xx causes panic → process crash → Kubernetes restart",
			HowFaultWasInjected:   "handler: w.WriteHeader(http.StatusServiceUnavailable)",
			HowRecoveryVerified:   "N/A — error propagation test",
			WhatDegradedMeans:     "ActuationResult.Error = 'http backend: status=503'; orchestrator logs and continues",
		},
		RunAt: l6Now(), GoVersion: l6GoVer(),
	})

	if !passed {
		t.Fatalf(
			"L6-NET-004 FAILED: panics=%d success=%v err=%v has_503=%v\n"+
				"FIX: HTTPBackend.Execute must return fmt.Errorf containing status code for non-2xx responses.\n"+
				"File: internal/actuator/backends/http_backend.go",
			panics, res.Success, res.Error, has503,
		)
	}
	t.Logf("L6-NET-004 PASS | panics=0 error=%v has_503=true", res.Error)
}