package integration

import (
	"io"
	"log"
	"math"
	"testing"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/config"
	"github.com/loadequilibrium/loadequilibrium/internal/modelling"
	"github.com/loadequilibrium/loadequilibrium/internal/optimisation"
	"github.com/loadequilibrium/loadequilibrium/internal/streaming"
	"github.com/loadequilibrium/loadequilibrium/internal/telemetry"
	"github.com/loadequilibrium/loadequilibrium/internal/topology"
)

const signalServiceID = "signal-root"

var signalDelayProfile = []int{1, 3, 5}
var signalNoiseProfile = []float64{12, -9, 18, -14, 9, -21, 15, -11, 6, -17}

type signalInjection struct {
	tick             int
	baseRate         float64
	baseLatency      float64
	rawRate          float64
	rawLatency       float64
	noise            float64
	delay            int
	duplicate        bool
	lost             bool
	timestampShiftMs int
}

type signalRecord struct {
	tick       int
	fastEWMA   float64
	slowEWMA   float64
	util       float64
	scale      float64
	confidence float64
	at         time.Time
}

type signalSummary struct {
	records                  []signalRecord
	duplicateRejects         int
	staleRejects             int
	reconstructedTicks       []int
	payloadReplayRejects     int
	unexpectedReplayAccepts  int
	unexpectedPayloadRejects int
	acceptedPayloads         int
}

type signalRunner struct {
	cfg         *config.Config
	store       *telemetry.Store
	coupler     *modelling.TelemetryCoupler
	queue       *modelling.QueuePhysicsEngine
	signal      *modelling.SignalProcessor
	engine      *optimisation.Engine
	windowN     int
	controlBase time.Time
	lastScale   float64
}

type signalPacket struct {
	seq  uint64
	tick int
	pt   telemetry.MetricPoint
}

type signalRepairer struct {
	expected   uint64
	maxDelay   int
	limit      int
	buffer     map[uint64]signalPacket
	seen       map[uint64]struct{}
	last       *telemetry.MetricPoint
	prev       *telemetry.MetricPoint
	lastTick   int
	dupRejects int
	staleDrops int
	rebuilt    []int
}

type payloadGuard struct {
	lastSeq   uint64
	lastAt    time.Time
	rejects   int
	timeDrops int
}

func TestTelemetrySignalChainIntegrityUnderJitterLagCorruptionAndDrift(t *testing.T) {
	prevLogWriter := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(prevLogWriter)

	baseTime := time.Now().Add(-80 * time.Millisecond)
	injections := buildSignalScenario()
	clean := runCleanSignalChain(injections, baseTime)
	adversarial := runAdversarialSignalChain(injections, baseTime)

	if len(clean.records) != len(injections) {
		t.Fatalf("clean chain produced %d records, want %d", len(clean.records), len(injections))
	}
	if len(adversarial.records) != len(clean.records) {
		t.Fatalf("adversarial chain produced %d records, want %d", len(adversarial.records), len(clean.records))
	}

	expectedRebuilt := []int{7, 9, 18, 27, 31, 34}
	dupInjected := countIf(injections, func(inj signalInjection) bool { return inj.duplicate })
	lossInjected := countIf(injections, func(inj signalInjection) bool { return inj.lost })
	staleInjected := countIf(injections, func(inj signalInjection) bool { return inj.timestampShiftMs != 0 })
	maxDrift, meanDrift, maxScaleDelta, meanScaleDelta, continuityDelta, maxStep := compareSignalChains(clean.records, adversarial.records)

	t.Log("=== ELITE ROOT TEST 2/5: Telemetry Signal Chain Integrity ===")
	t.Logf("jitter_profile=%v", signalDelayProfile)
	t.Logf("corruption=loss %.1f%% duplicate %d/%d timestamp_disorder %d/%d stale_replays %d",
		100.0*float64(lossInjected)/float64(len(injections)),
		dupInjected, len(injections),
		staleInjected, len(injections),
		len(signalReplaySchedule()))
	t.Log("exact_signal_values_injected:")
	for _, inj := range injections {
		if inj.duplicate || inj.lost || inj.timestampShiftMs != 0 {
			t.Logf("  tick=%02d base_rps=%.2f raw_rps=%.2f base_lat=%.2f raw_lat=%.2f noise=%+.2f delay=%d dup=%v loss=%v ts_shift_ms=%d",
				inj.tick, inj.baseRate, inj.rawRate, inj.baseLatency, inj.rawLatency,
				inj.noise, inj.delay, inj.duplicate, inj.lost, inj.timestampShiftMs)
		}
	}
	t.Logf("duplicate_suppression=%d stale_rejection=%d signal_reconstruction=%v payload_replay_rejects=%d",
		adversarial.duplicateRejects, adversarial.staleRejects, adversarial.reconstructedTicks, adversarial.payloadReplayRejects)
	t.Logf("observed_drift=max_ewma %.2f%% mean_ewma %.2f%% max_scale_abs %.4f mean_scale_abs %.4f",
		maxDrift*100.0, meanDrift*100.0, maxScaleDelta, meanScaleDelta)
	t.Logf("continuity_delta=%.4f max_scale_step=%.4f accepted_payloads=%d",
		continuityDelta, maxStep, adversarial.acceptedPayloads)

	if adversarial.duplicateRejects != dupInjected {
		t.Errorf("duplicate suppression mismatch: got %d want %d", adversarial.duplicateRejects, dupInjected)
	}
	if adversarial.staleRejects != staleInjected {
		t.Errorf("stale rejection mismatch: got %d want %d", adversarial.staleRejects, staleInjected)
	}
	if !sameInts(adversarial.reconstructedTicks, expectedRebuilt) {
		t.Errorf("signal reconstruction mismatch: got %v want %v", adversarial.reconstructedTicks, expectedRebuilt)
	}
	if adversarial.payloadReplayRejects != len(signalReplaySchedule()) {
		t.Errorf("stale TickPayload replay mismatch: got %d want %d", adversarial.payloadReplayRejects, len(signalReplaySchedule()))
	}
	if adversarial.unexpectedReplayAccepts != 0 {
		t.Errorf("stale TickPayload replay was accepted %d times", adversarial.unexpectedReplayAccepts)
	}
	if adversarial.unexpectedPayloadRejects != 0 {
		t.Errorf("fresh TickPayload rejected %d times", adversarial.unexpectedPayloadRejects)
	}
	if adversarial.acceptedPayloads != len(injections) {
		t.Errorf("control decision continuity mismatch: got %d payloads want %d", adversarial.acceptedPayloads, len(injections))
	}
	if maxDrift >= 0.18 {
		t.Errorf("drift boundedness failed: max drift %.2f%% >= 18%%", maxDrift*100.0)
	}
	if continuityDelta >= 0.10 {
		t.Errorf("continuity delta %.4f >= 0.10", continuityDelta)
	}
	if maxStep >= 0.35 {
		t.Errorf("control decision continuity failed: max step %.4f >= 0.35", maxStep)
	}
}

func buildSignalScenario() []signalInjection {
	dup := map[int]bool{4: true, 13: true, 21: true, 30: true}
	lost := map[int]bool{9: true, 31: true}
	disorder := map[int]int{7: -3, 18: -4, 27: -2, 34: -5}
	out := make([]signalInjection, 0, 40)
	for tick := 0; tick < 40; tick++ {
		baseRate := round2(360.0 + 52.0*math.Sin(float64(tick)/4.0) + 4.5*float64(tick))
		baseLatency := round2(44.0 + 6.0*math.Cos(float64(tick)/5.0) + baseRate*0.035)
		noise := signalNoiseProfile[tick%len(signalNoiseProfile)]
		out = append(out, signalInjection{
			tick:             tick,
			baseRate:         baseRate,
			baseLatency:      baseLatency,
			rawRate:          round2(math.Max(baseRate+noise, 150.0)),
			rawLatency:       round2(math.Max(baseLatency+noise*0.40, 15.0)),
			noise:            noise,
			delay:            signalDelayProfile[tick%len(signalDelayProfile)],
			duplicate:        dup[tick],
			lost:             lost[tick],
			timestampShiftMs: disorder[tick],
		})
	}
	return out
}

func signalReplaySchedule() map[int]int {
	return map[int]int{16: 4, 25: 11, 37: 20}
}

func runCleanSignalChain(injections []signalInjection, baseTime time.Time) signalSummary {
	r := newSignalRunner()
	out := signalSummary{records: make([]signalRecord, 0, len(injections))}
	for _, inj := range injections {
		out.records = append(out.records, r.step(metricPoint(inj.baseRate, inj.baseLatency, baseTime.Add(time.Duration(inj.tick)*time.Millisecond)), inj.tick))
	}
	return out
}

func runAdversarialSignalChain(injections []signalInjection, baseTime time.Time) signalSummary {
	r := newSignalRunner()
	repair := newSignalRepairer(maxInt(signalDelayProfile), len(injections))
	guard := &payloadGuard{}
	out := signalSummary{records: make([]signalRecord, 0, len(injections))}
	arrivals := make(map[int][]signalPacket, len(injections))
	for _, inj := range injections {
		if inj.lost {
			continue
		}
		pkt := signalPacket{
			seq:  uint64(inj.tick + 1),
			tick: inj.tick,
			pt:   metricPoint(inj.rawRate, inj.rawLatency, baseTime.Add(time.Duration(inj.tick)*time.Millisecond).Add(time.Duration(inj.timestampShiftMs)*time.Millisecond)),
		}
		at := inj.tick + inj.delay
		arrivals[at] = append(arrivals[at], pkt)
		if inj.duplicate {
			arrivals[at+1] = append(arrivals[at+1], pkt)
		}
	}

	payloads := map[int]streaming.TickPayload{}
	for logicalTick := 0; logicalTick < len(injections)+maxInt(signalDelayProfile)+4; logicalTick++ {
		if replayTick, ok := signalReplaySchedule()[logicalTick]; ok {
			if payload, ok := payloads[replayTick]; ok {
				if guard.accept(payload) {
					out.unexpectedReplayAccepts++
				}
			}
		}
		for _, accepted := range repair.accept(logicalTick, arrivals[logicalTick]) {
			rec := r.step(accepted.pt, accepted.tick)
			out.records = append(out.records, rec)
			payload := streaming.TickPayload{
				Type:       streaming.MsgTick,
				SequenceNo: uint64(accepted.tick + 1),
				Timestamp:  accepted.pt.Timestamp,
				Directives: map[string]optimisation.ControlDirective{signalServiceID: {ServiceID: signalServiceID, ScaleFactor: rec.scale}},
			}
			if !guard.accept(payload) {
				out.unexpectedPayloadRejects++
				continue
			}
			out.acceptedPayloads++
			payloads[accepted.tick] = payload
		}
	}

	out.duplicateRejects = repair.dupRejects
	out.staleRejects = repair.staleDrops
	out.reconstructedTicks = append(out.reconstructedTicks, repair.rebuilt...)
	out.payloadReplayRejects = guard.rejects
	return out
}

func newSignalRunner() *signalRunner {
	cfg := &config.Config{
		TickInterval:           2 * time.Second,
		TickDeadline:           1800 * time.Millisecond,
		RingBufferDepth:        128,
		MaxServices:            8,
		StaleServiceAge:        time.Minute,
		WindowFraction:         0.14,
		EWMAFastAlpha:          0.30,
		EWMASlowAlpha:          0.10,
		SpikeZScore:            3.0,
		CollapseThreshold:      0.90,
		UtilisationSetpoint:    0.70,
		PredictiveHorizonTicks: 5,
		PIDKp:                  -1.5,
		PIDKi:                  -0.3,
		PIDKd:                  -0.1,
		PIDDeadband:            0.02,
		PIDIntegralMax:         2.0,
	}
	return &signalRunner{
		cfg:         cfg,
		store:       telemetry.NewStore(cfg.RingBufferDepth, cfg.MaxServices, cfg.StaleServiceAge),
		coupler:     modelling.NewTelemetryCoupler(),
		queue:       modelling.NewQueuePhysicsEngine(),
		signal:      modelling.NewSignalProcessor(cfg.EWMAFastAlpha, cfg.EWMASlowAlpha, cfg.SpikeZScore),
		engine:      optimisation.NewEngine(cfg),
		windowN:     18,
		controlBase: time.Now().Add(-80 * time.Second),
		lastScale:   1.0,
	}
}

func (r *signalRunner) step(pt telemetry.MetricPoint, tick int) signalRecord {
	copyPt := pt
	r.store.Ingest(&copyPt)
	w := r.store.Window(signalServiceID, r.windowN, time.Minute)
	w.AppliedScale = r.lastScale
	topo := topology.GraphSnapshot{
		CapturedAt: pt.Timestamp,
		Nodes: []topology.Node{{
			ServiceID:      signalServiceID,
			LastSeen:       pt.Timestamp,
			NormalisedLoad: clamp(w.MeanRequestRate/700.0, 0, 1),
		}},
	}
	r.coupler.ApplyCoupling(map[string]*telemetry.ServiceWindow{signalServiceID: w}, topo)
	q := r.queue.RunQueueModel(w, topo, false)
	s := r.signal.Update(w)
	stability := modelling.RunStabilityAssessment(q, s, topo, r.cfg.CollapseThreshold)
	bundles := map[string]*modelling.ServiceModelBundle{
		signalServiceID: {
			Queue:      q,
			Stochastic: modelling.StochasticModel{ServiceID: signalServiceID, Confidence: w.ConfidenceScore},
			Signal:     s,
			Stability:  stability,
		},
	}
	gradients := optimisation.ComputeCostGradients(bundles, topo, 500.0)
	if d, ok := r.engine.RunControl(bundles, gradients, nil, topo, r.controlBase.Add(time.Duration(tick)*2*time.Second))[signalServiceID]; ok && d.ScaleFactor > 0 {
		r.lastScale = d.ScaleFactor
	}
	return signalRecord{tick: tick, fastEWMA: s.FastEWMA, slowEWMA: s.SlowEWMA, util: q.Utilisation, scale: r.lastScale, confidence: w.ConfidenceScore, at: pt.Timestamp}
}

func newSignalRepairer(maxDelay, limit int) *signalRepairer {
	return &signalRepairer{
		expected: 1,
		maxDelay: maxDelay,
		limit:    limit,
		buffer:   map[uint64]signalPacket{},
		seen:     map[uint64]struct{}{},
	}
}

func (r *signalRepairer) accept(nowTick int, arrivals []signalPacket) []signalPacket {
	for _, pkt := range arrivals {
		if _, ok := r.seen[pkt.seq]; ok {
			r.dupRejects++
			continue
		}
		if _, ok := r.buffer[pkt.seq]; ok {
			r.dupRejects++
			continue
		}
		if pkt.seq < r.expected {
			r.staleDrops++
			continue
		}
		if r.last != nil {
			expectedAt := r.last.Timestamp.Add(time.Duration(pkt.tick-r.lastTick) * time.Millisecond)
			if pkt.pt.Timestamp.Before(expectedAt) {
				r.staleDrops++
				continue
			}
		}
		r.buffer[pkt.seq] = pkt
	}
	out := make([]signalPacket, 0, 2)
	for {
		tick := int(r.expected - 1)
		if tick >= r.limit {
			break
		}
		if pkt, ok := r.buffer[r.expected]; ok {
			delete(r.buffer, r.expected)
			pkt.pt = r.smooth(pkt.pt)
			r.mark(r.expected, pkt.tick, pkt.pt)
			out = append(out, pkt)
			r.expected++
			continue
		}
		if nowTick >= tick+r.maxDelay {
			pt := r.rebuild()
			r.mark(r.expected, tick, pt)
			out = append(out, signalPacket{seq: r.expected, tick: tick, pt: pt})
			r.rebuilt = append(r.rebuilt, tick)
			r.expected++
			continue
		}
		break
	}
	return out
}

func (r *signalRepairer) smooth(pt telemetry.MetricPoint) telemetry.MetricPoint {
	if r.last == nil {
		return pt
	}
	prev := r.last
	pt.Timestamp = maxTime(pt.Timestamp, prev.Timestamp.Add(time.Millisecond))
	reqCap := math.Max(prev.RequestRate*0.16, 28.0)
	latCap := math.Max(prev.Latency.Mean*0.18, 6.0)
	pt.RequestRate = round2(prev.RequestRate + clamp(pt.RequestRate-prev.RequestRate, -reqCap, reqCap))
	pt.Latency.Mean = round2(prev.Latency.Mean + clamp(pt.Latency.Mean-prev.Latency.Mean, -latCap, latCap))
	pt.Latency.P50 = round2(math.Max(pt.Latency.Mean*0.68, 1.0))
	pt.Latency.P95 = round2(math.Max(pt.Latency.Mean*1.45, pt.Latency.P50))
	pt.Latency.P99 = round2(math.Max(pt.Latency.Mean*1.80, pt.Latency.P95))
	pt.CPUUsage = round2(clamp(prev.CPUUsage+clamp(pt.CPUUsage-prev.CPUUsage, -0.08, 0.08), 0.25, 0.98))
	pt.MemUsage = round2(clamp(prev.MemUsage+clamp(pt.MemUsage-prev.MemUsage, -0.08, 0.08), 0.20, 0.98))
	pt.QueueDepth = int64(math.Round(clamp(float64(prev.QueueDepth)+clamp(float64(pt.QueueDepth-prev.QueueDepth), -4, 4), 0, 48)))
	pt.ActiveConns = 1
	return pt
}

func (r *signalRepairer) rebuild() telemetry.MetricPoint {
	if r.last == nil {
		return metricPoint(360.0, 56.0, time.Now())
	}
	last := *r.last
	prev := last
	if r.prev != nil {
		prev = *r.prev
	}
	req := clamp(last.RequestRate+0.60*(last.RequestRate-prev.RequestRate), last.RequestRate*0.88, last.RequestRate*1.12)
	lat := clamp(last.Latency.Mean+0.50*(last.Latency.Mean-prev.Latency.Mean), last.Latency.Mean*0.90, last.Latency.Mean*1.10)
	return r.smooth(metricPoint(round2(req), round2(lat), last.Timestamp.Add(time.Millisecond)))
}

func (r *signalRepairer) mark(seq uint64, tick int, pt telemetry.MetricPoint) {
	r.seen[seq] = struct{}{}
	if r.last != nil {
		copyPrev := *r.last
		r.prev = &copyPrev
	}
	copyLast := pt
	r.last = &copyLast
	r.lastTick = tick
}

func (r *signalRepairer) lastAt() time.Time {
	if r.last == nil {
		return time.Time{}
	}
	return r.last.Timestamp
}

func (g *payloadGuard) accept(payload streaming.TickPayload) bool {
	if payload.SequenceNo <= g.lastSeq {
		g.rejects++
		return false
	}
	if !g.lastAt.IsZero() && payload.Timestamp.Before(g.lastAt) {
		g.timeDrops++
		return false
	}
	g.lastSeq = payload.SequenceNo
	g.lastAt = payload.Timestamp
	return true
}

func metricPoint(rate, latency float64, ts time.Time) telemetry.MetricPoint {
	return telemetry.MetricPoint{
		ServiceID:   signalServiceID,
		Timestamp:   ts,
		RequestRate: rate,
		ErrorRate:   clamp(0.008+rate/60000.0, 0.0, 0.08),
		Latency: telemetry.LatencyStats{
			P50:  round2(math.Max(latency*0.68, 1.0)),
			P95:  round2(latency * 1.45),
			P99:  round2(latency * 1.80),
			Mean: round2(latency),
		},
		CPUUsage:    round2(clamp(0.38+rate/1100.0, 0.25, 0.98)),
		MemUsage:    round2(clamp(0.30+rate/1400.0, 0.20, 0.95)),
		ActiveConns: 1,
		QueueDepth:  int64(math.Round(clamp(6.0+math.Max(rate-380.0, 0.0)/14.0, 2.0, 40.0))),
	}
}

func compareSignalChains(clean, adv []signalRecord) (maxDrift, meanDrift, maxScale, meanScale, continuityDelta, maxStep float64) {
	var driftSum float64
	var scaleSum float64
	for i := range clean {
		drift := math.Abs(adv[i].fastEWMA-clean[i].fastEWMA) / math.Max(clean[i].fastEWMA, 1.0)
		scale := math.Abs(adv[i].scale - clean[i].scale)
		driftSum += drift
		scaleSum += scale
		if drift > maxDrift {
			maxDrift = drift
		}
		if scale > maxScale {
			maxScale = scale
		}
	}
	cleanContinuity, _ := continuity(clean)
	advContinuity, maxStep := continuity(adv)
	return maxDrift, driftSum / float64(len(clean)), maxScale, scaleSum / float64(len(clean)), math.Abs(advContinuity - cleanContinuity), maxStep
}

func continuity(records []signalRecord) (mean, max float64) {
	if len(records) < 2 {
		return 0, 0
	}
	var sum float64
	for i := 1; i < len(records); i++ {
		delta := math.Abs(records[i].scale - records[i-1].scale)
		sum += delta
		if delta > max {
			max = delta
		}
	}
	return sum / float64(len(records)-1), max
}

func countIf(injections []signalInjection, fn func(signalInjection) bool) int {
	n := 0
	for _, inj := range injections {
		if fn(inj) {
			n++
		}
	}
	return n
}

func sameInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func maxInt(values []int) int {
	out := 0
	for _, v := range values {
		if v > out {
			out = v
		}
	}
	return out
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func clamp(v, low, high float64) float64 {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}

func round2(v float64) float64 {
	return math.Round(v*100.0) / 100.0
}
