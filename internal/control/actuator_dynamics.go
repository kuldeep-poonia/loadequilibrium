package control

import "math"

type ActuatorState struct {
	ReplicaTarget int
	ReplicaActual float64
	ReplicaWarm   float64
	ScaleCooldown float64

	QueueTarget   int
	QueueActual   float64
	QueueLag      float64
	QueueCooldown float64

	RetryTarget int
	RetryActual float64

	CacheTarget float64
	CacheActual float64
}

type ActuatorConfig struct {
	MinReplicas      int
	MaxReplicas      int
	MaxScaleRate     float64
	ScaleCooldownSec float64
	WarmupRate       float64

	MinQueue         int
	MaxQueue         int
	MaxQueueRate     float64
	QueueLagTau      float64
	QueueCooldownSec float64

	RetryRate            float64
	RetryDisturbanceGain float64
	MinRetry             int
	MaxRetry             int

	CacheRate            float64
	CacheMemPressureGain float64
}

func ApplyActuatorDynamics(
	sys *SystemState,
	act *ActuatorState,
	cmd Bundle,
	cfg ActuatorConfig,
	backlog float64,
	errorRate float64,
	memPressure float64,
	dt float64,
) {
	cfg = normalizeActuatorConfig(*sys, cmd, cfg)

	// ===== SCALING WITH WARMUP EFFECTIVENESS =====

	act.ReplicaTarget = clampInt(cmd.Replicas, cfg.MinReplicas, cfg.MaxReplicas)

	diff :=
		float64(act.ReplicaTarget) - act.ReplicaActual

	step :=
		clampFloat(diff,
			-cfg.MaxScaleRate*dt,
			cfg.MaxScaleRate*dt)

	act.ReplicaActual += step

	// warmup effectiveness lag
	warmDiff :=
		act.ReplicaActual - act.ReplicaWarm

	act.ReplicaWarm +=
		cfg.WarmupRate * warmDiff * dt

	if math.Abs(diff) < 0.05 &&
		act.ScaleCooldown <= 0 {

		act.ScaleCooldown = cfg.ScaleCooldownSec
	}

	act.ScaleCooldown =
		math.Max(0, act.ScaleCooldown-dt)

	act.ReplicaActual =
		clampFloat(
			act.ReplicaActual,
			float64(cfg.MinReplicas),
			float64(cfg.MaxReplicas),
		)

	// ===== QUEUE CONTROL WITH PROPAGATION LAG =====

	cmdQueue :=
		clampInt(int(math.Round(cmd.QueueLimit)),
			cfg.MinQueue,
			cfg.MaxQueue)

	act.QueueTarget = cmdQueue

	// first-order lag on policy propagation
	act.QueueLag +=
		(dt / math.Max(cfg.QueueLagTau, 0.001)) *
			(float64(act.QueueTarget) - act.QueueLag)

	safety :=
		1.0 /
			(1.0 + backlog/math.Max(act.QueueActual, 1))

	qdiff :=
		act.QueueLag - act.QueueActual

	qstep :=
		clampFloat(
			qdiff,
			-cfg.MaxQueueRate*dt,
			cfg.MaxQueueRate*dt*safety,
		)

	act.QueueActual += qstep

	act.QueueCooldown =
		math.Max(0, act.QueueCooldown-dt)

	act.QueueActual =
		clampFloat(
			act.QueueActual,
			float64(cfg.MinQueue),
			float64(cfg.MaxQueue),
		)

	// ===== RETRY WITH FAILURE DISTURBANCE =====

	act.RetryTarget =
		clampInt(cmd.RetryLimit,
			cfg.MinRetry,
			cfg.MaxRetry)

	rdiff :=
		float64(act.RetryTarget) -
			act.RetryActual

	disturb :=
		cfg.RetryDisturbanceGain *
			errorRate

	act.RetryActual +=
		(cfg.RetryRate*rdiff + disturb) *
			dt

	act.RetryActual =
		clampFloat(
			act.RetryActual,
			float64(cfg.MinRetry),
			float64(cfg.MaxRetry),
		)

	// ===== CACHE WITH MEMORY PRESSURE =====

	act.CacheTarget =
		clampFloat(cmd.CacheAggression, 0, 1)

	cdiff :=
		act.CacheTarget - act.CacheActual

	rate :=
		cfg.CacheRate /
			(1 + cfg.CacheMemPressureGain*
				memPressure)

	act.CacheActual +=
		rate * cdiff * dt

	act.CacheActual =
		clampFloat(act.CacheActual, 0, 1)

	// ===== WRITEBACK =====

	act.ReplicaActual = math.Max(1, act.ReplicaActual)

	// 🔥 FIX: remove deadband for scaling
	sys.Replicas = maxInt(1, int(math.Round(act.ReplicaActual)))
	sys.QueueLimit =
		hysteresisRound(
			sys.QueueLimit,
			act.QueueActual,
			1.2,
		)

	sys.RetryLimit =
		hysteresisRound(
			sys.RetryLimit,
			act.RetryActual,
			0.6,
		)

	sys.CacheAggression =
		act.CacheActual
}

func normalizeActuatorConfig(sys SystemState, cmd Bundle, cfg ActuatorConfig) ActuatorConfig {
	if cfg.MinReplicas <= 0 {
		cfg.MinReplicas = maxInt(1, sys.MinReplicas)
	}
	if cfg.MaxReplicas < cfg.MinReplicas {
		cfg.MaxReplicas = maxInt(maxInt(sys.MaxReplicas, cmd.Replicas), cfg.MinReplicas)
	}
	if cfg.MaxScaleRate <= 0 {
		cfg.MaxScaleRate = math.Max(1, float64(cfg.MaxReplicas-cfg.MinReplicas))
	}
	if cfg.WarmupRate <= 0 {
		cfg.WarmupRate = 1
	}
	if cfg.MinQueue <= 0 {
		cfg.MinQueue = 1
	}
	if cfg.MaxQueue < cfg.MinQueue {
		cfg.MaxQueue = maxInt(maxInt(sys.QueueLimit*2, int(math.Round(cmd.QueueLimit))), cfg.MinQueue)
	}
	if cfg.MaxQueueRate <= 0 {
		cfg.MaxQueueRate = math.Max(1, float64(cfg.MaxQueue-cfg.MinQueue))
	}
	if cfg.QueueLagTau <= 0 {
		cfg.QueueLagTau = 1
	}
	if cfg.MinRetry <= 0 {
		cfg.MinRetry = 1
	}
	if cfg.MaxRetry < cfg.MinRetry {
		cfg.MaxRetry = maxInt(maxInt(sys.MaxRetry, cmd.RetryLimit), cfg.MinRetry)
	}
	if cfg.RetryRate <= 0 {
		cfg.RetryRate = 1
	}
	if cfg.CacheRate <= 0 {
		cfg.CacheRate = 1
	}
	return cfg
}

func clampFloat(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, v))
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func hysteresisRound(current int,
	value float64,
	band float64) int {

	if math.Abs(float64(current)-value) < band {
		return current
	}

	return int(math.Round(value))
}
