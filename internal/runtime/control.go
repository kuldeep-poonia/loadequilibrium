package runtime

import (
	"errors"
	"strings"
	"sync/atomic"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/reasoning"
	"github.com/loadequilibrium/loadequilibrium/internal/streaming"
)

var ErrTickInFlight = errors.New("runtime tick already in flight")

func (o *Orchestrator) StepOnce(now time.Time) (uint64, error) {
	if o == nil {
		return 0, errors.New("runtime orchestrator offline")
	}
	if !atomic.CompareAndSwapUint32(&o.inFlightTicks, 0, 1) {
		atomic.AddUint64(&o.reentrantTicks, 1)
		return o.TickCount(), ErrTickInFlight
	}
	defer atomic.StoreUint32(&o.inFlightTicks, 0)

	o.lastTickScheduledAt = now
	o.executeTick(now)
	return o.TickCount(), nil
}

func (o *Orchestrator) executeTick(scheduled time.Time) {
	o.tick(scheduled)
	atomic.AddUint64(&o.processedTicks, 1)
	o.adaptInterval()
}

func (o *Orchestrator) SetActuationEnabled(enabled bool) bool {
	if o == nil {
		return false
	}
	o.actuationEnabled.Store(enabled)
	return enabled
}

func (o *Orchestrator) ToggleActuation() bool {
	if o == nil {
		return false
	}
	for {
		current := o.actuationEnabled.Load()
		if o.actuationEnabled.CompareAndSwap(current, !current) {
			return !current
		}
	}
}

func (o *Orchestrator) ActuationEnabled() bool {
	if o == nil {
		return false
	}
	return o.actuationEnabled.Load()
}

func (o *Orchestrator) SetPolicyPreset(preset string) string {
	if o == nil || o.phaseRuntime == nil {
		return "balanced"
	}
	return o.phaseRuntime.SetPolicyPreset(preset)
}

func (o *Orchestrator) ForceSandbox(durationTicks uint64) uint64 {
	if o == nil || o.phaseRuntime == nil {
		return 0
	}
	until := o.untilTick(durationTicks)
	o.phaseRuntime.ForceSandboxUntil(until)
	return until
}

func (o *Orchestrator) ForceIntelligenceRollout(durationTicks uint64) uint64 {
	if o == nil || o.phaseRuntime == nil {
		return 0
	}
	until := o.untilTick(durationTicks)
	o.phaseRuntime.ForceIntelligenceUntil(until)
	return until
}

func (o *Orchestrator) ForceSimulation(durationTicks uint64) uint64 {
	if o == nil {
		return 0
	}
	if durationTicks == 0 {
		o.forceSimulationUntil.Store(0)
		return 0
	}
	until := o.untilTick(durationTicks)
	o.forceSimulationUntil.Store(until)
	return until
}

func (o *Orchestrator) RequestSimulationReset() {
	if o == nil {
		return
	}
	o.resetSimulationRequested.Store(true)
}

func (o *Orchestrator) AcknowledgeAlert(alertID string, now time.Time) (int, bool) {
	if o == nil {
		return 0, false
	}
	alertID = strings.TrimSpace(alertID)
	if alertID == "" {
		return o.acknowledgedAlertCount(), false
	}

	o.alertMu.Lock()
	defer o.alertMu.Unlock()
	if o.ackedAlerts == nil {
		o.ackedAlerts = make(map[string]time.Time)
	}
	o.ackedAlerts[alertID] = now
	return len(o.ackedAlerts), true
}

func (o *Orchestrator) ControlState() streaming.ControlPlaneState {
	if o == nil {
		return streaming.ControlPlaneState{}
	}
	state := streaming.ControlPlaneState{
		Tick:                   o.TickCount(),
		ActuationEnabled:       o.ActuationEnabled(),
		ForcedSimulationUntil:  o.forceSimulationUntil.Load(),
		SimulationResetPending: o.resetSimulationRequested.Load(),
		AcknowledgedAlertCount: o.acknowledgedAlertCount(),
	}
	if o.phaseRuntime != nil {
		state.PolicyPreset = o.phaseRuntime.PolicyPreset()
		state.ForcedSandboxUntil = o.phaseRuntime.ForcedSandboxUntil()
		state.ForcedIntelligenceUntil = o.phaseRuntime.ForcedIntelligenceUntil()
	}
	if state.PolicyPreset == "" {
		state.PolicyPreset = "balanced"
	}
	return state
}

func (o *Orchestrator) isSimulationForcedAt(tick uint64) bool {
	if o == nil {
		return false
	}
	until := o.forceSimulationUntil.Load()
	return until > 0 && tick <= until
}

func (o *Orchestrator) consumeSimulationReset() bool {
	if o == nil {
		return false
	}
	return o.resetSimulationRequested.Swap(false)
}

func (o *Orchestrator) filterAcknowledgedEvents(events []reasoning.Event) []reasoning.Event {
	if o == nil || len(events) == 0 || o.acknowledgedAlertCount() == 0 {
		return events
	}
	o.alertMu.RLock()
	defer o.alertMu.RUnlock()
	if len(o.ackedAlerts) == 0 {
		return events
	}

	filtered := events[:0]
	for _, event := range events {
		if event.ID == "" {
			filtered = append(filtered, event)
			continue
		}
		if _, acknowledged := o.ackedAlerts[event.ID]; !acknowledged {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

func (o *Orchestrator) acknowledgedAlertCount() int {
	if o == nil {
		return 0
	}
	o.alertMu.RLock()
	defer o.alertMu.RUnlock()
	return len(o.ackedAlerts)
}

func (o *Orchestrator) untilTick(durationTicks uint64) uint64 {
	current := o.TickCount()
	if durationTicks == 0 {
		durationTicks = 1
	}
	return current + durationTicks
}
