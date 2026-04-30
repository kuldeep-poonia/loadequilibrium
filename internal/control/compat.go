package control

import "math"

type ActionBundle = Bundle

type ActionBounds struct {
	MinReplicas int
	MaxReplicas int
	MinQueue    int
	MaxQueue    int
	MinRetry    int
	MaxRetry    int
	MinCache    float64
	MaxCache    float64
}

type SystemState struct {
	Replicas         int
	QueueLimit       int
	RetryLimit       int
	CacheAggression  float64
	QueueDepth       float64
	PredictedArrival float64
	ArrivalRate      float64
	ServiceRate      float64
	Latency          float64
	Risk             float64
	Utilisation      float64
	SLATarget        float64
	MinReplicas      int
	MaxReplicas      int
	MinRetry         int
	MaxRetry         int
}

type GeneratorConfig struct {
	BaseRadius int
	Seed       int64
}

type BundleConfig struct {
	ReplicaRadius      int
	QueueRadius        int
	CacheRadius        int
	MaxScaleStep       int
	MinReplicas        int
	MaxReplicas        int
	QueueStep          float64
	MinQueue           float64
	MaxQueue           float64
	MinRetry           int
	MaxRetry           int
	CacheStep          float64
	MinCache           float64
	MaxCache           float64
	RetryAmplification float64
	EfficiencyDecay    float64
	TargetUtil         float64
	QueueWeight        float64
	ReplicaMovePenalty float64
	QueueMovePenalty   float64
	RetryMovePenalty   float64
	CacheMovePenalty   float64
	MinBundleDistance  float64
	TopK               int
	GenerationKeepProb float64
	Seed               int64
}

type SimConfig struct {
	HorizonSteps      int
	BaseLatency       float64
	Dt                float64
	DisturbanceStd    float64
	DisturbanceFreq   float64
	RetryFeedbackGain float64
	WarmupRate        float64
	EfficiencyDecay   float64
	MaxQueueDelay     float64
	HazardUtilGain    float64
	HazardBacklogGain float64
	HazardRetryGain   float64
	Seed              int64
}

func NewRegimeMemory() *RegimeMemory {
	return &RegimeMemory{
		Regime:     RegimeCalm,
		LastAction: ActionBundle{},
	}
}

func (r *RegimeMemory) UpdateCostTrend(delta float64) {
	r.CostTrendEWMA = 0.8*r.CostTrendEWMA + 0.2*delta
}

func (r *RegimeMemory) RecordAction(next ActionBundle) {
	dist := math.Abs(float64(next.Replicas-r.LastAction.Replicas)) +
		0.2*math.Abs(next.QueueLimit-r.LastAction.QueueLimit) +
		0.5*math.Abs(float64(next.RetryLimit-r.LastAction.RetryLimit)) +
		math.Abs(next.CacheAggression-r.LastAction.CacheAggression)

	r.OscillationEWMA = 0.85*r.OscillationEWMA + 0.15*dist
	r.LastAction = next
}

func defaultRegimeConfig() RegimeConfig {
	return RegimeConfig{
		EWMAAlpha:        0.20,
		HistorySize:      64,
		BaseUtilStress:   0.70,
		BaseRiskUnstable: 0.60,
		HysteresisMargin: 0.05,
	}
}

func GenerateBundles(
	current SystemState,
	cfg GeneratorConfig,
	mem *RegimeMemory,
) []Bundle {
	radius := cfg.BaseRadius
	if radius <= 0 {
		radius = 1
	}

	queueLimit := current.QueueLimit
	if queueLimit <= 0 {
		queueLimit = 10
	}

	return GenerateLocalBundles(current, BundleConfig{
		ReplicaRadius:      radius,
		QueueRadius:        radius,
		CacheRadius:        radius,
		MaxScaleStep:       radius,
		MinReplicas:        maxInt(current.MinReplicas, 1),
		MaxReplicas:        maxInt(current.MaxReplicas, maxInt(current.MinReplicas, current.Replicas+radius+1)),
		QueueStep:          math.Max(float64(queueLimit)*0.1, 1),
		MinQueue:           math.Max(1, float64(queueLimit)*0.5),
		MaxQueue:           math.Max(float64(queueLimit)*1.5, float64(queueLimit+1)),
		MinRetry:           maxInt(current.MinRetry, 1),
		MaxRetry:           maxInt(current.MaxRetry, maxInt(current.RetryLimit+radius, 3)),
		CacheStep:          0.1,
		MinCache:           0,
		MaxCache:           1,
		RetryAmplification: 0.2,
		EfficiencyDecay:    0.15,
		TargetUtil:         0.70,
		QueueWeight:        0.30,
		ReplicaMovePenalty: 0.15,
		QueueMovePenalty:   0.10,
		RetryMovePenalty:   0.08,
		CacheMovePenalty:   0.10,
		MinBundleDistance:  0.30,
		TopK:               8,
		GenerationKeepProb: 0.75,
		Seed:               cfg.Seed,
	}, mem)
}
