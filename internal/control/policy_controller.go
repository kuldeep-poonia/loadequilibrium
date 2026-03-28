package control

import "math"

type Controller struct {
	OptimizerCfg OptimizerConfig
	SimCfg       SimConfig
	CostCfg      CostParams
	ActuatorCfg  ActuatorConfig

	ActState *ActuatorState
	Memory   *RegimeMemory

	LastDecision Bundle

	decisionCooldown float64
	lastEnergy       float64
}

func (c *Controller) Tick(
	sys *SystemState,
	backlog float64,
	errorRate float64,
	memPressure float64,
	dt float64,
) {

	if c.ActState == nil {
		c.ActState = &ActuatorState{
			ReplicaActual: float64(sys.Replicas),
			ReplicaWarm:   float64(sys.Replicas),
			QueueActual:   float64(sys.QueueLimit),
			RetryActual:   float64(sys.RetryLimit),
			CacheActual:   sys.CacheAggression,
		}
	}

	if c.Memory == nil {
		c.Memory = NewRegimeMemory()
	}

	// ===== NONLINEAR REGIME ENERGY =====
	util :=
		sys.PredictedArrival /
			(float64(sys.Replicas)*sys.ServiceRate + 0.001)

	queuePressure :=
		backlog / math.Max(1, float64(sys.QueueLimit))

	retryStorm :=
		errorRate * float64(sys.RetryLimit)

	cacheSat :=
		memPressure * sys.CacheAggression

	energy :=
		math.Tanh(
			0.7*util*queuePressure +
				0.4*retryStorm +
				0.2*cacheSat,
		)

	c.Memory.Update(
		SystemState{
			Utilisation: util,
			Risk:        sys.Risk,
			Latency:     sys.Latency,
		},
		sys.SLATarget,
		sys.Risk,
		defaultRegimeConfig(),
	)

	// ===== ACTUATOR SATURATION FEEDBACK =====
	actuatorLag :=
		math.Abs(
			c.ActState.ReplicaWarm -
				float64(sys.Replicas),
		)

	if actuatorLag > 1.5 {
		c.Memory.RiskEWMA =
			math.Min(1,
				c.Memory.RiskEWMA+0.1)
	}

	// ===== ADAPTIVE CADENCE =====
	settleTime :=
		2 + actuatorLag

	if c.Memory.RiskEWMA > 0.6 {
		settleTime *= 0.6
	}

	if c.decisionCooldown > 0 {

		ApplyActuatorDynamics(
			sys,
			c.ActState,
			c.LastDecision,
			c.ActuatorCfg,
			backlog,
			errorRate,
			memPressure,
			dt,
		)

		c.decisionCooldown -= dt
		return
	}

	// ===== ADAPTIVE GENERATOR RADIUS =====
	radius :=
		1 + int(3*c.Memory.RiskEWMA)

	genCfg := GeneratorConfig{
		BaseRadius: radius,
	}

	candidates :=
		GenerateBundles(
			*sys,
			genCfg,
			c.Memory,
		)

	best :=
		SelectBestBundle(
			*sys,
			candidates,
			c.OptimizerCfg,
			c.SimCfg,
			c.CostCfg,
			c.Memory,
		)

	// ===== MODEL-BASED EMERGENCY LAW =====
	if energy > 0.85 {

		requiredCapacity :=
			sys.PredictedArrival * 1.4

		targetReplicas :=
			int(
				requiredCapacity /
					math.Max(sys.ServiceRate, 0.1),
			)

		best.Replicas =
			maxInt(best.Replicas, targetReplicas)

		best.RetryLimit =
			maxInt(1, best.RetryLimit-1)
	}

	c.LastDecision = best

	ApplyActuatorDynamics(
		sys,
		c.ActState,
		best,
		c.ActuatorCfg,
		backlog,
		errorRate,
		memPressure,
		dt,
	)

	// ===== REALISED ENERGY LEARNING =====
	realEnergy :=
		math.Tanh(
			0.5*sys.Risk +
				0.3*(sys.Latency/sys.SLATarget) +
				0.2*(backlog/
					float64(sys.QueueLimit)),
		)

	c.Memory.UpdateCostTrend(
		realEnergy - c.lastEnergy,
	)

	c.lastEnergy = realEnergy

	c.Memory.RecordAction(best)

	c.decisionCooldown = settleTime
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
