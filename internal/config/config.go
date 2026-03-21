package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	ListenAddr          string
	TickInterval        time.Duration
	TickDeadline        time.Duration
	RingBufferDepth     int
	WorkerPoolSize      int
	MaxServices         int
	StaleServiceAge     time.Duration
	DatabaseURL         string
	PersistInterval     time.Duration
	SimBudget           time.Duration
	SimHorizonMs        float64
	SimShockFactor      float64
	SimAsyncBuffer      int
	UtilisationSetpoint float64
	CollapseThreshold   float64
	WindowFraction      float64
	EWMAFastAlpha       float64
	EWMASlowAlpha       float64
	SpikeZScore         float64
	PIDKp               float64
	PIDKi               float64
	PIDKd               float64
	PIDDeadband         float64
	PIDIntegralMax      float64

	// Streaming
	MaxStreamClients int

	// Arrival rate estimation: "ewma" (default) or "median" (burst-resistant)
	ArrivalEstimatorMode string

	// Predictive control horizon in ticks (how many ticks ahead to project)
	PredictiveHorizonTicks int

	// Maximum cooldown map entries in reasoning engine (LRU cap)
	MaxReasoningCooldowns int

	// Simulation inter-arrival distribution: "exponential" (default) or "pareto"
	SimStochasticMode string

	// SafetyModeThreshold: number of consecutive tick overruns before entering
	// safety mode. In safety mode, non-critical stages are skipped and the
	// analysis window is minimised to protect tick cadence.
	SafetyModeThreshold int

	// Adaptive tick cadence bounds.
	// When sustained overruns are detected the engine stretches the tick interval
	// up to MaxTickInterval. It contracts back toward TickInterval when stable.
	MinTickInterval time.Duration // hard minimum — never tick faster than this
	MaxTickInterval time.Duration // hard maximum — never tick slower than this
	TickAdaptStep   float64       // multiplicative stretch per overrun step (e.g. 1.25)

	// StalenessBypassThreshold: system-wide mean staleness score [0,1] above which
	// deep modelling (stochastic, extended stability) is bypassed for this tick.
	StalenessBypassThreshold float64

	// SLALatencyThresholdMs: per-request latency threshold defining SLA compliance.
	// The simulation uses this to compute P(SLA violation) per service.
	// Default 500ms. Set to 0 to disable SLA tracking.
	SLALatencyThresholdMs float64
}

func Load() *Config {
	return &Config{
		ListenAddr:             envStr("LISTEN_ADDR", ":8080"),
		TickInterval:           envDuration("TICK_INTERVAL", 2*time.Second),
		TickDeadline:           envDuration("TICK_DEADLINE", 1800*time.Millisecond),
		RingBufferDepth:        envInt("RING_BUFFER_DEPTH", 300),
		WorkerPoolSize:         envInt("WORKER_POOL_SIZE", 8),
		MaxServices:            envInt("MAX_SERVICES", 200),
		StaleServiceAge:        envDuration("STALE_SERVICE_AGE", 5*time.Minute),
		DatabaseURL:            envStr("DATABASE_URL", ""),
		PersistInterval:        envDuration("PERSIST_INTERVAL", 30*time.Second),
		SimBudget:              envDuration("SIM_BUDGET", 45*time.Millisecond),
		SimHorizonMs:           envFloat("SIM_HORIZON_MS", 60_000),
		SimShockFactor:         envFloat("SIM_SHOCK_FACTOR", 2.0),
		SimAsyncBuffer:         envInt("SIM_ASYNC_BUFFER", 4),
		UtilisationSetpoint:    envFloat("UTILISATION_SETPOINT", 0.70),
		CollapseThreshold:      envFloat("COLLAPSE_THRESHOLD", 0.90),
		WindowFraction:         envFloat("WINDOW_FRACTION", 0.10),
		EWMAFastAlpha:          envFloat("EWMA_FAST_ALPHA", 0.30),
		EWMASlowAlpha:          envFloat("EWMA_SLOW_ALPHA", 0.10),
		SpikeZScore:            envFloat("SPIKE_Z_SCORE", 3.0),
		PIDKp:                  envFloat("PID_KP", 1.5),
		PIDKi:                  envFloat("PID_KI", 0.3),
		PIDKd:                  envFloat("PID_KD", 0.1),
		PIDDeadband:            envFloat("PID_DEADBAND", 0.02),
		PIDIntegralMax:         envFloat("PID_INTEGRAL_MAX", 2.0),
		MaxStreamClients:       envInt("MAX_STREAM_CLIENTS", 50),
		ArrivalEstimatorMode:   envStr("ARRIVAL_ESTIMATOR_MODE", "ewma"),
		PredictiveHorizonTicks: envInt("PREDICTIVE_HORIZON_TICKS", 5),
		MaxReasoningCooldowns:  envInt("MAX_REASONING_COOLDOWNS", 500),
		SimStochasticMode:      envStr("SIM_STOCHASTIC_MODE", "exponential"),
		SafetyModeThreshold:    envInt("SAFETY_MODE_THRESHOLD", 3),
		MinTickInterval:        envDuration("MIN_TICK_INTERVAL", 1*time.Second),
		MaxTickInterval:        envDuration("MAX_TICK_INTERVAL", 10*time.Second),
		TickAdaptStep:          envFloat("TICK_ADAPT_STEP", 1.25),
		StalenessBypassThreshold: envFloat("STALENESS_BYPASS_THRESHOLD", 0.70),
		SLALatencyThresholdMs:    envFloat("SLA_LATENCY_THRESHOLD_MS", 500.0),
	}
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}
func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
