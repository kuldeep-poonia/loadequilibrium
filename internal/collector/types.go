package collector

import (
	"sync/atomic"
	"time"
)

type ServiceTarget struct {
	ContainerID      string
	ContainerName    string
	ServiceID        string
	Environment      string
	Team             string
	MetricsURLs      []string
	Labels           map[string]string
	MemoryLimitBytes int64
	DiscoveredAt     time.Time
}

type Stats struct {
	DiscoveredServices atomic.Int64
	ScrapeSuccessTotal atomic.Int64
	ScrapeErrorTotal   atomic.Int64
	PointsBuiltTotal   atomic.Int64
	PointsQueuedTotal  atomic.Int64
	PointsDroppedTotal atomic.Int64
	BatchesSentTotal   atomic.Int64
	BatchesErrorTotal  atomic.Int64
	CircuitOpen        atomic.Bool
	LastDiscoveryUnix  atomic.Int64
	LastScrapeUnix     atomic.Int64
	LastIngestUnix     atomic.Int64
	LastErrorUnix      atomic.Int64
}

func (s *Stats) markError(now time.Time) {
	s.LastErrorUnix.Store(now.Unix())
}
