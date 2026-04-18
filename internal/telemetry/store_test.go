package telemetry

import (
	"testing"
	"time"
)

func TestIngestUpdatesLastSeenForPruning(t *testing.T) {
	store := NewStore(8, 4, time.Second)
	observedAt := time.Now().Add(-2 * time.Second)

	store.Ingest(&MetricPoint{
		ServiceID:   "checkout",
		Timestamp:   observedAt,
		RequestRate: 10,
		Latency:     LatencyStats{Mean: 12},
	})

	pruned := store.Prune(time.Now())
	if len(pruned) != 1 || pruned[0] != "checkout" {
		t.Fatalf("expected checkout to be pruned from lastSeen state, got %#v", pruned)
	}
}
