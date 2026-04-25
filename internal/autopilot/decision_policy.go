package autopilot

import (
	"math"
	"strconv"
)

// Decision is the output of the policy layer.
type Decision struct {
	// Action ∈ {"scale_up", "scale_down", "hold", "emergency"}.
	Action string

	// ScaleDelta ∈ [0,1]: normalized response magnitude.
	//
	
	ScaleDelta float64

	Urgency float64
	Reason  string

	// Mode ∈ {"normal", "cautious", "fallback"}.
	Mode string
}

// Decision Input is the policy input vector
type DecisionInput struct {
	Instability    float64
	Confidence     float64
	Anomaly        AnomalyType
	Backlog        float64
	Workers        float64
	TargetCapacity float64
	Effectiveness  float64
	Oscillation    float64
	Trend          float64
}

func Decide(in DecisionInput) Decision {

	inst := clamp01(in.Instability)
	conf := clamp01(in.Confidence)
	backlog := pos(in.Backlog)
	workers := math.Max(1.0, in.Workers)

	// 1. GAP CALCULATION (TARGET TRACKING)
	gap := in.TargetCapacity - workers

	// 2. BASE SCALING CURVE (Continuous smooth function, no hard thresholds)
	absGap := math.Abs(gap)
	
	rateMultiplier := 1.0
	if backlog > 0.0 {
		rateMultiplier *= 1.5 // accelerate if there is backlog
	}
	
	baseDelta := (absGap / (absGap + 2.0)) * rateMultiplier 

	// 3. MEMORY INTEGRATION
	memFactor := (1.0 + 0.6*in.Effectiveness) * (1.0 - in.Oscillation)
	if in.Trend > 0 {
		memFactor *= (1.0 + in.Trend)
	}

	// 4. CONFIDENCE FIX (Controls speed, not disabling action)
	speedFactor := 0.2 + 0.8*conf

	delta := baseDelta * memFactor * speedFactor
	if delta > 1.0 {
		delta = 1.0
	}

	// 5. SELECT ACTION
	var action string
	if gap > 0.05 {
		action = "scale_up"
	} else if gap < -0.05 {
		action = "scale_down"
	} else {
		action = "hold"
		delta = 0.0
	}

	// 6. NO FREEZE RULE & ALWAYS ACT RULE
	if action == "hold" {
		if backlog > 0 || in.Trend > 0.1 || inst > 0.5 {
			action = "scale_up"
			delta = 0.02
		} else if backlog == 0 && inst < 0.3 && in.Trend <= 0 {
			// system is at equilibrium — hold truly still
			action = "hold"
			delta = 0.0
		} else {
			action = "scale_down"
			delta = 0.01
		}
	}else {
		// Minimum action if not explicitly holding
		if delta < 0.01 {
			delta = 0.01
		}
	}

	// Determine operational mode purely on confidence
	mode := "normal"
	if conf < 0.5 {
		mode = "cautious"
	}
	if conf < 0.2 {
		mode = "fallback"
	}

	urgency := (inst * norm(backlog)) / (1.0 + inst + norm(backlog))

	return Decision{
		Action:     action,
		ScaleDelta: clamp01(delta),
		Urgency:    clamp01(urgency),
		Reason:     buildReason(action, in.Anomaly, inst, conf, gap),
		Mode:       mode,
	}
}

func anomalyFactor(a AnomalyType) float64 {
	switch a {
	case Cascade:
		return 1.0
	case Systemic:
		return 0.7
	case Local:
		return 0.4
	default: // Stable
		return 0.2
	}
}

func buildReason(action string, anomaly AnomalyType, inst, conf, gap float64) string {
	return action +
		" | anomaly=" + string(anomaly) +
		" | instability=" + floatToStr(inst) +
		" | confidence=" + floatToStr(conf) +
		" | gap=" + floatToStr(gap)
}

func floatToStr(x float64) string {
	return strconv.FormatFloat(x, 'f', 3, 64)
}