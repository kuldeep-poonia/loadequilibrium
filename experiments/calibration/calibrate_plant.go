package main

import (
	"fmt"
	"github.com/loadequilibrium/loadequilibrium/internal/physics"
)

func main() {
	fmt.Printf("%-15s | %-10s | %-10s\n", "InflowHigh", "avgQ", "maxQ")
	fmt.Println("-------------------------------------------")

	for imh := 0.8; imh <= 1.401; imh += 0.05 {
		plant := physics.NewFluidPlant(42)
		plant.P.InflowMeanHigh = imh

		// Run for 200,000 steps (10,000s virtual time)
		steps := 200000
		dt := 0.05
		var sumQ float64
		maxQ := 0.0

		// Burn-in (10,000 steps)
		for i := 0; i < 10000; i++ {
			plant.Step(dt)
		}

		// Measurement
		for i := 0; i < steps; i++ {
			plant.Step(dt)
			q := plant.Q
			sumQ += q
			if q > maxQ {
				maxQ = q
			}
		}

		avgQ := sumQ / float64(steps)
		fmt.Printf("%-15.2f | %-10.4f | %-10.4f\n", imh, avgQ, maxQ)
	}
}
