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

// ── buffer pool ───────────────────────────────────────────────────────────────
//
// Each flush serialises a batch to JSON. The original code allocated a fresh
// []byte for every batch. At high volume that is millions of allocs/sec.
// The pool recycles buffers. json.Marshal appends into the existing backing array.

var jsonBufPool = sync.Pool{
	New: func() any { return bytes.NewBuffer(make([]byte, 0, 64*1024)) },
}

// ── IngestClient ─────────────────────────────────────────────────────────────

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
		cfg.QueueSize = 32768
	}
	if cfg.BatchMaxPoints <= 0 {
		cfg.BatchMaxPoints = 512
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
				// Keep a healthy pool of persistent connections to the ingest endpoint.
				// Without this every POST opens a new TCP connection, adding ~1ms RTT each.
				MaxIdleConns:          256,
				MaxIdleConnsPerHost:   256,
				MaxConnsPerHost:       256,
				IdleConnTimeout:       120 * time.Second,
				DisableKeepAlives:     false,
				ForceAttemptHTTP2:     false, // HTTP/1.1 keep-alive is faster for short POSTs
				ResponseHeaderTimeout: cfg.FlushTimeout,
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

// Run is the collector flush loop.
//
// Optimization: rate limiting (RequestRateLimitRPS) is disabled by default
// (cfg.RequestRateLimitRPS == 0). The original default of 10 RPS hard-capped
// throughput to 10 POST/s regardless of batch size. With 512-point batches
// and no rate limit, throughput is bounded only by network and the ingest
// endpoint — which is now the sharded store capable of 100k+ points/sec.
func (c *IngestClient) Run(ctx context.Context) {
	ticker := time.NewTicker(c.cfg.BatchMaxAge)
	defer ticker.Stop()

	// Rate limiter is opt-in only. Zero value means unlimited.
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

		// Take a buffer from the pool to avoid per-flush allocation.
		buf := jsonBufPool.Get().(*bytes.Buffer)
		buf.Reset()

		enc := json.NewEncoder(buf)
		if err := enc.Encode(batch); err != nil {
			jsonBufPool.Put(buf)
			log.Printf(`{"component":"le-collector","level":"warn","event":"json_encode_failed","error":%q}`, err.Error())
			batch = batch[:0]
			return
		}

		bodyBytes := make([]byte, buf.Len())
		copy(bodyBytes, buf.Bytes())
		jsonBufPool.Put(buf)

		if err := c.flushWithRetry(ctx, bodyBytes, len(batch)); err != nil {
			c.stats.BatchesErrorTotal.Add(1)
			c.stats.markError(time.Now())
			log.Printf(`{"component":"le-collector","level":"warn","event":"ingest_failed","points":%d,"error":%q}`, len(batch), err.Error())
		} else {
			c.stats.BatchesSentTotal.Add(1)
			c.stats.LastIngestUnix.Store(time.Now().Unix())
		}
		batch = batch[:0]
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case point := <-c.queue:
			// Drop oldest if batch is overflowing (should not happen with large queue).
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

func (c *IngestClient) flushWithRetry(ctx context.Context, body []byte, pointCount int) error {
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
		if err := c.postBytes(ctx, body); err != nil {
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

// postBytes sends a pre-serialised JSON body to the ingest endpoint.
// Using a pre-serialised body (vs json.Marshal inside post()) means the
// hot path does zero allocation on retry — same bytes, new request.
func (c *IngestClient) postBytes(ctx context.Context, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.IngestURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.IngestToken != "" {
		req.Header.Set("X-Ingest-Token", c.cfg.IngestToken)
	}
	// Hint: body size is known — allows Content-Length header without chunked encoding.
	req.ContentLength = int64(len(body))

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// Drain body so the connection is returned to the pool.
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1024)) //nolint:errcheck
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ingest status=%d", resp.StatusCode)
	}
	return nil
}

// ── circuit breaker ───────────────────────────────────────────────────────────

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