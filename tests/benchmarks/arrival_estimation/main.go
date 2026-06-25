package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"runtime"
	"sort"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/autopilot"
)

type Scenario struct {
	Name string
	Data []float64
	True []float64
}

func generateScenario(name string, n int, seed int64) Scenario {
	rand.Seed(seed)
	sc := Scenario{Name: name, Data: make([]float64, n), True: make([]float64, n)}

	switch name {
	case "Constant":
		for i := 0; i < n; i++ {
			sc.True[i] = 100.0
			sc.Data[i] = 100.0 + rand.NormFloat64()*5.0
		}
	case "Regime-Switching":
		for i := 0; i < n; i++ {
			base := 10.0
			if i > n/4 { base = 50.0 }
			if i > n/2 { base = 500.0 }
			if i > 3*n/4 { base = 100.0 }
			sc.True[i] = base
			sc.Data[i] = base + rand.NormFloat64()*(base*0.1)
		}
	case "Heavy-Tailed":
		for i := 0; i < n; i++ {
			base := 50.0
			u := rand.Float64()
			paretoNoise := 5.0 / math.Pow(1.0-u, 1.0/1.5)
			if paretoNoise > 1000 { paretoNoise = 1000 }
			sc.True[i] = base
			sc.Data[i] = base + paretoNoise
		}
	case "Brownian Noise":
		base := 100.0
		for i := 0; i < n; i++ {
			base += rand.NormFloat64() * 2.0
			if base < 10 { base = 10 }
			sc.True[i] = base
			sc.Data[i] = base + rand.NormFloat64()*5.0
		}
	}
	return sc
}

type MetricSet struct {
	Mean   float64 `json:"mean"`
	Median float64 `json:"median"`
	StdDev float64 `json:"std_dev"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	CI95   float64 `json:"ci_95"`
}

type ReportEntry struct {
	Scenario   string    `json:"scenario"`
	Algorithm  string    `json:"algorithm"`
	RMSE       MetricSet `json:"rmse"`
	MAE        MetricSet `json:"mae"`
	P50Err     MetricSet `json:"p50_err"`
	P95Err     MetricSet `json:"p95_err"`
	P99Err     MetricSet `json:"p99_err"`
	MaxErr     MetricSet `json:"max_err"`
	CPUTimeMs  float64   `json:"cpu_time_ms"`
	AllocBytes uint64    `json:"alloc_bytes"`
	GC_Pauses  uint32    `json:"gc_pauses"`
}

func stats(data []float64) MetricSet {
	if len(data) == 0 { return MetricSet{} }
	sort.Float64s(data)
	min := data[0]
	max := data[len(data)-1]
	med := data[len(data)/2]

	var sum float64
	for _, v := range data { sum += v }
	mean := sum / float64(len(data))

	var vSum float64
	for _, v := range data { vSum += (v - mean)*(v - mean) }
	std := math.Sqrt(vSum / float64(len(data)))

	ci95 := 1.96 * std / math.Sqrt(float64(len(data)))
	return MetricSet{
		Mean:   mean,
		Median: med,
		StdDev: std,
		Min:    min,
		Max:    max,
		CI95:   ci95,
	}
}

func percentile(data []float64, p float64) float64 {
	if len(data) == 0 { return 0 }
	// data is assumed to be unsorted in this function, so copy and sort
	cp := make([]float64, len(data))
	copy(cp, data)
	sort.Float64s(cp)
	idx := int(math.Ceil(float64(len(cp))*p)) - 1
	if idx < 0 { idx = 0 }
	return cp[idx]
}

func main() {
	outJSON := flag.String("out", "benchmark_results.json", "Output JSON path")
	seeds := flag.Int("seeds", 10, "Number of random seeds")
	ticks := flag.Int("ticks", 10000, "Ticks per simulation")
	flag.Parse()

	scenarios := []string{"Constant", "Regime-Switching", "Heavy-Tailed", "Brownian Noise"}
	
	algos := []struct {
		Name     string
		Strategy autopilot.ArrivalEstimator
	}{
		{"1. Legacy", &autopilot.LegacyArrivalEstimator{}},
		{"2. Statistical 5-Sigma", &autopilot.StatisticalArrivalEstimator{SigmaMultiplier: 5.0, NoiseFloorRatio: 0.1}},
	}

	var report []ReportEntry

	fmt.Printf("Running MC Benchmark Suite (seeds=%d, ticks=%d)\n", *seeds, *ticks)

	for _, sname := range scenarios {
		for _, algo := range algos {
			
			var rmses, maes, maxs, p50s, p95s, p99s []float64
			var totalCPU time.Duration
			var totalAlloc uint64
			var totalGC uint32
			
			for seed := int64(0); seed < int64(*seeds); seed++ {
				sc := generateScenario(sname, *ticks, seed)
				
				eng := &autopilot.IdentificationEngine{
					FastGain: 0.15, SlowGain: 0.02, BlendGain: 0.05, VarGain: 0.1,
					ArrivalStrategy: algo.Strategy,
				}
				s := autopilot.IdentificationState{
					ArrivalFast: sc.Data[0], ArrivalSlow: sc.Data[0], ArrivalEstimate: sc.Data[0],
					ArrivalWelfordM: sc.Data[0],
				}
				
				var sse, sumAbs, maxErr float64
				var errs []float64
				
				startMem := new(runtime.MemStats)
				runtime.ReadMemStats(startMem)
				t0 := time.Now()
				
				for i := 0; i < len(sc.Data); i++ {
					s = eng.ArrivalStrategy.Update(s, sc.Data[i], eng)
					err := math.Abs(s.ArrivalEstimate - sc.True[i])
					sse += err*err
					sumAbs += err
					if err > maxErr { maxErr = err }
					errs = append(errs, err)
				}
				
				t1 := time.Now()
				endMem := new(runtime.MemStats)
				runtime.ReadMemStats(endMem)
				
				totalCPU += t1.Sub(t0)
				totalAlloc += endMem.TotalAlloc - startMem.TotalAlloc
				totalGC += endMem.NumGC - startMem.NumGC
				
				rmses = append(rmses, math.Sqrt(sse/float64(*ticks)))
				maes = append(maes, sumAbs/float64(*ticks))
				maxs = append(maxs, maxErr)
				p50s = append(p50s, percentile(errs, 0.50))
				p95s = append(p95s, percentile(errs, 0.95))
				p99s = append(p99s, percentile(errs, 0.99))
			}
			
			entry := ReportEntry{
				Scenario:   sname,
				Algorithm:  algo.Name,
				RMSE:       stats(rmses),
				MAE:        stats(maes),
				P50Err:     stats(p50s),
				P95Err:     stats(p95s),
				P99Err:     stats(p99s),
				MaxErr:     stats(maxs),
				CPUTimeMs:  float64(totalCPU.Milliseconds()) / float64(*seeds),
				AllocBytes: totalAlloc / uint64(*seeds),
				GC_Pauses:  totalGC,
			}
			report = append(report, entry)
			fmt.Printf("Completed %s - %s\n", sname, algo.Name)
		}
	}

	file, _ := os.Create(*outJSON)
	defer file.Close()
	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	enc.Encode(report)
	
	fmt.Printf("Report saved to %s\n", *outJSON)
}
