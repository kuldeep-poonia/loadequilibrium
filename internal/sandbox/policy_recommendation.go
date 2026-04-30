package sandbox

import "math"

type RecommendationSignals struct {
	ThroughputMargin float64
	CostGradient     float64
	DegradationRate  float64
}

type PolicyRecommendation struct {
	CapacityDelta   float64
	EfficiencyDelta float64

	DampingDelta       float64
	RetryPressureDelta float64
	BrownoutDelta      float64

	RiskScore  float64
	UrgencyMag float64
	RiskUp     bool

	Confidence float64

	RegimeCollapseProb float64
	RegimeCongestProb  float64
	RegimeIneffProb    float64
}

type RecommendationConfig struct {
	CapacityGain   float64
	EfficiencyGain float64

	DampingGain  float64
	RetryGain    float64
	BrownoutGain float64

	RiskCollapseW  float64
	RiskInteractW  float64
	RiskViabilityW float64

	SLA_CollapseRef   float64
	SLA_InteractRef   float64
	SLA_MinThroughput float64

	RiskThreshold float64

	TrendGain float64

	SoftmaxTemp float64
}

func RecommendPolicy(
	comp ComparisonResult,
	sig RecommendationSignals,
	cfg RecommendationConfig,
) PolicyRecommendation {

	// ----- throughput viability deficit (real margin) -----
	vDef :=
		math.Max(
			0,
			-sig.ThroughputMargin/
				(cfg.SLA_MinThroughput+1e-6),
		)

	cNorm :=
		comp.CollapseEnergy /
			(cfg.SLA_CollapseRef + 1e-6)

	iNorm :=
		comp.InteractionRisk /
			(cfg.SLA_InteractRef + 1e-6)

	risk :=
		math.Sqrt(
			cfg.RiskCollapseW*cNorm*cNorm +
				cfg.RiskInteractW*iNorm*iNorm +
				cfg.RiskViabilityW*vDef*vDef,
		)

	// ----- bounded trend shaping -----
	trendShape :=
		math.Tanh(
			cfg.TrendGain *
				sig.CostGradient,
		)

	capAdj :=
		cfg.CapacityGain *
			math.Tanh(risk) *
			(1 + trendShape)

	// ----- arbitration considers congestion trend -----
	effAdj := 0.0

	if risk < cfg.RiskThreshold &&
		iNorm < 0.7 {

		effAdj =
			cfg.EfficiencyGain *
				math.Tanh(
					comp.UtilityScore-risk,
				)
	}

	dampAdj :=
		cfg.DampingGain *
			math.Tanh(
				(1-comp.TemporalRobustness)+
					iNorm,
			)

	// retry reacts to both collapse and congestion coupling
	retryAdj :=
		cfg.RetryGain *
			math.Tanh(
				cNorm+0.7*iNorm,
			)

	brownAdj :=
		cfg.BrownoutGain *
			math.Tanh(
				math.Max(
					0,
					-comp.SafetyMarg+
						0.5*iNorm,
				),
			)

	// ----- urgency includes degradation speed -----
	urg :=
		math.Tanh(
			risk+
				0.5*math.Abs(sig.DegradationRate),
		) *
			comp.Confidence

	pCollapse, pCongest, pIneff :=
		regimeSoftmaxTemp(
			cNorm,
			iNorm,
			vDef,
			cfg.SoftmaxTemp,
		)

	return PolicyRecommendation{

		CapacityDelta:   capAdj,
		EfficiencyDelta: effAdj,

		DampingDelta:       dampAdj,
		RetryPressureDelta: retryAdj,
		BrownoutDelta:      brownAdj,

		RiskScore:  risk,
		UrgencyMag: urg,
		RiskUp:     risk > cfg.RiskThreshold,

		Confidence: comp.Confidence,

		RegimeCollapseProb: pCollapse,
		RegimeCongestProb:  pCongest,
		RegimeIneffProb:    pIneff,
	}
}

func regimeSoftmaxTemp(
	c, i, v, T float64,
) (float64, float64, float64) {

	if T <= 0 {
		T = 1
	}

	e1 := math.Exp(c / T)
	e2 := math.Exp(i / T)
	e3 := math.Exp(v / T)

	s := e1 + e2 + e3

	return e1 / s, e2 / s, e3 / s
}
