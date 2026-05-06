package reasoning

import "time"

// Severity orders events from informational to critical.
type Severity int

const (
	SeverityInfo     Severity = iota
	SeverityWarning
	SeverityCritical
)

func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "INFO"
	case SeverityWarning:
		return "WARN"
	case SeverityCritical:
		return "CRIT"
	default:
		return "UNKN"
	}
}

// Event is a single structured reasoning output with cause→model→prediction→action chain.
type Event struct {
	ID                 string    `json:"id"`
	Timestamp          time.Time `json:"timestamp"`
	ServiceID          string    `json:"service_id,omitempty"`
	Severity           Severity  `json:"severity"`
	Category           string    `json:"category"`
	Description        string    `json:"description"`
	Recommendation     string    `json:"recommendation,omitempty"`
	Evidence           Evidence  `json:"evidence"`
	// Intelligence fields
	UncertaintyScore   float64   `json:"uncertainty_score"`   // 0=certain, 1=highly uncertain
	OperationalPriority int      `json:"operational_priority"` // 0..9
	ModelChain         string    `json:"model_chain,omitempty"` // cause→model→prediction→action
}

// Evidence is the quantified supporting data for an event.
type Evidence struct {
	Utilisation     float64 `json:"utilisation,omitempty"`
	CollapseRisk    float64 `json:"collapse_risk,omitempty"`
	OscillationRisk float64 `json:"oscillation_risk,omitempty"`
	QueueWaitMs     float64 `json:"queue_wait_ms,omitempty"`
	SaturationSec   float64 `json:"saturation_sec,omitempty"`
	BurstFactor     float64 `json:"burst_factor,omitempty"`
	CascadeRisk     float64 `json:"cascade_risk,omitempty"`
	StabilityMargin float64 `json:"stability_margin,omitempty"`
	CompositeScore  float64 `json:"composite_score,omitempty"`
}
