package sandbox

import "time"

type PlantPoint struct {
    Time        time.Duration
    Latency     float64
    Backlog     float64
    Throughput  float64
    ServiceRate float64
    Collapsed   bool
}

type PlantTrace struct {
    Points []PlantPoint
}
