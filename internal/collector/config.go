package collector

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddr           string
	DockerHost           string
	DiscoveryInterval    time.Duration
	ScrapeInterval       time.Duration
	ScrapeTimeout        time.Duration
	IngestURL            string
	IngestToken          string
	QueueSize            int
	BatchMaxPoints       int
	BatchMaxAge          time.Duration
	FlushTimeout         time.Duration
	MaxRetries           int
	RetryBaseDelay       time.Duration
	RetryMaxDelay        time.Duration
	CircuitOpenAfter     int
	CircuitCooldown      time.Duration
	MaxScrapeConcurrency int
	MaxScrapeBodyBytes   int64
	RequestRateLimitRPS  float64
	DiscoveryLabel       string
	DefaultMetricsPath   string
}

func LoadConfig() Config {
	return Config{
		ListenAddr:           envStr("LE_COLLECTOR_LISTEN_ADDR", ":9091"),
		DockerHost:           envStr("DOCKER_HOST", "unix:///var/run/docker.sock"),
		DiscoveryInterval:    envDuration("LE_DISCOVERY_INTERVAL", 15*time.Second),
		ScrapeInterval:       envDuration("LE_SCRAPE_INTERVAL", 2*time.Second),
		ScrapeTimeout:        envDuration("LE_SCRAPE_TIMEOUT", 1500*time.Millisecond),
		IngestURL:            envStr("LE_INGEST_URL", "http://loadequilibrium:8080/api/v1/ingest"),
		IngestToken:          envStr("INGEST_TOKEN", ""),
		QueueSize:            envInt("LE_COLLECTOR_QUEUE_SIZE", 32768), // was 4096 — 8× larger
		BatchMaxPoints:       envInt("LE_BATCH_MAX_POINTS", 512),       // was 128 — 4× larger batches
		BatchMaxAge:          envDuration("LE_BATCH_MAX_AGE", 2*time.Second),
		FlushTimeout:         envDuration("LE_FLUSH_TIMEOUT", 5*time.Second),
		MaxRetries:           envInt("LE_INGEST_MAX_RETRIES", 4),
		RetryBaseDelay:       envDuration("LE_INGEST_RETRY_BASE", 250*time.Millisecond),
		RetryMaxDelay:        envDuration("LE_INGEST_RETRY_MAX", 5*time.Second),
		CircuitOpenAfter:     envInt("LE_CIRCUIT_OPEN_AFTER", 5),
		CircuitCooldown:      envDuration("LE_CIRCUIT_COOLDOWN", 15*time.Second),
		MaxScrapeConcurrency: envInt("LE_MAX_SCRAPE_CONCURRENCY", 64),  // was 16 — 4× higher concurrency
		MaxScrapeBodyBytes:   int64(envInt("LE_MAX_SCRAPE_BODY_BYTES", 8<<20)), // was 4MB — 8MB
		RequestRateLimitRPS:  envFloat("LE_INGEST_REQUEST_RPS", 0),     // was 10 — 0 disables throttle
		DiscoveryLabel:       envStr("LE_DISCOVERY_LABEL", "le.enable"),
		DefaultMetricsPath:   cleanPath(envStr("LE_DEFAULT_METRICS_PATH", "/metrics")),
	}
}

func envStr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			return parsed
		}
	}
	return fallback
}

func envFloat(key string, fallback float64) float64 {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			return parsed
		}
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if parsed, err := time.ParseDuration(v); err == nil {
			return parsed
		}
	}
	return fallback
}

func cleanPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/metrics"
	}
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
}