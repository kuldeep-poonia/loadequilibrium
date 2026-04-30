package control_test

import (
	"encoding/json"
	"os"
	"testing"

	ctrl "github.com/loadequilibrium/loadequilibrium/internal/control"
)

type IntelligenceReport struct {
	ScaleLowLoad   float64 `json:"scale_low_load"`
	ScaleHighLoad  float64 `json:"scale_high_load"`
	ScaleChange    float64 `json:"scale_change"`

	MaxReplicas    int     `json:"max_replicas"`
	FinalBacklog   float64 `json:"final_backlog"`
	PeakBacklog    float64 `json:"peak_backlog"`

	Oscillation    float64 `json:"oscillation"`

	Health         string  `json:"health"`
}

func Test_Generate_Intelligence_Report(t *testing.T) {

	report := IntelligenceReport{}

	controller := ctrl.Controller{}

	sys := ctrl.SystemState{
		Replicas:         5,
		QueueLimit:       50,
		RetryLimit:       3,
		CacheAggression:  0.2,
		QueueDepth:       20,
		PredictedArrival: 40,
		ServiceRate:      10,
		SLATarget:        120,
		MinReplicas:      1,
		MaxReplicas:      50,
		MinRetry:         1,
		MaxRetry:         10,
	}

	var prevRep float64
	var osc float64
	var peakBacklog float64
	var maxReplicas int

	// track scaling
	var lowLoadScale float64
	var highLoadScale float64

	for step := 0; step < 120; step++ {

		// === LOAD PATTERN ===
		if step > 30 && step < 80 {
			sys.PredictedArrival = 120
		} else {
			sys.PredictedArrival = 40
		}

		// capture scale
		if step == 10 {
			lowLoadScale = float64(sys.Replicas)
		}
		if step == 60 {
			highLoadScale = float64(sys.Replicas)
		}

		controller.Tick(
			&sys,
			sys.QueueDepth,
			0.1,
			0.2,
			1,
			float64(sys.Replicas),
		)

		// oscillation
		if prevRep != 0 {
			osc += abs(float64(sys.Replicas) - prevRep)
		}
		prevRep = float64(sys.Replicas)

		// track max replicas
		if sys.Replicas > maxReplicas {
			maxReplicas = sys.Replicas
		}

		// backlog evolve
		sys.QueueDepth += sys.PredictedArrival - float64(sys.Replicas)*sys.ServiceRate
		if sys.QueueDepth < 0 {
			sys.QueueDepth = 0
		}

		// track peak backlog
		if sys.QueueDepth > peakBacklog {
			peakBacklog = sys.QueueDepth
		}
	}

	report.ScaleLowLoad = lowLoadScale
	report.ScaleHighLoad = highLoadScale
	report.ScaleChange = highLoadScale - lowLoadScale

	report.MaxReplicas = maxReplicas
	report.FinalBacklog = sys.QueueDepth
	report.PeakBacklog = peakBacklog
	report.Oscillation = osc

	// === HEALTH LOGIC ===
	switch {
	case report.FinalBacklog > 5000:
		report.Health = "FAIL (no control)"
	case report.ScaleChange < 0.5:
		report.Health = "FAIL (no scaling response)"
	case report.Oscillation > 120:
		report.Health = "UNSTABLE (oscillating)"
	case report.FinalBacklog < 500:
		report.Health = "GOOD"
	default:
		report.Health = "WEAK"
	}

	// === WRITE JSON ===
	file, err := os.Create("intelligence_report.json")
	if err != nil {
		t.Fatalf("file error: %v", err)
	}

	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")

	if err := enc.Encode(report); err != nil {
		t.Fatalf("json error: %v", err)
	}

	file.Close()
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}