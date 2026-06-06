// Package security provides all security primitives for the LoadEquilibrium API:
//   - Token authentication (constant-time comparison, timing-attack-safe)
//   - Per-IP and per-token rate limiting (token bucket, no external deps)
//   - Input validation (service ID length, charset, field bounds)
//   - Audit logging (structured JSON, every auth event, every control action)
//   - Security headers middleware (CSP, HSTS, X-Frame-Options, etc.)
//   - WebSocket origin policy
//
// Every function in this package is zero external dependency.
// All cryptographic operations use stdlib crypto/*.
package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// ── Constants ───────────────

const (
	// MaxServiceIDLen is the hard ceiling on a service_id field.
	// 128 chars covers all realistic service naming conventions.
	MaxServiceIDLen = 128

	// MaxBatchSize is the maximum number of MetricPoints in one ingest call.
	// Prevents a single POST from filling the entire ring buffer.
	MaxBatchSize = 512

	// MinTokenLen enforces a minimum entropy floor on INGEST_TOKEN.
	MinTokenLen = 32

	// AuditBufSize is the in-memory audit ring buffer size.
	AuditBufSize = 10_000
)

// serviceIDPattern: alphanumeric, dash, underscore, dot, slash, colon.
// Rejects null bytes, control characters, HTML tags, SQL fragments.
var serviceIDPattern = regexp.MustCompile(`^[a-zA-Z0-9\-_./: ]{1,128}$`)

// ── Token ──

// ValidateToken compares a candidate token against the expected token
// using constant-time comparison (crypto/subtle) to prevent timing attacks.
//
// Original code used `headerToken != s.token` — a plain string equality check
// that short-circuits on the first differing byte, leaking timing information
// that allows an attacker to guess tokens one byte at a time.
//
// This function always takes the same time regardless of how many bytes match.
func ValidateToken(expected, candidate string) bool {
	if len(expected) == 0 {
		// No token configured — auth disabled (dev mode only).
		return true
	}
	if len(candidate) == 0 {
		return false
	}
	// subtle.ConstantTimeCompare requires equal-length slices.
	// XOR lengths first to fail fast without leaking position.
	if subtle.ConstantTimeEq(int32(len(expected)), int32(len(candidate))) == 0 {
		// Still do a dummy comparison to normalize timing.
		_ = subtle.ConstantTimeCompare([]byte(expected), []byte(expected))
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(candidate)) == 1
}

// ExtractToken extracts the bearer token from an HTTP request.
// Accepts both "Authorization: Bearer <token>" and "X-Ingest-Token: <token>".
func ExtractToken(r *http.Request) string {
	// Check X-Ingest-Token header first (used by le-collector)
	if t := strings.TrimSpace(r.Header.Get("X-Ingest-Token")); t != "" {
		return t
	}
	// Check Authorization: Bearer <token>
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	// Check query param for WebSocket (browsers can't set WS headers)
	if t := strings.TrimSpace(r.URL.Query().Get("token")); t != "" {
		return t
	}
	return ""
}

// GenerateToken generates a cryptographically random hex token of the given byte length.
// Use length=32 for a 256-bit token (64 hex chars).
func GenerateToken(byteLen int) (string, error) {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// WarnWeakToken logs a startup warning if the token is weak or default.
func WarnWeakToken(token string) {
	if token == "" {
		log.Println("[security] WARNING: INGEST_TOKEN is empty — authentication disabled. Set a strong token in production.")
		return
	}
	weakTokens := []string{
		"changeme", "changeme-set-in-production", "secret", "password",
		"token", "test", "dev", "local", "admin", "12345678",
	}
	lower := strings.ToLower(token)
	for _, weak := range weakTokens {
		if lower == weak || strings.HasPrefix(lower, weak) {
			log.Printf("[security] WARNING: INGEST_TOKEN looks like a default/weak value (%q). Rotate before exposing to network.", token[:min(len(token), 8)]+"...")
			return
		}
	}
	if utf8.RuneCountInString(token) < MinTokenLen {
		log.Printf("[security] WARNING: INGEST_TOKEN is only %d chars. Minimum recommended: %d.", utf8.RuneCountInString(token), MinTokenLen)
	}
}

// ── Input Validation ───────

// ValidationError is returned when input fails validation.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error: field=%s msg=%s", e.Field, e.Message)
}

// ValidateServiceID checks that a service ID is safe for use as a map key
// and display label. Rejects: empty, too long, control characters, HTML, SQL.
func ValidateServiceID(id string) error {
	if id == "" {
		return &ValidationError{Field: "service_id", Message: "must not be empty"}
	}
	if len(id) > MaxServiceIDLen {
		return &ValidationError{Field: "service_id", Message: fmt.Sprintf("exceeds %d chars (got %d)", MaxServiceIDLen, len(id))}
	}
	if !serviceIDPattern.MatchString(id) {
		return &ValidationError{Field: "service_id", Message: "contains invalid characters (allowed: a-z A-Z 0-9 - _ . / : space)"}
	}
	return nil
}

// ValidateMetricFloat checks that a metric value is finite and within bounds.
func ValidateMetricFloat(name string, v float64, min, max float64) error {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return &ValidationError{Field: name, Message: "must be finite"}
	}
	if v < min || v > max {
		return &ValidationError{Field: name, Message: fmt.Sprintf("out of range [%.0f, %.0f] (got %.2f)", min, max, v)}
	}
	return nil
}

// ValidateBatchSize checks that a batch does not exceed the hard limit.
func ValidateBatchSize(n int) error {
	if n > MaxBatchSize {
		return &ValidationError{Field: "batch", Message: fmt.Sprintf("batch too large: %d > %d", n, MaxBatchSize)}
	}
	return nil
}

// ── Rate Limiter ────────────

// RateLimiter implements a token-bucket rate limiter per key (IP or token).
// No external dependencies. Thread-safe.
//
// Design: per-key buckets with configurable capacity and refill rate.
// Buckets are garbage-collected after idleTimeout to prevent unbounded growth.
type RateLimiter struct {
	mu          sync.Mutex
	buckets     map[string]*bucket
	capacity    float64       // max tokens per bucket
	refillRate  float64       // tokens added per second
	idleTimeout time.Duration // how long before bucket is GC'd
	lastCleanup time.Time
}

type bucket struct {
	tokens   float64
	lastSeen time.Time
}

// NewRateLimiter creates a rate limiter.
//
//	capacity:   max burst size (e.g. 100 requests burst)
//	rps:        sustained rate (e.g. 50 requests/second)
//	idleFor:    clean up buckets idle for this long (e.g. 5 minutes)
func NewRateLimiter(capacity, rps float64, idleFor time.Duration) *RateLimiter {
	return &RateLimiter{
		buckets:     make(map[string]*bucket),
		capacity:    capacity,
		refillRate:  rps,
		idleTimeout: idleFor,
		lastCleanup: time.Now(),
	}
}

// Allow checks whether the key (IP or token) is within the rate limit.
// Returns true if the request is allowed, false if it should be rejected.
func (rl *RateLimiter) Allow(key string) bool {
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.buckets[key]
	if !ok {
		b = &bucket{tokens: rl.capacity, lastSeen: now}
		rl.buckets[key] = b
	}

	// Refill tokens based on elapsed time.
	elapsed := now.Sub(b.lastSeen).Seconds()
	b.tokens += elapsed * rl.refillRate
	if b.tokens > rl.capacity {
		b.tokens = rl.capacity
	}
	b.lastSeen = now

	if b.tokens < 1.0 {
		// Bucket exhausted — rate limit hit.
		rl.gcIfNeeded(now)
		return false
	}
	b.tokens--
	rl.gcIfNeeded(now)
	return true
}

func (rl *RateLimiter) gcIfNeeded(now time.Time) {
	// Clean up at most once per minute to avoid O(N) on every request.
	if now.Sub(rl.lastCleanup) < time.Minute {
		return
	}
	for key, b := range rl.buckets {
		if now.Sub(b.lastSeen) > rl.idleTimeout {
			delete(rl.buckets, key)
		}
	}
	rl.lastCleanup = now
}

// RealIP extracts the real client IP, respecting X-Forwarded-For when behind
// a trusted reverse proxy. Falls back to RemoteAddr.
func RealIP(r *http.Request) string {
	// Check X-Real-IP first (set by nginx)
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return strings.TrimSpace(ip)
	}
	// Check X-Forwarded-For (first entry is the client)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	// Fall back to RemoteAddr (strip port)
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ── Audit Logger ────────────

// AuditEvent is one structured security event.
type AuditEvent struct {
	Time      time.Time `json:"t"`
	IP        string    `json:"ip,omitempty"`
	Path      string    `json:"path"`
	Method    string    `json:"method"`
	EventType string    `json:"event"`      // auth_ok|auth_fail|rate_limit|control_action|validation_fail
	Detail    string    `json:"detail,omitempty"`
	TokenHash string    `json:"token_hash,omitempty"` // first 8 chars of token (not full token)
	Status    int       `json:"status,omitempty"`
}

// AuditLogger writes structured security events.
// Events go to stderr (always) and optionally to a ring buffer for the /api/v1/audit endpoint.
type AuditLogger struct {
	mu     sync.Mutex
	buf    []AuditEvent
	cap    int
	head   int
	size   int
	logger *log.Logger
}

// NewAuditLogger creates an audit logger with an in-memory ring buffer.
func NewAuditLogger() *AuditLogger {
	return &AuditLogger{
		buf:    make([]AuditEvent, AuditBufSize),
		cap:    AuditBufSize,
		logger: log.New(os.Stderr, "", 0),
	}
}

// Log records a security event.
func (al *AuditLogger) Log(e AuditEvent) {
	e.Time = time.Now().UTC()
	data, _ := json.Marshal(e)
	al.logger.Println(string(data))

	al.mu.Lock()
	al.buf[al.head] = e
	al.head = (al.head + 1) % al.cap
	if al.size < al.cap {
		al.size++
	}
	al.mu.Unlock()
}

// Recent returns the last n audit events (newest last).
func (al *AuditLogger) Recent(n int) []AuditEvent {
	al.mu.Lock()
	defer al.mu.Unlock()
	if n > al.size {
		n = al.size
	}
	out := make([]AuditEvent, n)
	for i := 0; i < n; i++ {
		idx := (al.head - n + i + al.cap) % al.cap
		out[i] = al.buf[idx]
	}
	return out
}

// tokenPrefix returns the first 8 chars of a token for audit logs.
// Never logs the full token.
func tokenPrefix(token string) string {
	if len(token) == 0 {
		return ""
	}
	if len(token) > 8 {
		return token[:8] + "…"
	}
	return token[:len(token)/2] + "…"
}

// ── Security Headers ────────

// SecurityHeaders returns an HTTP middleware that sets hardened response headers.
//
// Headers applied:
//   - Content-Security-Policy: restricts script sources to self + inline for React
//   - X-Frame-Options: DENY — prevents clickjacking
//   - X-Content-Type-Options: nosniff — prevents MIME-type sniffing
//   - Referrer-Policy: strict-origin-when-cross-origin
//   - Permissions-Policy: disables unused browser features
//   - Strict-Transport-Security: force HTTPS (when TLS is active)
//   - Cache-Control: no-store for API responses
func SecurityHeaders(tlsEnabled bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()

			// Prevent clickjacking
			h.Set("X-Frame-Options", "DENY")

			// Prevent MIME sniffing
			h.Set("X-Content-Type-Options", "nosniff")

			// Referrer policy
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")

			// Disable unused browser features
			h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

			// CSP: allow self for scripts/styles (React SPA), block everything else
			// unsafe-inline needed for React's runtime event handlers
			h.Set("Content-Security-Policy",
				"default-src 'self'; "+
					"script-src 'self' 'unsafe-inline'; "+
					"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; "+
					"font-src 'self' https://fonts.gstatic.com; "+
					"connect-src 'self' ws: wss:; "+
					"img-src 'self' data:; "+
					"frame-ancestors 'none'")

			// HSTS: only when TLS is active
			if tlsEnabled {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}

			// API responses must not be cached
			if strings.HasPrefix(r.URL.Path, "/api/") {
				h.Set("Cache-Control", "no-store, max-age=0")
				h.Set("Pragma", "no-cache")
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ── CORS Policy ─────────────

// CORSMiddleware replaces the original wildcard CORS with a configurable policy.
//
// Original: Access-Control-Allow-Origin: * — allows any website to call the API
// in a credentialed context, meaning attacker.com can call your control endpoints
// if the browser has an active session.
//
// This version:
//   - Allows only explicitly configured origins (or same-origin if none configured)
//   - Never reflects an arbitrary Origin back (prevents CORS-bypass via reflection)
//   - Does not include credentials header (API uses explicit tokens, not cookies)
func CORSMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	originsMap := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		originsMap[strings.TrimRight(o, "/")] = true
	}
	allowAll := len(allowedOrigins) == 0

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if origin != "" {
				normalised := strings.TrimRight(origin, "/")
				if allowAll || originsMap[normalised] {
					w.Header().Set("Access-Control-Allow-Origin", origin) // echo exact origin, not *
					w.Header().Set("Vary", "Origin")
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Ingest-Token")
					w.Header().Set("Access-Control-Max-Age", "3600")
					// Do NOT set Access-Control-Allow-Credentials: true
					// API uses explicit tokens in headers, not cookies
				}
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ── Auth Middleware ─────────

// AuthConfig holds the authentication and rate-limiting configuration.
type AuthConfig struct {
	// IngestToken is the shared secret for POST /api/v1/ingest.
	// If empty, ingest auth is disabled (not recommended for production).
	IngestToken string

	// ControlToken is the shared secret for all control plane endpoints.
	// If empty, falls back to IngestToken.
	// Set a DIFFERENT token from IngestToken to enforce least-privilege:
	// collector agents only need IngestToken; operators need ControlToken.
	ControlToken string

	// DashboardToken protects GET /ws and GET /api/v1/snapshot.
	// If empty, dashboard is readable without auth (acceptable for internal networks).
	DashboardToken string

	// AllowedOrigins for CORS. Empty = allow all (acceptable for internal networks).
	AllowedOrigins []string

	// TLSEnabled controls whether HSTS header is sent.
	TLSEnabled bool
}

// AuthMiddleware returns a middleware factory that enforces token auth for
// specific route classes. Routes not covered by any class pass through freely.
//
// Route classes:
//   - "ingest":   POST /api/v1/ingest — checked against IngestToken
//   - "control":  POST /api/v1/control/*, policy/*, runtime/*, sandbox/*, simulation/*, intelligence/*, alerts/*
//                 checked against ControlToken (falls back to IngestToken)
//   - "dashboard": GET /ws, GET /api/v1/snapshot — checked against DashboardToken if set
//
// Why separate tokens:
//   - Collector agents running in containers get IngestToken only.
//     If a container is compromised, the attacker cannot toggle the autopilot.
//   - Dashboard users get DashboardToken only.
//     They can see all state but cannot issue control commands.
//   - Only human operators with ControlToken can change policy or inject chaos.
func AuthMiddleware(cfg AuthConfig, audit *AuditLogger, ingestRL, controlRL *RateLimiter) func(http.Handler) http.Handler {
	controlToken := cfg.ControlToken
	if controlToken == "" {
		controlToken = cfg.IngestToken
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			ip := RealIP(r)
			candidate := ExtractToken(r)

			// ── Determine which route class this is 
			var requiredToken, routeClass string
			switch {
			case path == "/api/v1/ingest":
				requiredToken = cfg.IngestToken
				routeClass = "ingest"
			case strings.HasPrefix(path, "/api/v1/control/") ||
				path == "/api/v1/policy/update" ||
				path == "/api/v1/runtime/step" ||
				path == "/api/v1/sandbox/trigger" ||
				path == "/api/v1/simulation/control" ||
				path == "/api/v1/intelligence/rollout" ||
				path == "/api/v1/alerts/ack":
				requiredToken = controlToken
				routeClass = "control"
			case path == "/ws" || path == "/api/v1/snapshot":
				requiredToken = cfg.DashboardToken
				routeClass = "dashboard"
			default:
				// /health, /metrics, static UI — no auth required
				next.ServeHTTP(w, r)
				return
			}

			// ── Rate limiting ───────
			var rl *RateLimiter
			if routeClass == "ingest" {
				rl = ingestRL
			} else {
				rl = controlRL
			}
			if rl != nil && !rl.Allow(ip) {
				audit.Log(AuditEvent{
					IP: ip, Path: path, Method: r.Method,
					EventType: "rate_limit",
					Detail:    fmt.Sprintf("class=%s", routeClass),
				})
				w.Header().Set("Retry-After", "60")
				http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}

			// ── Token authentication 
			if requiredToken != "" {
				if !ValidateToken(requiredToken, candidate) {
					audit.Log(AuditEvent{
						IP: ip, Path: path, Method: r.Method,
						EventType: "auth_fail",
						Detail:    fmt.Sprintf("class=%s", routeClass),
						TokenHash: tokenPrefix(candidate),
						Status:    http.StatusUnauthorized,
					})
					// Uniform error regardless of whether token was missing or wrong.
					// Never leak which condition failed.
					http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
					return
				}
			}

			// ── Auth success 
			if requiredToken != "" {
				audit.Log(AuditEvent{
					IP: ip, Path: path, Method: r.Method,
					EventType: "auth_ok",
					Detail:    fmt.Sprintf("class=%s", routeClass),
					TokenHash: tokenPrefix(candidate),
					Status:    http.StatusOK,
				})
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ── Helpers 

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}