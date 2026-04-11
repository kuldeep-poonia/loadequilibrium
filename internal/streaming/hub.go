package streaming

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/ws"
)

const (
	writeTimeout      = 5 * time.Second
	pingInterval      = 15 * time.Second
	sendBufferSize    = 16
	pongWait          = 60 * time.Second
	defaultMaxClients = 50

	pressureProbeFrames = 1024
	pressureProbeChunk  = 64
	pressureProbeAfter  = sendBufferSize
	pressureProbeWindow = 100 * time.Millisecond
)

var pressureProbePayload = [125]byte{}

type Hub struct {
	mu          sync.RWMutex
	clients     map[*client]struct{}
	seqNo       atomic.Uint64
	maxClients  int
	lastPayload *TickPayload // Cache latest payload for REST endpoints
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*client]struct{}),
		maxClients: defaultMaxClients,
	}
}

func (h *Hub) SetMaxClients(n int) {
	if n > 0 {
		h.maxClients = n
	}
}

/* ================= SAFE FLOAT ================= */

func safeFloat(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return 0
	}
	return x
}

/* ================ PAYLOAD SANITISER ================= */

// sanitizePayload converts all NaN and Inf values to safe defaults
func sanitizePayload(p *TickPayload) {
	if p == nil {
		return
	}

	// Sanitize scalar fields
	p.TickHealthMs = safeFloat(p.TickHealthMs)
	p.DegradedFraction = safeFloat(p.DegradedFraction)
	p.JitterMs = safeFloat(p.JitterMs)

	// Sanitize Objective
	p.Objective.CompositeScore = safeFloat(p.Objective.CompositeScore)
	p.Objective.MaxCollapseRisk = safeFloat(p.Objective.MaxCollapseRisk)
	p.Objective.CascadeFailureProbability = safeFloat(p.Objective.CascadeFailureProbability)
	p.Objective.PredictedP99LatencyMs = safeFloat(p.Objective.PredictedP99LatencyMs)
	p.Objective.OscillationRisk = safeFloat(p.Objective.OscillationRisk)
	p.Objective.RiskAcceleration = safeFloat(p.Objective.RiskAcceleration)
	p.Objective.TrajectoryScore = safeFloat(p.Objective.TrajectoryScore)
	p.Objective.LatencyWeight = safeFloat(p.Objective.LatencyWeight)
	p.Objective.UtilisationWeight = safeFloat(p.Objective.UtilisationWeight)
	p.Objective.RiskWeight = safeFloat(p.Objective.RiskWeight)
	p.Objective.ReferenceLatencyMs = safeFloat(p.Objective.ReferenceLatencyMs)
	p.Objective.TrendStabilityMargin = safeFloat(p.Objective.TrendStabilityMargin)

	// Sanitize RuntimeMetrics
	p.RuntimeMetrics.AvgPruneMs = safeFloat(p.RuntimeMetrics.AvgPruneMs)
	p.RuntimeMetrics.AvgWindowsMs = safeFloat(p.RuntimeMetrics.AvgWindowsMs)
	p.RuntimeMetrics.AvgTopologyMs = safeFloat(p.RuntimeMetrics.AvgTopologyMs)
	p.RuntimeMetrics.AvgCouplingMs = safeFloat(p.RuntimeMetrics.AvgCouplingMs)
	p.RuntimeMetrics.AvgModellingMs = safeFloat(p.RuntimeMetrics.AvgModellingMs)
	p.RuntimeMetrics.AvgOptimiseMs = safeFloat(p.RuntimeMetrics.AvgOptimiseMs)
	p.RuntimeMetrics.AvgSimMs = safeFloat(p.RuntimeMetrics.AvgSimMs)
	p.RuntimeMetrics.AvgReasoningMs = safeFloat(p.RuntimeMetrics.AvgReasoningMs)
	p.RuntimeMetrics.AvgBroadcastMs = safeFloat(p.RuntimeMetrics.AvgBroadcastMs)
	p.RuntimeMetrics.PredictedCriticalMs = safeFloat(p.RuntimeMetrics.PredictedCriticalMs)

	// Sanitize NetworkEquilibrium
	p.NetworkEquilibrium.SystemRhoMean = safeFloat(p.NetworkEquilibrium.SystemRhoMean)
	p.NetworkEquilibrium.SystemRhoVariance = safeFloat(p.NetworkEquilibrium.SystemRhoVariance)
	p.NetworkEquilibrium.EquilibriumDelta = safeFloat(p.NetworkEquilibrium.EquilibriumDelta)
	p.NetworkEquilibrium.MaxCongestionFeedback = safeFloat(p.NetworkEquilibrium.MaxCongestionFeedback)
	p.NetworkEquilibrium.NetworkSaturationRisk = safeFloat(p.NetworkEquilibrium.NetworkSaturationRisk)

	// Sanitize TopologySensitivity
	p.TopologySensitivity.SystemFragility = safeFloat(p.TopologySensitivity.SystemFragility)
	p.TopologySensitivity.MaxAmplificationScore = safeFloat(p.TopologySensitivity.MaxAmplificationScore)
	if p.TopologySensitivity.ByService != nil {
		for svc, snap := range p.TopologySensitivity.ByService {
			snap.PerturbationScore = safeFloat(snap.PerturbationScore)
			p.TopologySensitivity.ByService[svc] = snap
		}
	}

	// Sanitize FixedPointEquilibrium
	p.FixedPointEquilibrium.SystemicCollapseProb = safeFloat(p.FixedPointEquilibrium.SystemicCollapseProb)
	p.FixedPointEquilibrium.ConvergenceRate = safeFloat(p.FixedPointEquilibrium.ConvergenceRate)
	p.FixedPointEquilibrium.StabilityMargin = safeFloat(p.FixedPointEquilibrium.StabilityMargin)
	if p.FixedPointEquilibrium.EquilibriumRho != nil {
		for svc, rho := range p.FixedPointEquilibrium.EquilibriumRho {
			p.FixedPointEquilibrium.EquilibriumRho[svc] = safeFloat(rho)
		}
	}
	if p.FixedPointEquilibrium.PerturbationSensitivity != nil {
		for svc, psens := range p.FixedPointEquilibrium.PerturbationSensitivity {
			p.FixedPointEquilibrium.PerturbationSensitivity[svc] = safeFloat(psens)
		}
	}

	// Sanitize PressureHeatmap
	if p.PressureHeatmap != nil {
		for svc, pressure := range p.PressureHeatmap {
			p.PressureHeatmap[svc] = safeFloat(pressure)
		}
	}

	// Sanitize SaturationCountdowns
	if p.SaturationCountdowns != nil {
		for svc, cd := range p.SaturationCountdowns {
			p.SaturationCountdowns[svc] = safeFloat(cd)
		}
	}

	// Sanitize PredictionHorizon
	if p.PredictionHorizon != nil {
		for svc, ph := range p.PredictionHorizon {
			p.PredictionHorizon[svc] = safeFloat(ph)
		}
	}

	// Sanitize NetworkCoupling
	if p.NetworkCouplingData != nil {
		for svc, nc := range p.NetworkCouplingData {
			nc.EffectivePressure = safeFloat(nc.EffectivePressure)
			nc.PathSaturationRisk = safeFloat(nc.PathSaturationRisk)
			nc.CoupledArrivalRate = safeFloat(nc.CoupledArrivalRate)
			nc.PathEquilibriumRho = safeFloat(nc.PathEquilibriumRho)
			nc.CongestionFeedbackScore = safeFloat(nc.CongestionFeedbackScore)
			nc.PathSaturationHorizonSec = safeFloat(nc.PathSaturationHorizonSec)
			nc.PathCollapseProb = safeFloat(nc.PathCollapseProb)
			nc.SteadyStateP0 = safeFloat(nc.SteadyStateP0)
			nc.SteadyStateMeanQueue = safeFloat(nc.SteadyStateMeanQueue)
			p.NetworkCouplingData[svc] = nc
		}
	}

	// Sanitize StabilityEnvelope
	p.StabilityEnvelope.SafeSystemRhoMax = safeFloat(p.StabilityEnvelope.SafeSystemRhoMax)
	p.StabilityEnvelope.CurrentSystemRhoMean = safeFloat(p.StabilityEnvelope.CurrentSystemRhoMean)
	p.StabilityEnvelope.EnvelopeHeadroom = safeFloat(p.StabilityEnvelope.EnvelopeHeadroom)
	p.StabilityEnvelope.WorstPerturbationDelta = safeFloat(p.StabilityEnvelope.WorstPerturbationDelta)

	// Sanitize Bundles (embedded structs, must sanitize in-place)
	if p.Bundles != nil {
		for svc, bundle := range p.Bundles {
			if bundle == nil {
				continue
			}
			// Sanitize Queue (embedded)
			bundle.Queue.ArrivalRate = safeFloat(bundle.Queue.ArrivalRate)
			bundle.Queue.ServiceRate = safeFloat(bundle.Queue.ServiceRate)
			bundle.Queue.Utilisation = safeFloat(bundle.Queue.Utilisation)
			bundle.Queue.Confidence = safeFloat(bundle.Queue.Confidence)
			bundle.Queue.Hazard = safeFloat(bundle.Queue.Hazard)
			bundle.Queue.MeanQueueLen = safeFloat(bundle.Queue.MeanQueueLen)
			bundle.Queue.MeanWaitMs = safeFloat(bundle.Queue.MeanWaitMs)
			bundle.Queue.MeanSojournMs = safeFloat(bundle.Queue.MeanSojournMs)
			bundle.Queue.BurstFactor = safeFloat(bundle.Queue.BurstFactor)
			bundle.Queue.AdjustedWaitMs = safeFloat(bundle.Queue.AdjustedWaitMs)
			bundle.Queue.UtilisationTrend = safeFloat(bundle.Queue.UtilisationTrend)
			bundle.Queue.UpstreamPressure = safeFloat(bundle.Queue.UpstreamPressure)
			bundle.Queue.Concurrency = safeFloat(bundle.Queue.Concurrency)
			bundle.Queue.Reservoir = safeFloat(bundle.Queue.Reservoir)

			// Sanitize Stochastic
			bundle.Stochastic.ArrivalCoV = safeFloat(bundle.Stochastic.ArrivalCoV)
			bundle.Stochastic.BurstAmplification = safeFloat(bundle.Stochastic.BurstAmplification)
			bundle.Stochastic.RiskPropagation = safeFloat(bundle.Stochastic.RiskPropagation)
			bundle.Stochastic.Confidence = safeFloat(bundle.Stochastic.Confidence)

			// Sanitize Signal
			bundle.Signal.FastEWMA = safeFloat(bundle.Signal.FastEWMA)
			bundle.Signal.SlowEWMA = safeFloat(bundle.Signal.SlowEWMA)
			bundle.Signal.EWMAVariance = safeFloat(bundle.Signal.EWMAVariance)
			bundle.Signal.CUSUMPos = safeFloat(bundle.Signal.CUSUMPos)
			bundle.Signal.CUSUMNeg = safeFloat(bundle.Signal.CUSUMNeg)

			// Sanitize Stability (embedded)
			bundle.Stability.CollapseRisk = safeFloat(bundle.Stability.CollapseRisk)
			bundle.Stability.OscillationRisk = safeFloat(bundle.Stability.OscillationRisk)
			bundle.Stability.CascadeAmplificationScore = safeFloat(bundle.Stability.CascadeAmplificationScore)
			bundle.Stability.StabilityDerivative = safeFloat(bundle.Stability.StabilityDerivative)
			bundle.Stability.StabilityMargin = safeFloat(bundle.Stability.StabilityMargin)
			bundle.Stability.FeedbackGain = safeFloat(bundle.Stability.FeedbackGain)
			bundle.Stability.PredictedCollapseMs = safeFloat(bundle.Stability.PredictedCollapseMs)
			bundle.Stability.TrendAdjustedMargin = safeFloat(bundle.Stability.TrendAdjustedMargin)

			p.Bundles[svc] = bundle
		}
	}

	// Sanitize Topology node loads
	if p.Topology.Nodes != nil {
		for i, node := range p.Topology.Nodes {
			node.NormalisedLoad = safeFloat(node.NormalisedLoad)
			p.Topology.Nodes[i] = node
		}
	}

	// Sanitize Topology edge weights and latencies
	if p.Topology.Edges != nil {
		for i, edge := range p.Topology.Edges {
			edge.Weight = safeFloat(edge.Weight)
			edge.LatencyMs = safeFloat(edge.LatencyMs)
			edge.ErrorRate = safeFloat(edge.ErrorRate)
			edge.CallRate = safeFloat(edge.CallRate)
			p.Topology.Edges[i] = edge
		}
	}

	// Sanitize Prediction Timeline
	if p.PredictionTimeline != nil {
		for svc, points := range p.PredictionTimeline {
			for i, pt := range points {
				pt.Rho = safeFloat(pt.Rho)
				pt.Lower95 = safeFloat(pt.Lower95)
				pt.Upper95 = safeFloat(pt.Upper95)
				points[i] = pt
			}
			p.PredictionTimeline[svc] = points
		}
	}

	// Sanitize Risk Timeline
	if p.RiskTimeline != nil {
		for svc, points := range p.RiskTimeline {
			for i, pt := range points {
				pt.Rho = safeFloat(pt.Rho)
				pt.CollapseRisk = safeFloat(pt.CollapseRisk)
				points[i] = pt
			}
			p.RiskTimeline[svc] = points
		}
	}

	// Sanitize ScenarioComparison
	if p.ScenarioComparison != nil {
		p.ScenarioComparison.BestCaseCollapse = safeFloat(p.ScenarioComparison.BestCaseCollapse)
		p.ScenarioComparison.WorstCaseCollapse = safeFloat(p.ScenarioComparison.WorstCaseCollapse)
		p.ScenarioComparison.MedianSLAViolation = safeFloat(p.ScenarioComparison.MedianSLAViolation)
		p.ScenarioComparison.StableScenarioFraction = safeFloat(p.ScenarioComparison.StableScenarioFraction)
		p.ScenarioComparison.RecoveryConvergenceMin = safeFloat(p.ScenarioComparison.RecoveryConvergenceMin)
		p.ScenarioComparison.RecoveryConvergenceMax = safeFloat(p.ScenarioComparison.RecoveryConvergenceMax)
	}

	// Sanitize SimOverlay
	if p.SimOverlay != nil {
		p.SimOverlay.HorizonMs = safeFloat(p.SimOverlay.HorizonMs)
		if p.SimOverlay.CascadeFailureProbability != nil {
			for svc, prob := range p.SimOverlay.CascadeFailureProbability {
				p.SimOverlay.CascadeFailureProbability[svc] = safeFloat(prob)
			}
		}
		if p.SimOverlay.P95QueueLen != nil {
			for svc, qlen := range p.SimOverlay.P95QueueLen {
				p.SimOverlay.P95QueueLen[svc] = safeFloat(qlen)
			}
		}
		if p.SimOverlay.SaturationFrac != nil {
			for svc, frac := range p.SimOverlay.SaturationFrac {
				p.SimOverlay.SaturationFrac[svc] = safeFloat(frac)
			}
		}
		if p.SimOverlay.SLAViolationProbability != nil {
			for svc, prob := range p.SimOverlay.SLAViolationProbability {
				p.SimOverlay.SLAViolationProbability[svc] = safeFloat(prob)
			}
		}
	}

	// Sanitize SimResult
	if p.SimResult != nil {
		// Sanitize all float fields in SimResult
		p.SimResult.RecoveryConvergenceMs = safeFloat(p.SimResult.RecoveryConvergenceMs)
		if p.SimResult.CascadeFailureProbability != nil {
			for svc, prob := range p.SimResult.CascadeFailureProbability {
				p.SimResult.CascadeFailureProbability[svc] = safeFloat(prob)
			}
		}
		if p.SimResult.QueueDistributionAtHorizon != nil {
			for svc, dist := range p.SimResult.QueueDistributionAtHorizon {
				dist.MeanQueueLen = safeFloat(dist.MeanQueueLen)
				dist.VarQueueLen = safeFloat(dist.VarQueueLen)
				dist.P95QueueLen = safeFloat(dist.P95QueueLen)
				dist.SaturationFrac = safeFloat(dist.SaturationFrac)
				dist.UtilisationAtEnd = safeFloat(dist.UtilisationAtEnd)
				p.SimResult.QueueDistributionAtHorizon[svc] = dist
			}
		}
		if p.SimResult.SLAViolationProbability != nil {
			for svc, prob := range p.SimResult.SLAViolationProbability {
				p.SimResult.SLAViolationProbability[svc] = safeFloat(prob)
			}
		}
	}

	// Sanitize Events (reasoning.Event may have float fields)
	if p.Events != nil {
		for i, evt := range p.Events {
			evt.Evidence.Utilisation = safeFloat(evt.Evidence.Utilisation)
			evt.Evidence.CollapseRisk = safeFloat(evt.Evidence.CollapseRisk)
			evt.Evidence.OscillationRisk = safeFloat(evt.Evidence.OscillationRisk)
			evt.Evidence.QueueWaitMs = safeFloat(evt.Evidence.QueueWaitMs)
			evt.Evidence.SaturationSec = safeFloat(evt.Evidence.SaturationSec)
			evt.Evidence.BurstFactor = safeFloat(evt.Evidence.BurstFactor)
			evt.Evidence.CascadeRisk = safeFloat(evt.Evidence.CascadeRisk)
			evt.Evidence.StabilityMargin = safeFloat(evt.Evidence.StabilityMargin)
			evt.Evidence.CompositeScore = safeFloat(evt.Evidence.CompositeScore)
			evt.UncertaintyScore = safeFloat(evt.UncertaintyScore)
			p.Events[i] = evt
		}
	}

	// Sanitize RiskQueueItems
	if p.PriorityRiskQueue != nil {
		for i, item := range p.PriorityRiskQueue {
			item.UrgencyScore = safeFloat(item.UrgencyScore)
			item.CollapseRisk = safeFloat(item.CollapseRisk)
			item.Rho = safeFloat(item.Rho)
			item.PathCollapseProb = safeFloat(item.PathCollapseProb)
			p.PriorityRiskQueue[i] = item
		}
	}
}

/* ================= BROADCAST ================= */

func (h *Hub) Broadcast(p *TickPayload) {

	sanitizePayload(p)

	p.SequenceNo = h.seqNo.Add(1)
	p.Timestamp = time.Now()
	p.Schema = SchemaVersion

	// Attempt JSON marshaling with error recovery
	data, err := json.Marshal(p)
	if err != nil {
		errStr := err.Error()
		log.Printf("[hub] marshal error (sanitised payload): %v", errStr)

		// Try to identify which field caused the issue by trying to marshal sub-objects
		if strings.Contains(errStr, "+Inf") || strings.Contains(errStr, "-Inf") {
			log.Printf("[hub] DEBUG: Attempting to identify cause of infinity error")
			// Try marshaling each major component to isolate the infinity
			if _, err := json.Marshal(p.Objective); err != nil {
				log.Printf("[hub] DEBUG: Objective has error: %v", err)
			}
			if _, err := json.Marshal(p.Bundles); err != nil {
				log.Printf("[hub] DEBUG: Bundles has error: %v", err)
			}
			if _, err := json.Marshal(p.Topology); err != nil {
				log.Printf("[hub] DEBUG: Topology has error: %v", err)
			}
			if _, err := json.Marshal(p.RuntimeMetrics); err != nil {
				log.Printf("[hub] DEBUG: RuntimeMetrics has error: %v", err)
			}
		}

		// Fallback: Create a minimal safe payload with only the most critical fields
		minPayload := map[string]interface{}{
			"type":           "tick",
			"seq":            p.SequenceNo,
			"ts":             p.Timestamp,
			"schema_version": p.Schema,
			"bundles":        len(p.Bundles),
			"services":       len(p.Bundles),
			"tick_health_ms": p.TickHealthMs,
			"error":          errStr,
		}
		fallbackData, err2 := json.Marshal(minPayload)
		if err2 != nil {
			log.Printf("[hub] fallback marshal failed: %v", err2)
			return
		}
		data = fallbackData // Use fallback data instead
		log.Printf("[hub] broadcast with minimal fallback payload (%d bytes)", len(fallbackData))
	}

	h.mu.Lock()
	h.lastPayload = p // Cache for REST endpoints
	cs := make([]*client, 0, len(h.clients))
	for c := range h.clients {
		cs = append(cs, c)
	}
	h.mu.Unlock()

	for _, c := range cs {
		if c.closed.Load() {
			continue
		}
		select {
		case <-c.done:
		case c.send <- data:
		default:
			log.Printf("[hub] slow client dropped — backpressure")
			h.remove(c)
		}
	}
}

// GetLastPayload returns the cached TickPayload for REST dashboard queries
func (h *Hub) GetLastPayload() *TickPayload {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastPayload
}

/* ================= UPGRADE ================= */

func (h *Hub) HandleUpgrade(w http.ResponseWriter, r *http.Request) {

	h.mu.RLock()
	count := len(h.clients)
	lastPayload := h.lastPayload
	h.mu.RUnlock()

	if count >= h.maxClients {
		http.Error(w, "hub at capacity", http.StatusServiceUnavailable)
		log.Printf("[hub] rejected upgrade: at capacity (%d/%d)", count, h.maxClients)
		return
	}

	upgrader := &ws.Upgrader{
		ReadBufferSize:  512,
		WriteBufferSize: 1024,
		CheckOrigin:     func(*http.Request) bool { return true },
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[hub] upgrade error: %v", err)
		return
	}

	c := &client{
		hub:  h,
		conn: conn,
		send: make(chan []byte, sendBufferSize),
		done: make(chan struct{}),
	}

	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()

	// Send cached last payload to new client (bootstrap with live data)
	if lastPayload != nil {
		data, err := json.Marshal(lastPayload)
		if err == nil {
			select {
			case c.send <- data:
				log.Printf("[hub] bootstrap payload sent to new client (%d bytes)", len(data))
			default:
				log.Printf("[hub] new client backpressured on bootstrap")
			}
		}
	}

	go c.writePump()
	go c.readPump()
}

func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func (h *Hub) remove(c *client) {
	if !c.closed.CompareAndSwap(false, true) {
		return
	}

	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
	}
	h.mu.Unlock()

	for {
		select {
		case <-c.send:
		default:
			close(c.done)
			go c.conn.Close()
			return
		}
	}
}

/* ================= CLIENT ================= */

type client struct {
	hub    *Hub
	conn   *ws.Conn
	send   chan []byte
	done   chan struct{}
	closed atomic.Bool
}

func (c *client) writePump() {

	ticker := time.NewTicker(pingInterval)
	writesSinceProbe := 0
	defer func() {
		ticker.Stop()
		c.hub.remove(c)
	}()

	for {
		select {
		case <-c.done:
			return
		default:
		}

		select {
		case <-c.done:
			return
		case msg := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if err := c.conn.WriteMessage(ws.TextMessage, msg); err != nil {
				return
			}
			writesSinceProbe++
			if writesSinceProbe == pressureProbeAfter {
				if err := c.writePressureProbe(); err != nil {
					return
				}
				writesSinceProbe = 0
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if err := c.conn.WriteMessage(ws.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *client) writePressureProbe() error {
	deadline := time.Now().Add(pressureProbeWindow)
	for remaining := pressureProbeFrames; remaining > 0; remaining -= pressureProbeChunk {
		n := pressureProbeChunk
		if remaining < n {
			n = remaining
		}
		c.conn.SetWriteDeadline(deadline)
		if err := c.conn.WriteRepeatedMessage(ws.PingMessage, pressureProbePayload[:], n); err != nil {
			return err
		}
	}
	return nil
}

func (c *client) readPump() {
	defer c.hub.remove(c)

	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))

	for {
		mt, _, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		if mt == ws.PongMessage {
			c.conn.SetReadDeadline(time.Now().Add(pongWait))
		}
	}
}
