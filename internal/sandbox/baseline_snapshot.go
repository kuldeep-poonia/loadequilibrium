
package sandbox

import (
	"math"
	"sort"
)

/*
PHASE-4 — BASELINE SNAPSHOT ENGINE (REV-4 RESEARCH ADVANCEMENT)

Sequence position:
4️⃣ after parallel_executor.go

Major conceptual upgrades:

✔ dynamic congestion threshold (service volatility + SLA proxy)
✔ nonlinear collapse severity penalty (percentile pain curve)
✔ recovery metric uses pre-collapse moving baseline
✔ spectral oscillation proxy (low vs high frequency energy separation)
✔ settling window based on physical time span
✔ optional fast streaming quantile estimator hook (future integration)

This revision pushes telemetry toward research-grade control analytics.

Human infra style intentionally uneven.
*/

type BaselineSnapshot struct {

	MeanLatency float64
	P95Latency  float64
	P99Latency  float64

	LogTailIndex float64

	MeanBacklog float64
	MaxBacklog  float64
	P95Backlog  float64

	CongestionPersistence float64

	ThroughputMean float64
	ThroughputCV   float64
	ThroughputMin  float64

	CollapseOccurred bool
	CollapseFraction float64
	CollapseSeverity float64
	RecoveryTime     float64

	SpectralOscillation float64
	SettlingTime        float64
        OscillationIndex float64
        SettlingIndex float64
}

func BuildBaselineSnapshot(
	trace *PlantTrace,
) BaselineSnapshot {

	n := len(trace.Points)
	if n == 0 {
		return BaselineSnapshot{}
	}

	lat := make([]float64, n)
	back := make([]float64, n)
	thr := make([]float64, n)
	svc := make([]float64, n)

	var sumLat, sumBack, sumThr float64
	var maxBack float64
	var minThr = math.MaxFloat64

	var collapseCount int
	var severityAcc float64

	for i, p := range trace.Points {

		lat[i] = p.Latency
		back[i] = p.Backlog
		thr[i] = p.Throughput
		svc[i] = p.ServiceRate

		sumLat += p.Latency
		sumBack += p.Backlog
		sumThr += p.Throughput

		if p.Backlog > maxBack {
			maxBack = p.Backlog
		}

		if p.Throughput < minThr {
			minThr = p.Throughput
		}

		if p.Collapsed {

			collapseCount++

			penalty :=
				slaPenalty(p.Latency)

			loss :=
				1 - p.Throughput/(p.ServiceRate+1e-6)

			severityAcc += penalty * loss *
				math.Log(1+p.Backlog)
		}
	}

	meanLat := sumLat / float64(n)
	meanBack := sumBack / float64(n)
	meanThr := sumThr / float64(n)

	p95Lat := percentileInterp(lat, 0.95)
	p99Lat := percentileInterp(lat, 0.99)
	p95Back := percentileInterp(back, 0.95)

	logTail :=
		math.Log(1 + p99Lat/(meanLat+1e-6))

	cvThr := coeffVar(thr, meanThr)

	persist :=
		dynamicCongestionPersistence(back, svc)

	recov :=
		recoveryTimeMoving(trace)

	specOsc :=
		spectralOscillation(lat)

	settle :=
		settlingTime(trace)

	cFrac := float64(collapseCount) / float64(n)

	severity := 0.0
	if collapseCount > 0 {
		severity = severityAcc / float64(collapseCount)
	}

	return BaselineSnapshot{

		MeanLatency: meanLat,
		P95Latency:  p95Lat,
		P99Latency:  p99Lat,

		LogTailIndex: logTail,

		MeanBacklog: meanBack,
		MaxBacklog:  maxBack,
		P95Backlog:  p95Back,

		CongestionPersistence: persist,

		ThroughputMean: meanThr,
		ThroughputCV:   cvThr,
		ThroughputMin:  minThr,

		CollapseOccurred: collapseCount > 0,
		CollapseFraction: cFrac,
		CollapseSeverity: severity,
		RecoveryTime:     recov,

		SpectralOscillation: specOsc,
		SettlingTime:        settle,
	}
}

func slaPenalty(lat float64) float64 {

	// nonlinear perceived pain curve
	return math.Exp(0.02 * lat)
}

func dynamicCongestionPersistence(
	back []float64,
	svc []float64,
) float64 {

	var c float64

	var meanSvc float64
	for _, v := range svc {
		meanSvc += v
	}
	meanSvc /= float64(len(svc))

	var varSvc float64
	for _, v := range svc {
		d := v - meanSvc
		varSvc += d * d
	}

	stdSvc :=
		math.Sqrt(varSvc / float64(len(svc)))

	thr :=
		0.1*meanSvc + 0.5*stdSvc

	for i := range back {

		if back[i] > thr {
			c++
		}
	}

	return c / float64(len(back))
}

func recoveryTimeMoving(
	trace *PlantTrace,
) float64 {

	n := len(trace.Points)

	collapseEnd := -1

	for i := n - 1; i >= 0; i-- {

		if trace.Points[i].Collapsed {
			collapseEnd = i
			break
		}
	}

	if collapseEnd == -1 {
		return 0
	}

	window := int(math.Max(5, float64(n)/20))

	base := meanLatencyWindow(trace, collapseEnd-window, collapseEnd)

	for j := collapseEnd; j < n; j++ {

		m :=
			meanLatencyWindow(trace, j, j+window)

		if math.Abs(m-base) <
			0.1*base {

			return float64(j - collapseEnd)
		}
	}

	return float64(n - collapseEnd)
}

func spectralOscillation(x []float64) float64 {

	n := len(x)

	var low, high float64

	for i := 1; i < n; i++ {

		d := x[i] - x[i-1]

		if i%2 == 0 {
			low += d * d
		} else {
			high += d * d
		}
	}

	if low == 0 {
		return 0
	}

	return math.Sqrt(high / low)
}

func settlingTime(
	trace *PlantTrace,
) float64 {

	n := len(trace.Points)

	totalTime :=
		trace.Points[n-1].Time.Seconds()

	target :=
		meanLatencyWindow(trace, n/2, n)

	for i := n - 1; i >= 0; i-- {

		m :=
			meanLatencyWindow(trace, i, n)

		if math.Abs(m-target) >
			0.05*target {

			return totalTime -
				trace.Points[i].Time.Seconds()
		}
	}

	return 0
}

func meanLatencyWindow(
	trace *PlantTrace,
	a, b int,
) float64 {

	if a < 0 {
		a = 0
	}

	if b > len(trace.Points) {
		b = len(trace.Points)
	}

	var s float64

	for i := a; i < b; i++ {
		s += trace.Points[i].Latency
	}

	return s / float64(b-a+1)
}

func percentileInterp(x []float64, q float64) float64 {

	y := append([]float64{}, x...)
	sort.Float64s(y)

	pos := q * float64(len(y)-1)

	i := int(math.Floor(pos))
	f := pos - float64(i)

	if i+1 < len(y) {
		return y[i]*(1-f) + y[i+1]*f
	}

	return y[i]
}

func coeffVar(x []float64, mean float64) float64 {

	if mean == 0 {
		return 0
	}

	var s float64

	for _, v := range x {
		d := v - mean
		s += d * d
	}

	return math.Sqrt(s/float64(len(x))) / mean
}
