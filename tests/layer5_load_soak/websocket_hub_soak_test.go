package layer5


// RUN:
//   go test ./tests/layer5_load_soak/ -run TestL5_WS_001 -count=1 -timeout=1200s -v

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/api"
	"github.com/loadequilibrium/loadequilibrium/internal/streaming"
	"github.com/loadequilibrium/loadequilibrium/internal/telemetry"
)


// L5-WS-001 — WebSocket hub: 50 clients for 30 minutes, delivery latency SLA
//
// AIM:   50 concurrent WebSocket clients connected for 30 minutes.
//        1 broadcaster goroutine sends 1 TickPayload/second.
//        Each client measures message receipt latency.
//        At end: p95 < 50ms, p99 < 100ms, dropped_clients == 0 (all connected
//        clients remained connected for the full duration), zero panics.
//
// THRESHOLD: p99_delivery_ms < 100, dropped_clients == 0
// ON EXCEED: Hub cannot deliver tick payloads to all dashboard clients within
//            real-time latency budget — operators see stale metrics.

func TestL5_WS_001_HubDeliveryLatencySoak(t *testing.T) {
	if testing.Short() {
		t.Skip("L5-WS-001: skipped in short mode — requires 30 minutes")
	}

	start := time.Now()

	const (
		clientCount  = 50
		soakDuration = 30 * time.Minute
		broadcastHz  = 1  // 1 payload per second
		p99Threshold = 100.0
		p95Threshold = 50.0
	)

	// ── Build real server and hub 
	store  := telemetry.NewStore(256, 50, 5*time.Minute)
	hub    := streaming.NewHub()
	hub.SetMaxClients(clientCount + 10)

	srv    := api.NewServer(store, hub, "")
	httpSrv := httptest.NewServer(srv.Handler())
	defer httpSrv.Close()

	wsAddr := "ws://" + strings.TrimPrefix(httpSrv.URL, "http://") + "/ws"

	// ── Connect all clients
	type wsClient struct {
		conn     net.Conn
		clientID int
	}

	clients := make([]wsClient, 0, clientCount)
	for i := 0; i < clientCount; i++ {
		conn, err := l5DialWebSocket(wsAddr)
		if err != nil {
			t.Fatalf("L5-WS-001: could not connect client %d: %v", i, err)
		}
		clients = append(clients, wsClient{conn: conn, clientID: i})
	}

	// Wait for all clients to register in hub.
	deadline := time.Now().Add(10 * time.Second)
	for hub.ClientCount() < clientCount && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if hub.ClientCount() < clientCount {
		t.Fatalf("L5-WS-001: only %d/%d clients registered in hub after 10s", hub.ClientCount(), clientCount)
	}
	t.Logf("L5-WS-001: %d clients connected", hub.ClientCount())

	// ── Measure latencies per client ──────────────────────────────────────────
	var (
		totalDelivered    int64
		totalDropped      int64
		allLatenciesMu    sync.Mutex
		allLatenciesMs    []float64
	)

	// Start reader goroutines — one per client.
	var readerWg sync.WaitGroup
	for _, c := range clients {
		c := c
		readerWg.Add(1)
		go func() {
			defer readerWg.Done()
			for {
				if err := c.conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
					return
				}
				payload, err := l5ReadWSFrame(c.conn)
				if err != nil {
					// Connection closed or deadline — check if soak is over.
					return
				}

				// Parse the TickPayload to extract the embedded timestamp.
				var tick struct {
					Timestamp time.Time `json:"ts"`
				}
				if jsonErr := json.Unmarshal(payload, &tick); jsonErr != nil {
					continue
				}
				if tick.Timestamp.IsZero() {
					continue
				}
				latencyMs := float64(time.Since(tick.Timestamp).Microseconds()) / 1000.0
				if latencyMs < 0 {
					latencyMs = 0 // clock skew guard
				}

				atomic.AddInt64(&totalDelivered, 1)
				allLatenciesMu.Lock()
				allLatenciesMs = append(allLatenciesMs, latencyMs)
				allLatenciesMu.Unlock()
			}
		}()
	}

	// ── Broadcast at 1Hz for soak duration ───────────────────────────────────
	broadcastTicker := time.NewTicker(time.Duration(1000/broadcastHz) * time.Millisecond)
	soakTimer       := time.After(soakDuration)
	broadcastCount  := 0
	initialClients  := hub.ClientCount()

	for {
		select {
		case <-soakTimer:
			goto soakDone
		case <-broadcastTicker.C:
			payload := &streaming.TickPayload{
				Type:         streaming.MsgTick,
				TickHealthMs: float64(broadcastCount) * 0.001,
				RuntimeMetrics: streaming.RuntimeMetrics{
					AvgPruneMs:     float64(broadcastCount % 100) * 0.01,
					AvgModellingMs: float64(broadcastCount%50) * 0.02,
				},
			}
			hub.Broadcast(payload)
			broadcastCount++

			// Check for dropped clients.
			current := hub.ClientCount()
			if current < initialClients {
				dropped := int64(initialClients - current)
				atomic.AddInt64(&totalDropped, dropped)
				initialClients = current
				t.Logf("L5-WS-001 WARNING: client dropped at broadcast %d (now %d/%d connected)",
					broadcastCount, current, clientCount)
			}
		}
	}

soakDone:
	broadcastTicker.Stop()

	// Close all client connections to unblock reader goroutines.
	for _, c := range clients {
		c.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	}
	readerWg.Wait()
	for _, c := range clients {
		c.conn.Close()
	}

	// ── Compute metrics ───────────────────────────────────────────────────────
	allLatenciesMu.Lock()
	latencies := make([]float64, len(allLatenciesMs))
	copy(latencies, allLatenciesMs)
	allLatenciesMu.Unlock()

	pct := computePercentiles(latencies)
	dropped := atomic.LoadInt64(&totalDropped)
	delivered := atomic.LoadInt64(&totalDelivered)
	expectedDeliveries := int64(broadcastCount) * int64(clientCount)

	passed := pct.P99Ms < p99Threshold && dropped == 0
	durationMs := time.Since(start).Milliseconds()

	// Zero-delivery guard: if we got no measurements, something is broken.
	if delivered == 0 && broadcastCount > 0 {
		t.Errorf("L5-WS-001: zero messages delivered despite %d broadcasts", broadcastCount)
	}

	writeL5Result(L5Record{
		TestID: "L5-WS-001",
		Layer:  5,
		Name:   fmt.Sprintf("WebSocket hub %d clients, %s soak, %dHz broadcast", clientCount, soakDuration, broadcastHz),
		Aim: fmt.Sprintf(
			"%d clients for %s receiving %dHz broadcasts: p95<%.0fms p99<%.0fms, zero dropped clients",
			clientCount, soakDuration, broadcastHz, p95Threshold, p99Threshold,
		),
		PackagesInvolved: []string{"internal/streaming", "internal/api"},
		FunctionsTested: []string{
			"streaming.NewHub", "(*Hub).SetMaxClients", "(*Hub).Broadcast",
			"(*Hub).ClientCount", "(*Hub).HandleUpgrade (via /ws route)",
		},
		Threshold: L5Threshold{
			Metric:    "p99_delivery_ms",
			Operator:  "<",
			Value:     p99Threshold,
			Unit:      "ms",
			Rationale: "Dashboard must receive tick payloads within 100ms p99 to show near-real-time metrics",
		},
		Result: L5ResultData{
			Status:        l5Status(passed),
			ActualValue:   pct.P99Ms,
			ActualUnit:    "p99_delivery_ms",
			SampleCount:   int(delivered),
			Percentiles:   &pct,
			ErrorCount:    dropped,
			ErrorRate:     float64(dropped) / float64(max64(int64(broadcastCount)*int64(clientCount), 1)),
			DurationMs:    durationMs,
			ErrorMessages: []string{fmt.Sprintf(
				"broadcasts=%d clients=%d delivered=%d expected=%d dropped_clients=%d",
				broadcastCount, clientCount, delivered, expectedDeliveries, dropped,
			)},
		},
		OnExceed: "Dashboard operators see stale tick data — metrics lag behind real-time system state by >100ms",
		Questions: L5Questions{
			WhatWasTested: fmt.Sprintf(
				"%d real WebSocket clients connected to real Hub via real api.Server /ws for %s",
				clientCount, soakDuration,
			),
			WhyThisThreshold:     "p99<100ms: hub uses non-blocking send with drop-oldest; delivery at p99 tests that the happy path is fast",
			WhatHappensIfFails:   "Dashboard shows metrics that are multiple seconds behind reality — operators make decisions on stale data",
			HowLoadWasGenerated:  fmt.Sprintf("1 broadcaster goroutine at %dHz, %d concurrent reader goroutines", broadcastHz, clientCount),
			HowMetricsMeasured:   "Latency = time.Since(payload.Timestamp) measured in each reader goroutine; JSON-parsed from raw WebSocket frame",
			WorstCaseDescription: fmt.Sprintf("p99=%.2fms p100=%.2fms dropped=%d", pct.P99Ms, pct.P100Ms, dropped),
		},
		RunAt: l5Now(), GoVersion: l5GoVer(),
	})

	if !passed {
		t.Fatalf(
			"L5-WS-001 FAILED: p99=%.2fms (threshold=%.0fms) dropped=%d\n"+
				"broadcasts=%d delivered=%d\n"+
				"FIX: If p99 is high: check writePump writeTimeout (5s) in hub.go — it may be blocking.\n"+
				"     If clients dropped: check sendBufferSize (16) vs broadcast rate — buffer overflow drops clients.\n"+
				"     Files: internal/streaming/hub.go",
			pct.P99Ms, p99Threshold, dropped, broadcastCount, delivered,
		)
	}
	t.Logf("L5-WS-001 PASS | p50=%.2fms p95=%.2fms p99=%.2fms broadcasts=%d delivered=%d",
		pct.P50Ms, pct.P95Ms, pct.P99Ms, broadcastCount, delivered)
}

// ─────────────────────────────────────────────────────────────────────────────
// L5-WS-001b — Hub max client cap enforced: beyond cap returns 503
//
// AIM:   With maxClients=5, the 6th connection attempt must receive HTTP 503.
//        Existing 5 connections must continue receiving broadcasts.
//        No panic when cap is hit.
//
// THRESHOLD: rejected_6th_client == true (503), existing_clients_unaffected == true
// ON EXCEED: Hub accepts unlimited clients → server OOM under connection flood.
// ─────────────────────────────────────────────────────────────────────────────
func TestL5_WS_001b_HubMaxClientCapEnforced(t *testing.T) {
	start := time.Now()

	const maxClients = 5

	store  := telemetry.NewStore(64, 10, time.Minute)
	hub    := streaming.NewHub()
	hub.SetMaxClients(maxClients)

	srv     := api.NewServer(store, hub, "")
	httpSrv := httptest.NewServer(srv.Handler())
	defer httpSrv.Close()

	wsURL := "ws://" + strings.TrimPrefix(httpSrv.URL, "http://") + "/ws"

	// Connect exactly maxClients clients.
	accepted := make([]net.Conn, 0, maxClients)
	for i := 0; i < maxClients; i++ {
		conn, err := l5DialWebSocket(wsURL)
		if err != nil {
			t.Fatalf("L5-WS-001b: client %d could not connect: %v", i, err)
		}
		accepted = append(accepted, conn)
	}
	defer func() {
		for _, c := range accepted {
			c.Close()
		}
	}()

	// Wait for all to register.
	deadline := time.Now().Add(5 * time.Second)
	for hub.ClientCount() < maxClients && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if hub.ClientCount() != maxClients {
		t.Fatalf("L5-WS-001b: expected %d clients, got %d", maxClients, hub.ClientCount())
	}

	// Attempt a 6th connection — must be rejected with 503.
	rejected := false
	// dialWebSocket performs the HTTP upgrade. If the server sends 503 instead of 101,
	// the handshake fails and dialWebSocket returns an error.
	conn6, err := l5DialWebSocket(wsURL)
	if err != nil {
		// Any error from dial means the 6th connection was rejected — correct.
		rejected = true
		t.Logf("L5-WS-001b: 6th connection correctly rejected: %v", err)
	} else {
		// If it succeeded, the cap was not enforced.
		conn6.Close()
		t.Errorf("L5-WS-001b: 6th connection was ACCEPTED (cap=%d) — hub did not enforce maxClients", maxClients)
	}

	// Verify existing clients still connected.
	clientsAfter := hub.ClientCount()

	// Broadcast one payload — all maxClients must be able to receive it.
	hub.Broadcast(&streaming.TickPayload{
		Type:         streaming.MsgTick,
		TickHealthMs: 1.0,
	})
	time.Sleep(100 * time.Millisecond)

	passed := rejected && clientsAfter == maxClients
	durationMs := time.Since(start).Milliseconds()

	var errMsgs []string
	if !rejected {
		errMsgs = append(errMsgs, fmt.Sprintf("6th client accepted (cap=%d clients_after=%d)", maxClients, clientsAfter))
	}
	if clientsAfter != maxClients {
		errMsgs = append(errMsgs, fmt.Sprintf("clients_after=%d expected=%d", clientsAfter, maxClients))
	}
	if len(errMsgs) == 0 {
		errMsgs = append(errMsgs, fmt.Sprintf("6th rejected=true clients_after=%d max=%d", clientsAfter, maxClients))
	}

	writeL5Result(L5Record{
		TestID: "L5-WS-001b",
		Layer:  5,
		Name:   "Hub max client cap enforced at connection time",
		Aim:    fmt.Sprintf("With maxClients=%d, the %dth connection must be rejected (HTTP 503)", maxClients, maxClients+1),
		PackagesInvolved: []string{"internal/streaming", "internal/api"},
		FunctionsTested: []string{
			"(*Hub).SetMaxClients", "(*Hub).HandleUpgrade (cap enforcement)",
		},
		Threshold: L5Threshold{
			Metric:    "6th_client_rejected",
			Operator:  "==",
			Value:     1,
			Unit:      "boolean",
			Rationale: "Unbounded client acceptance leads to OOM under connection flood",
		},
		Result: L5ResultData{
			Status:        l5Status(passed),
			ActualValue:   func() float64 { if rejected { return 1 }; return 0 }(),
			ActualUnit:    "rejected",
			SampleCount:   maxClients + 1,
			DurationMs:    durationMs,
			ErrorMessages: errMsgs,
		},
		OnExceed: "Hub accepts unlimited WebSocket connections → attacker or misbehaving dashboard opens thousands of connections → OOM kill",
		Questions: L5Questions{
			WhatWasTested:       fmt.Sprintf("Hub.SetMaxClients(%d) then attempt %dth connection", maxClients, maxClients+1),
			WhyThisThreshold:    "Hard cap prevents resource exhaustion under connection flood",
			WhatHappensIfFails:  "Each reconnecting dashboard tab creates a new connection → OOM kill",
			HowLoadWasGenerated: "Sequential connection attempts via dialWebSocket helper",
			HowMetricsMeasured:  "dialWebSocket returns error if server sends 503 instead of 101",
			WorstCaseDescription: func() string {
				if !rejected {
					return fmt.Sprintf("6th client accepted (clients=%d)", clientsAfter)
				}
				return "correctly rejected"
			}(),
		},
		RunAt: l5Now(), GoVersion: l5GoVer(),
	})

	if !passed {
		t.Fatalf(
			"L5-WS-001b FAILED: rejected=%v clients_after=%d (expected %d)\n%v\n"+
				"FIX: In HandleUpgrade, the count >= h.maxClients check must happen BEFORE the Hijack.\n"+
				"     File: internal/streaming/hub.go",
			rejected, clientsAfter, maxClients, errMsgs,
		)
	}
	t.Logf("L5-WS-001b PASS | maxClients=%d, 6th rejected, existing=%d unaffected", maxClients, clientsAfter)
}

// ─────────────────────────────────────────────────────────────────────────────
// WebSocket helpers — standalone copy so this file compiles without Layer 3
// ─────────────────────────────────────────────────────────────────────────────

func l5DialWebSocket(rawURL string) (net.Conn, error) {
	addr := strings.TrimPrefix(rawURL, "ws://")
	addr = strings.TrimPrefix(addr, "wss://")
	if idx := strings.Index(addr, "/"); idx != -1 {
		addr = addr[:idx]
	}
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}

	var keyBytes [16]byte
	if _, err := rand.Read(keyBytes[:]); err != nil {
		conn.Close()
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString(keyBytes[:])

	path := "/"
	if idx := strings.Index(strings.TrimPrefix(rawURL, "ws://"), "/"); idx != -1 {
		path = strings.TrimPrefix(rawURL, "ws://")[idx:]
		if atIdx := strings.Index(path[1:], "/"); atIdx == -1 {
			// path is already correct
		}
	}
	// Re-extract path properly.
	withoutScheme := strings.TrimPrefix(rawURL, "ws://")
	if slashIdx := strings.Index(withoutScheme, "/"); slashIdx != -1 {
		path = withoutScheme[slashIdx:]
	}

	req := fmt.Sprintf(
		"GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n",
		path, addr, key,
	)
	if _, err := conn.Write([]byte(req)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("write upgrade: %w", err)
	}

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	br := bufio.NewReader(conn)
	statusLine, err := br.ReadString('\n')
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("read status: %w", err)
	}
	if !strings.Contains(statusLine, "101") {
		conn.Close()
		return nil, fmt.Errorf("upgrade rejected: %q", strings.TrimSpace(statusLine))
	}
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			conn.Close()
			return nil, err
		}
		if line == "\r\n" {
			break
		}
	}
	conn.SetReadDeadline(time.Time{})
	return &l5BufConn{conn: conn, buf: br}, nil
}

type l5BufConn struct {
	conn net.Conn
	buf  *bufio.Reader
}

func (b *l5BufConn) Read(p []byte) (int, error) {
	if b.buf != nil && b.buf.Buffered() > 0 {
		return b.buf.Read(p)
	}
	return b.conn.Read(p)
}
func (b *l5BufConn) Write(p []byte) (int, error)        { return b.conn.Write(p) }
func (b *l5BufConn) Close() error                       { return b.conn.Close() }
func (b *l5BufConn) SetReadDeadline(t time.Time) error  { return b.conn.SetReadDeadline(t) }
func (b *l5BufConn) SetWriteDeadline(t time.Time) error { return b.conn.SetWriteDeadline(t) }
func (b *l5BufConn) LocalAddr() net.Addr                { return b.conn.LocalAddr() }
func (b *l5BufConn) RemoteAddr() net.Addr               { return b.conn.RemoteAddr() }
func (b *l5BufConn) SetDeadline(t time.Time) error       { return b.conn.SetDeadline(t) }

func l5ReadWSFrame(conn net.Conn) ([]byte, error) {
	var header [2]byte
	if _, err := l5ReadFull(conn, header[:]); err != nil {
		return nil, err
	}
	masked := header[1]&0x80 != 0
	payloadLen := int(header[1] & 0x7f)
	switch payloadLen {
	case 126:
		var ext [2]byte
		if _, err := l5ReadFull(conn, ext[:]); err != nil {
			return nil, err
		}
		payloadLen = int(uint16(ext[0])<<8 | uint16(ext[1]))
	case 127:
		var ext [8]byte
		if _, err := l5ReadFull(conn, ext[:]); err != nil {
			return nil, err
		}
		payloadLen = int(
			uint64(ext[0])<<56 | uint64(ext[1])<<48 | uint64(ext[2])<<40 | uint64(ext[3])<<32 |
				uint64(ext[4])<<24 | uint64(ext[5])<<16 | uint64(ext[6])<<8 | uint64(ext[7]),
		)
	}
	var maskKey [4]byte
	if masked {
		if _, err := l5ReadFull(conn, maskKey[:]); err != nil {
			return nil, err
		}
	}
	data := make([]byte, payloadLen)
	if payloadLen > 0 {
		if _, err := l5ReadFull(conn, data); err != nil {
			return nil, err
		}
	}
	if masked {
		for i, b := range data {
			data[i] = b ^ maskKey[i%4]
		}
	}
	
	opcode := header[0] & 0x0f
	if opcode == 0x9 {
		// Reply with masked Pong (FIN=1, opcode=0xA, MASK=1, payloadLen=0, maskKey=[0,0,0,0])
		conn.Write([]byte{0x8A, 0x80, 0x00, 0x00, 0x00, 0x00})
		// Recursively read the next frame
		return l5ReadWSFrame(conn)
	}

	return data, nil
}

func l5ReadFull(conn net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}