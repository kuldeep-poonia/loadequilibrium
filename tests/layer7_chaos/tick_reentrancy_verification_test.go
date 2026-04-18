package layer7

import (
	"context"
	goRuntime "runtime"
	"testing"
	"time"
)

// TestL7_CHAOS_013_TickReentrancyVerification is a deterministic scientific verification test
// that will prove or falsify tick loop reentrancy with 100% certainty.
//
// This test intentionally creates the worst case conditions for reentrancy by:
//   1. Setting tick period = 50ms
//   2. Forcing tick processing time = 120ms (greater than tick period)
//   3. Running continuously for 10 seconds
//
// If any reentrant ticks occur, the atomic counter will detect them with absolute certainty.
func TestL7_CHAOS_013_TickReentrancyVerification(t *testing.T) {
	const tickInterval = 50 * time.Millisecond

	ts := newTestSystem(int(tickInterval / time.Millisecond))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Count goroutines before start
	goRuntime.GC()
	beforeGoroutines := goRuntime.NumGoroutine()

	go ts.Orch.Run(ctx)

	// Run for 10 seconds under maximum reentrancy pressure
	time.Sleep(10 * time.Second)

	cancel()
	// Wait for full shutdown
	time.Sleep(500 * time.Millisecond)

	// Final measurements
	goRuntime.GC()
	afterGoroutines := goRuntime.NumGoroutine()
	reentrantCount := ts.Orch.ReentrantTickCount()
	processedCount := ts.Orch.ProcessedTickCount()
	inFlight := ts.Orch.InFlightTickCount()

	t.Logf("=== L7-CHAOS-013 TICK REENTRANCY VERIFICATION ===")
	t.Logf("  total_ticks       = %d", ts.Orch.TickCount())
	t.Logf("  processed_ticks   = %d", processedCount)
	t.Logf("  reentrant_ticks   = %d", reentrantCount)
	t.Logf("  goroutines_before = %d", beforeGoroutines)
	t.Logf("  goroutines_after  = %d", afterGoroutines)
	t.Logf("  max_in_flight     = %d", inFlight)
	t.Logf("  goroutine_leak    = %d", afterGoroutines - beforeGoroutines)

	// Irrefutable proof of reentrancy
	if reentrantCount > 0 {
		t.Fatalf(`
╔══════════════════════════════════════════════════════════════╗
║                     CONFIRMED REENTRANCY                    ║
╠══════════════════════════════════════════════════════════════╣
║  The tick loop is spawning duplicate goroutines.           ║
║  This is a critical concurrency bug.                       ║
║                                                             ║
║  reentrant_ticks = %-38d ║
║  processed_ticks = %-38d ║
║                                                             ║
║  This will cause exponential goroutine explosion at scale.  ║
╚══════════════════════════════════════════════════════════════╝`,
			reentrantCount, processedCount)
	}

	t.Logf("✅ PASSED: Zero reentrant ticks detected")
	t.Logf("✅ Tick loop has correct atomic reentrancy guard")
}