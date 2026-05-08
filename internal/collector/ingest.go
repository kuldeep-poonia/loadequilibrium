package collector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/telemetry"
)

type IngestClient struct {
	cfg      Config
	stats    *Stats
	queue    chan telemetry.MetricPoint
	client   *http.Client
	mu       sync.Mutex
	failures int
	openTil  time.Time
}

func NewIngestClient(cfg Config, stats *Stats) *IngestClient {
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 4096
	}
	if cfg.BatchMaxPoints <= 0 {
		cfg.BatchMaxPoints = 128
	}
	if cfg.BatchMaxAge <= 0 {
		cfg.BatchMaxAge = 2 * time.Second
	}
	if cfg.FlushTimeout <= 0 {
		cfg.FlushTimeout = 5 * time.Second
	}
	return &IngestClient{
		cfg:   cfg,
		stats: stats,
		queue: make(chan telemetry.MetricPoint, cfg.QueueSize),
		client: &http.Client{
			Timeout: cfg.FlushTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        64,
				MaxIdleConnsPerHost: 16,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

func (c *IngestClient) Enqueue(point telemetry.MetricPoint) bool {
	select {
	case c.queue <- point:
		c.stats.PointsQueuedTotal.Add(1)
		return true
	default:
		c.stats.PointsDroppedTotal.Add(1)
		c.stats.markError(time.Now())
		return false
	}
}

func (c *IngestClient) Run(ctx context.Context) {
	ticker := time.NewTicker(c.cfg.BatchMaxAge)
	defer ticker.Stop()

	var rate <-chan time.Time
	if c.cfg.RequestRateLimitRPS > 0 {
		period := time.Duration(float64(time.Second) / c.cfg.RequestRateLimitRPS)
		if period < time.Millisecond {
			period = time.Millisecond
		}
		limiter := time.NewTicker(period)
		defer limiter.Stop()
		rate = limiter.C
	}

	batch := make([]telemetry.MetricPoint, 0, c.cfg.BatchMaxPoints)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if rate != nil {
			select {
			case <-ctx.Done():
				return
			case <-rate:
			}
		}
		if err := c.flushWithRetry(ctx, batch); err != nil {
			c.stats.BatchesErrorTotal.Add(1)
			c.stats.markError(time.Now())
			log.Printf(`{"component":"le-collector","level":"warn","event":"ingest_failed","points":%d,"error":%q}`, len(batch), err.Error())
			return
		}
		c.stats.BatchesSentTotal.Add(1)
		c.stats.LastIngestUnix.Store(time.Now().Unix())
		batch = batch[:0]
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case point := <-c.queue:
			if len(batch) >= c.cfg.BatchMaxPoints*4 {
				copy(batch, batch[1:])
				batch = batch[:len(batch)-1]
				c.stats.PointsDroppedTotal.Add(1)
			}
			batch = append(batch, point)
			if len(batch) >= c.cfg.BatchMaxPoints {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func (c *IngestClient) flushWithRetry(ctx context.Context, points []telemetry.MetricPoint) error {
	if c.circuitOpen(time.Now()) {
		return fmt.Errorf("ingest circuit open until %s", c.openUntil().Format(time.RFC3339))
	}

	var lastErr error
	delay := c.cfg.RetryBaseDelay
	if delay <= 0 {
		delay = 250 * time.Millisecond
	}
	maxDelay := c.cfg.RetryMaxDelay
	if maxDelay <= 0 {
		maxDelay = 5 * time.Second
	}
	attempts := c.cfg.MaxRetries + 1
	if attempts < 1 {
		attempts = 1
	}

	for attempt := 0; attempt < attempts; attempt++ {
		if err := c.post(ctx, points); err != nil {
			lastErr = err
			if attempt == attempts-1 {
				break
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
			continue
		}
		c.recordSuccess()
		return nil
	}

	c.recordFailure()
	return lastErr
}

func (c *IngestClient) post(ctx context.Context, points []telemetry.MetricPoint) error {
	body, err := json.Marshal(points)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.IngestURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.IngestToken != "" {
		req.Header.Set("X-Ingest-Token", c.cfg.IngestToken)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("ingest status=%d body=%s", resp.StatusCode, string(bytes.TrimSpace(msg)))
	}
	return nil
}

func (c *IngestClient) circuitOpen(now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	open := !c.openTil.IsZero() && now.Before(c.openTil)
	c.stats.CircuitOpen.Store(open)
	return open
}

func (c *IngestClient) openUntil() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.openTil
}

func (c *IngestClient) recordSuccess() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failures = 0
	c.openTil = time.Time{}
	c.stats.CircuitOpen.Store(false)
}

func (c *IngestClient) recordFailure() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failures++
	if c.cfg.CircuitOpenAfter > 0 && c.failures >= c.cfg.CircuitOpenAfter {
		c.openTil = time.Now().Add(c.cfg.CircuitCooldown)
		c.stats.CircuitOpen.Store(true)
	}
}
