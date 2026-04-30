package main

import (
	"fmt"
	"log"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/sandbox"
)

func main() {
	// Configure the scenario
	scenarioCfg := sandbox.ScenarioConfig{
		BaseArrival: 500,
		BaseService: 80,
		Duration:    3 * time.Hour,
		Step:        time.Second,
		Seed:        42,
		NoiseStd:    0.05,
		ARCoef:      0.8,
		RetryGain:   1.5,
		RetryDecay:  0.95,
		RetryJitter: 0.1,
		SaturationCap: 200,
		BurstOnProb: 0.1,
		BurstOffProb: 0.2,
		ParetoAlpha: 1.5,
		BurstCeiling: 300,
		HeavyTailProb: 0.05,
		Harmonics: []float64{10, 5, 2},
		PhaseDrift: 0.01,
		ShockTimes: []time.Duration{15 * time.Minute, 45 * time.Minute},
		ShockMag: 50,
		RelaxTau: 300, // 5 minutes in seconds
		CapacityDrop: 0.3,
		CollapseProb: 0.2,
		RateLimit: 200,
		ShedProb: 0.1,
		BreakerThresh: 5.0,
		FanoutBase: 1.0,
		FanoutLoad: 0.1,
		FanoutVar: 0.05,
	}

	// Configure the executor
	execCfg := sandbox.ExecutorConfig{
		InitWorkers:    4,
		MaxWorkers:     8,
		JobBuffer:      32,
		ResultBuffer:   16,
		ScalerInterval: 300 * time.Millisecond,
		EnableAutoscale: true,
		IdleTimeout:    2 * time.Second,
		ReorderWindow:  128,
		FlushOnCancel:  false,
	}

	// Run Phase 1 comparison
	fmt.Println("Starting Phase 1: Baseline vs Engine Comparison...")
	result, err := sandbox.RunPhase1Comparison(
		42,                        // seed
		sandbox.ScenarioSpike,     // scenario kind
		scenarioCfg,               // scenario configuration
		execCfg,                   // executor configuration
	)
	if err != nil {
		log.Fatalf("Phase 1 comparison failed: %v", err)
	}

	fmt.Println("\n=== Phase 1 Comparison Results ===")
	fmt.Printf("\nBaseline Metrics:\n")
	fmt.Printf("  Failures:      %d\n", result.Baseline.Failures)
	fmt.Printf("  Recovery Time: %.2f seconds\n", result.Baseline.RecoveryTime)
	fmt.Printf("  Throughput:    %.2f requests/sec\n", result.Baseline.Throughput)
	fmt.Printf("  Total Cost:    %.4f\n", result.Baseline.TotalCost)

	fmt.Printf("\nEngine Metrics:\n")
	fmt.Printf("  Failures:      %d\n", result.Engine.Failures)
	fmt.Printf("  Recovery Time: %.2f seconds\n", result.Engine.RecoveryTime)
	fmt.Printf("  Throughput:    %.2f requests/sec\n", result.Engine.Throughput)
	fmt.Printf("  Total Cost:    %.4f\n", result.Engine.TotalCost)

	fmt.Printf("\nImprovement Metrics:\n")
	for key, value := range result.Improvement {
		fmt.Printf("  %s: %.4f\n", key, value)
	}

	// Save result to JSON
	outputPath := "tests/results/phase1_proof.json"
	fmt.Printf("\nSaving results to %s...\n", outputPath)
	err = sandbox.SavePhase1Result(result, outputPath)
	if err != nil {
		log.Fatalf("Failed to save Phase 1 result: %v", err)
	}

	fmt.Printf("✓ Phase 1 comparison completed successfully!\n")
	fmt.Printf("✓ Results saved to: %s\n", outputPath)
}
