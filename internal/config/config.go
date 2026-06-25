package config

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	// ── HTTP server ────────────────────────────────────────────────────────────
	ListenAddr string
	TLSEnabled bool // set true when TLS terminates at this process (not at proxy)

	// ── Tick engine ───────────────────────────────────────────────────────────
	TickInterval        time.Duration
	TickDeadline        time.Duration
	MinTickInterval     time.Duration
	MaxTickInterval     time.Duration
	TickAdaptStep       float64
	WorkerPoolSize      int
	SafetyModeThreshold int

	// ── Telemetry store ───────────────────────────────────────────────────────
	RingBufferDepth int
	MaxServices     int
	StaleServiceAge time.Duration

	// ── Persistence ───────────────────────────────────────────────────────────
	DatabaseURL     string
	PersistInterval time.Duration

	// ── Simulation ────────────────────────────────────────────────────────────
	SimBudget         time.Duration
	SimHorizonMs      float64
	SimShockFactor    float64
	SimAsyncBuffer    int
	SimStochasticMode string

	// ── Control policy ────────────────────────────────────────────────────────
	UtilisationSetpoint      float64
	CollapseThreshold        float64
	WindowFraction           float64
	EWMAFastAlpha            float64
	EWMASlowAlpha            float64
	SpikeZScore              float64
	PIDKp                    float64
	PIDKi                    float64
	PIDKd                    float64
	PIDDeadband              float64
	PIDIntegralMax           float64
	ArrivalEstimatorMode     string
	PredictiveHorizonTicks   int
	MaxReasoningCooldowns    int
	StalenessBypassThreshold float64
	SLALatencyThresholdMs    float64
	ScenarioMode             string

	// ── Streaming ─────────────────────────────────────────────────────────────
	MaxStreamClients int

	// ══════════════════════════════════════════════════════════════════════════
	// SECURITY FIELDS
	// ══════════════════════════════════════════════════════════════════════════

	// IngestToken is required on all POST /api/v1/ingest requests.
	// Used by the embedded collector and external push agents.
	// REQUIRED in production. Leave empty only in isolated development environments.
	IngestToken string

	// ControlToken protects all control-plane write endpoints:
	//   POST /api/v1/control/*  /api/v1/policy/*  /api/v1/runtime/*
	//   POST /api/v1/sandbox/*  /api/v1/simulation/*  /api/v1/intelligence/*
	//   POST /api/v1/alerts/ack
	//   GET  /api/v1/audit
	//
	// If empty, falls back to IngestToken (single-token mode).
	// RECOMMENDED: set a DIFFERENT value from IngestToken to enforce least-privilege.
	// Collector agents only need IngestToken. Only human operators need ControlToken.
	ControlToken string

	// DashboardToken protects GET /ws (WebSocket) and GET /api/v1/snapshot.
	// If empty, dashboard is readable without auth (acceptable for internal networks
	// behind a VPN or firewall). Set it when the dashboard is internet-accessible.
	DashboardToken string

	// AllowedOrigins is the CORS allowlist. Examples:
	//   "https://dashboard.example.com,https://ops.example.com"
	// Empty = allow all origins (acceptable for internal deployments).
	// REQUIRED when the API is internet-accessible to prevent CSRF.
	AllowedOrigins []string

	// IngestRPS is the sustained ingest rate limit per client IP (requests/second).
	// Burst is automatically 3× this value.
	// Default: 5000 rps (high — collector batches many services per request)
	IngestRPS float64

	// ControlRPS is the sustained control API rate limit per client IP.
	// Default: 10 rps (low — control actions are human-issued, not scripted)
	ControlRPS float64
}

func Load() *Config {
	return &Config{
		ListenAddr:          envStr("LISTEN_ADDR", ":8080"),
		TLSEnabled:          envBool("TLS_ENABLED", false),
		TickInterval:        envDuration("TICK_INTERVAL", 2*time.Second),
		TickDeadline:        envDuration("TICK_DEADLINE", 1800*time.Millisecond),
		MinTickInterval:     envDuration("MIN_TICK_INTERVAL", 1*time.Second),
		MaxTickInterval:     envDuration("MAX_TICK_INTERVAL", 10*time.Second),
		TickAdaptStep:       envFloat("TICK_ADAPT_STEP", 1.25),
		WorkerPoolSize:      envInt("WORKER_POOL_SIZE", defaultWorkers()),
		SafetyModeThreshold: envInt("SAFETY_MODE_THRESHOLD", 3),

		RingBufferDepth: envInt("RING_BUFFER_DEPTH", 150),
		MaxServices:     envInt("MAX_SERVICES", 2000),
		StaleServiceAge: envDuration("STALE_SERVICE_AGE", 5*time.Minute),

		DatabaseURL:     envStr("DATABASE_URL", ""),
		PersistInterval: envDuration("PERSIST_INTERVAL", 30*time.Second),

		SimBudget:         envDuration("SIM_BUDGET", 45*time.Millisecond),
		SimHorizonMs:      envFloat("SIM_HORIZON_MS", 60_000),
		SimShockFactor:    envFloat("SIM_SHOCK_FACTOR", 2.0),
		SimAsyncBuffer:    envInt("SIM_ASYNC_BUFFER", 16),
		SimStochasticMode: envStr("SIM_STOCHASTIC_MODE", "exponential"),

		UtilisationSetpoint:      envFloat("UTILISATION_SETPOINT", 0.70),
		CollapseThreshold:        envFloat("COLLAPSE_THRESHOLD", 0.90),
		WindowFraction:           envFloat("WINDOW_FRACTION", 0.10),
		EWMAFastAlpha:            envFloat("EWMA_FAST_ALPHA", 0.30),
		EWMASlowAlpha:            envFloat("EWMA_SLOW_ALPHA", 0.10),
		SpikeZScore:              envFloat("SPIKE_Z_SCORE", 3.0),
		PIDKp:                    envFloat("PID_KP", -1.5),
		PIDKi:                    envFloat("PID_KI", -0.3),
		PIDKd:                    envFloat("PID_KD", -0.1),
		PIDDeadband:              envFloat("PID_DEADBAND", 0.02),
		PIDIntegralMax:           envFloat("PID_INTEGRAL_MAX", 2.0),
		ArrivalEstimatorMode:     envStr("ARRIVAL_ESTIMATOR_MODE", "ewma"),
		PredictiveHorizonTicks:   envInt("PREDICTIVE_HORIZON_TICKS", 5),
		MaxReasoningCooldowns:    envInt("MAX_REASONING_COOLDOWNS", 500),
		StalenessBypassThreshold: envFloat("STALENESS_BYPASS_THRESHOLD", 0.70),
		SLALatencyThresholdMs:    envFloat("SLA_LATENCY_THRESHOLD_MS", 500.0),
		ScenarioMode:             envStr("SCENARIO_MODE", "off"),

		MaxStreamClients: envInt("MAX_STREAM_CLIENTS", 200),

		// ── Security ──────────────────────────────────────────────────────────
		IngestToken:    envStr("INGEST_TOKEN", ""),
		ControlToken:   envStr("CONTROL_TOKEN", ""),
		DashboardToken: envStr("DASHBOARD_TOKEN", ""),
		AllowedOrigins: envStringSlice("ALLOWED_ORIGINS", nil),
		IngestRPS:      envFloat("INGEST_RATE_LIMIT_RPS", 5000),
		ControlRPS:     envFloat("CONTROL_RATE_LIMIT_RPS", 10),
	}
}

func defaultWorkers() int {
	n := runtime.NumCPU()
	if n < 4 {
		return 4
	}
	if n > 64 {
		return 64
	}
	return n
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
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

func envStringSlice(key string, def []string) []string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}
