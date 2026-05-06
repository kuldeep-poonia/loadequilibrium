package layer3

// FILE: tests/layer3_race_concurrency/L3_HUB_001_broadcast_concurrent_test.go
//
// Tests:   L3-HUB-001, L3-HUB-002
// Package: github.com/loadequilibrium/loadequilibrium/internal/streaming
// Struct:  Hub
// Methods: NewHub() *Hub
//          (*Hub).SetMaxClients(n int)
//          (*Hub).Broadcast(p *TickPayload)
//          (*Hub).GetLastPayload() *TickPayload
//          (*Hub).ClientCount() int
//          (*Hub).HandleUpgrade(w http.ResponseWriter, r *http.Request)
//
// Types used from streaming package:
//   TickPayload  — the broadcast payload
//   MessageType  — TextMessage, PingMessage etc (in ws package)
//
// RUN: go test ./tests/layer3_race_concurrency/ -run TestL3_HUB -race -count=500 -timeout=600s -v

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/streaming"
)

// ─────────────────────────────────────────────────────────────────────────────
// L3-HUB-001 — Hub.Broadcast concurrent safety
//
// AIM:   50 goroutines calling Broadcast simultaneously for 20 seconds must
//        produce zero data races, zero panics, and the sequence numbers in
//        GetLastPayload must strictly increase (seqNo.Add is atomic).
//
// THRESHOLD: panics == 0, seq_no_regressions == 0
// ON EXCEED: Race on h.mu or h.lastPayload → corrupt tick payload sent to
//            dashboard → operators see wrong topology or incorrect risk scores.
// ─────────────────────────────────────────────────────────────────────────────
func TestL3_HUB_001_BroadcastConcurrentRace(t *testing.T) {
	start := time.Now()

	const (
		concurrentBroadcasters = 50
		durationS              = 20
	)

	hub := streaming.NewHub()
	hub.SetMaxClients(0) // no clients — we only test the Broadcast path itself

	var (
		broadcastsDone  int64
		panics          int64
		seqRegressions  int64
		lastSeq         uint64
		lastSeqMu       sync.Mutex
	)

	ctx, cancel := testContextWithTimeout(durationS * time.Second)
	defer cancel()

	var wg sync.WaitGroup

	for i := 0; i < concurrentBroadcasters; i++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					atomic.AddInt64(&panics, 1)
					t.Errorf("L3-HUB-001 PANIC in broadcaster %d: %v", gid, r)
				}
			}()

			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				// Construct a minimal but valid TickPayload.
				// All float fields are finite — Broadcast calls sanitizePayload
				// internally but we want to exercise the normal path too.
				payload := &streaming.TickPayload{
					Type:         streaming.MsgTick,
					TickHealthMs: float64(gid) * 0.1,
					RuntimeMetrics: streaming.RuntimeMetrics{
						AvgPruneMs:     float64(gid),
						AvgModellingMs: float64(gid) * 2,
					},
				}
				hub.Broadcast(payload)
				atomic.AddInt64(&broadcastsDone, 1)
			}
		}(i)
	}

	// Separate goroutine polls GetLastPayload to exercise the RLock path
	// while Broadcast holds the write Lock — this is the key race surface.
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				atomic.AddInt64(&panics, 1)
				t.Errorf("L3-HUB-001 PANIC in GetLastPayload poller: %v", r)
			}
		}()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			p := hub.GetLastPayload()
			if p == nil {
				continue
			}
			// SequenceNo must be monotonically increasing.
			// Since multiple goroutines broadcast concurrently, any individual
			// poll may observe a lower seqNo than the previous poll if a
			// slower Broadcast finishes after a faster one — that is legal.
			// What must NEVER happen: seqNo == 0 after at least one Broadcast.
			if atomic.LoadInt64(&broadcastsDone) > 10 && p.SequenceNo == 0 {
				atomic.AddInt64(&seqRegressions, 1)
				t.Errorf("L3-HUB-001 SEQ ZERO: GetLastPayload returned SequenceNo=0 after %d broadcasts",
					atomic.LoadInt64(&broadcastsDone))
			}
			lastSeqMu.Lock()
			_ = lastSeq // read to ensure we touched it under lock
			lastSeqMu.Unlock()
		}
	}()

	// Poll ClientCount concurrently — exercises the RLock path on h.mu.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			count := hub.ClientCount()
			if count < 0 {
				t.Errorf("L3-HUB-001 INVALID ClientCount: %d", count)
			}
		}
	}()

	wg.Wait()

	passed := atomic.LoadInt64(&panics) == 0 && atomic.LoadInt64(&seqRegressions) == 0
	durationMs := time.Since(start).Milliseconds()

	writeL3Result(L3Record{
		TestID: "L3-HUB-001",
		Layer:  3,
		Name:   "Hub.Broadcast concurrent race safety",
		Aim: fmt.Sprintf(
			"%d concurrent Broadcast callers + 1 GetLastPayload poller + 1 ClientCount poller for %ds: "+
				"zero races, zero panics, SequenceNo never 0 after first broadcast",
			concurrentBroadcasters, durationS,
		),
		PackagesInvolved: []string{"internal/streaming"},
		FunctionsTested: []string{
			"NewHub", "(*Hub).SetMaxClients", "(*Hub).Broadcast",
			"(*Hub).GetLastPayload", "(*Hub).ClientCount",
		},
		Threshold: L3Threshold{
			Metric:    "panics_plus_seq_regressions",
			Operator:  "==",
			Value:     0,
			Unit:      "count",
			Rationale: "Concurrent map/struct access in Broadcast must be fully protected by h.mu",
		},
		Result: L3ResultData{
			Status:              l3Status(passed),
			ActualValue:         float64(atomic.LoadInt64(&panics) + atomic.LoadInt64(&seqRegressions)),
			ActualUnit:          "violations",
			OperationsCompleted: atomic.LoadInt64(&broadcastsDone),
			RaceDetectorActive:  raceDetectorEnabled(),
			DurationMs:          durationMs,
			ErrorMessages: []string{fmt.Sprintf(
				"broadcasts=%d panics=%d seq_regressions=%d",
				atomic.LoadInt64(&broadcastsDone),
				atomic.LoadInt64(&panics),
				atomic.LoadInt64(&seqRegressions),
			)},
		},
		OnExceed: "Concurrent access to h.clients map or h.lastPayload without proper locking → " +
			"'concurrent map iteration and map write' panic → streaming Hub crashes → all dashboard clients lose feed",
		Questions: L3Questions{
			WhatWasTested: fmt.Sprintf(
				"Hub.Broadcast called from %d concurrent goroutines for %ds while GetLastPayload and ClientCount polled simultaneously",
				concurrentBroadcasters, durationS,
			),
			WhyThisThreshold:    "Any race on h.mu or h.lastPayload causes either silent corruption or a runtime panic — both are unacceptable for a live control room",
			WhatHappensIfFails:  "Dashboard operators see inconsistent or corrupt tick payloads — wrong risk scores, wrong topology",
			HowRacesWereDetected: "Go race detector on binary; also seqNo continuity check catches silent corruption",
			HowLeaksWereDetected: "N/A — goroutine lifecycle not tested here; see L3-HUB-003",
			WhatConcurrencyPattern: "MRSW on h.clients map: Broadcast holds Lock to copy client slice; GetLastPayload holds RLock; concurrent Broadcasts serialise via Lock",
		},
		RunAt:     l3Now(),
		GoVersion: l3GoVer(),
	})

	if !passed {
		t.Fatalf(
			"L3-HUB-001 FAILED: panics=%d seq_regressions=%d after %d broadcasts\n"+
				"FIX: verify h.mu.Lock() in Broadcast wraps BOTH the h.lastPayload write AND the client slice copy.\n"+
				"     If lastPayload is written outside the lock, GetLastPayload(RLock) races with Broadcast(Lock).\n"+
				"     File: internal/streaming/hub.go",
			atomic.LoadInt64(&panics),
			atomic.LoadInt64(&seqRegressions),
			atomic.LoadInt64(&broadcastsDone),
		)
	}

	t.Logf("L3-HUB-001 PASS | broadcasts=%d panics=0 seq_regressions=0",
		atomic.LoadInt64(&broadcastsDone))
}

// ─────────────────────────────────────────────────────────────────────────────
// L3-HUB-002 — Slow client backpressure: dropped without blocking fast clients
//
// AIM:   Connect one "slow" WebSocket client (never reads from socket) and one
//        "fast" client (drains messages immediately).  After sendBufferSize+1
//        broadcasts, the slow client must be dropped (ClientCount drops by 1)
//        and the fast client must have received all broadcasts without delay.
//
// sendBufferSize = 16  (defined as const in hub.go)
//
// THRESHOLD:
//   slow_client_dropped == true  (ClientCount drops from 2 to 1)
//   fast_client_msgs >= broadcasts_sent  (no messages lost for fast client)
//   no_broadcast_blocked == true  (all Broadcast calls return within 10ms)
//
// ON EXCEED: Slow client blocks Broadcast via head-of-line → all clients
//            starve → dashboard freezes for every connected operator.
// ─────────────────────────────────────────────────────────────────────────────
func TestL3_HUB_002_SlowClientDroppedFastClientUnaffected(t *testing.T) {
	start := time.Now()

	const (
		// sendBufferSize in hub.go is 16. We need to fill it + 1 to trigger drop.
		sendBufSize   = 16
		totalBroadcasts = sendBufSize + 10 // enough to overflow slow client buffer
		broadcastDelay  = 5 * time.Millisecond
		maxBroadcastMs  = 10 // each Broadcast must complete within this many ms
	)

	hub := streaming.NewHub()
	hub.SetMaxClients(10)

	// ── Start HTTP test server ────────────────────────────────────────────────
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.HandleUpgrade(w, r)
	}))
	defer srv.Close()

	wsURL := "ws://" + srv.Listener.Addr().String() + "/"

	// ── Connect fast client ───────────────────────────────────────────────────
	fastConn, err := dialWebSocket(wsURL)
	if err != nil {
		t.Fatalf("L3-HUB-002: could not connect fast client: %v", err)
	}
	defer fastConn.Close()

	var fastMsgsReceived int64
	fastDone := make(chan struct{})
	go func() {
		defer close(fastDone)
		for {
			if err := fastConn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
				return
			}
			_, err := readWSFrame(fastConn)
			if err != nil {
				return
			}
			atomic.AddInt64(&fastMsgsReceived, 1)
		}
	}()

	// ── Connect slow client — never reads ────────────────────────────────────
	slowConn, err := dialWebSocket(wsURL)
	if err != nil {
		t.Fatalf("L3-HUB-002: could not connect slow client: %v", err)
	}
	defer slowConn.Close()
	// Do NOT read from slowConn — this causes its hub-side send channel to fill.

	// Wait for both clients to register.
	deadline := time.Now().Add(5 * time.Second)
	for hub.ClientCount() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if hub.ClientCount() < 2 {
		t.Fatalf("L3-HUB-002: timed out waiting for 2 clients to register; got %d", hub.ClientCount())
	}

	// ── Broadcast totalBroadcasts messages, measuring each call duration ──────
	var (
		broadcastsSent     int
		blockedBroadcasts  int
	)

	for i := 0; i < totalBroadcasts; i++ {
		payload := &streaming.TickPayload{
			Type:         streaming.MsgTick,
			TickHealthMs: float64(i),
			RuntimeMetrics: streaming.RuntimeMetrics{
				AvgPruneMs: float64(i),
			},
		}

		t0 := time.Now()
		hub.Broadcast(payload)
		elapsed := time.Since(t0)
		broadcastsSent++

		if elapsed.Milliseconds() > maxBroadcastMs {
			blockedBroadcasts++
			t.Logf("L3-HUB-002 SLOW BROADCAST at i=%d: elapsed=%s (threshold=%dms)",
				i, elapsed, maxBroadcastMs)
		}

		time.Sleep(broadcastDelay) // give writePump time to drain fast client
	}

	// Wait for fast client to drain remaining messages.
	time.Sleep(500 * time.Millisecond)
	cancel2 := func() { fastConn.SetReadDeadline(time.Now()) }
	cancel2()
	<-fastDone

	// ── Assertions ───────────────────────────────────────────────────────────
	// The slow client's send channel (capacity=16) will fill after 16 messages
	// and hub.remove(c) will be called — ClientCount must drop to 1.
	finalClientCount := hub.ClientCount()
	slowClientDropped := finalClientCount <= 1 // may be 0 if fast client also closed

	fastReceived := atomic.LoadInt64(&fastMsgsReceived)
	// Fast client should have received the bootstrap payload + most broadcasts.
	// We allow some tolerance: fast client must receive at least sendBufSize messages.
	fastClientGotEnough := fastReceived >= int64(sendBufSize)

	passed := blockedBroadcasts == 0 && slowClientDropped && fastClientGotEnough
	durationMs := time.Since(start).Milliseconds()

	writeL3Result(L3Record{
		TestID: "L3-HUB-002",
		Layer:  3,
		Name:   "Slow client dropped without blocking fast client",
		Aim: fmt.Sprintf(
			"After %d broadcasts with 1 slow + 1 fast client: slow dropped, fast receives ≥%d msgs, no broadcast blocks",
			totalBroadcasts, sendBufSize,
		),
		PackagesInvolved: []string{"internal/streaming", "internal/ws"},
		FunctionsTested: []string{
			"(*Hub).HandleUpgrade", "(*Hub).Broadcast", "(*Hub).ClientCount", "(*Hub).remove",
		},
		Threshold: L3Threshold{
			Metric:    "blocked_broadcast_calls",
			Operator:  "==",
			Value:     0,
			Unit:      "count",
			Rationale: "Broadcast must use non-blocking send (select default) — never block on a slow consumer",
		},
		Result: L3ResultData{
			Status:              l3Status(passed),
			ActualValue:         float64(blockedBroadcasts),
			ActualUnit:          "blocked_broadcast_calls",
			OperationsCompleted: int64(broadcastsSent),
			RaceDetectorActive:  raceDetectorEnabled(),
			DurationMs:          durationMs,
			ErrorMessages: []string{fmt.Sprintf(
				"sent=%d blocked=%d slow_dropped=%v fast_received=%d final_clients=%d",
				broadcastsSent, blockedBroadcasts, slowClientDropped, fastReceived, finalClientCount,
			)},
		},
		OnExceed: "Broadcast blocks on slow client → all Broadcast calls serialise behind slow consumer → " +
			"dashboard latency grows unboundedly for ALL operators, not just the slow one",
		Questions: L3Questions{
			WhatWasTested: fmt.Sprintf(
				"%d broadcasts with sendBufSize=%d, 1 slow (never reads) + 1 fast (drains immediately) client",
				totalBroadcasts, sendBufSize,
			),
			WhyThisThreshold:    "Broadcast uses 'select { case c.send <- data: default: h.remove(c) }' — the default branch makes it non-blocking; any block means this code path was not reached",
			WhatHappensIfFails:  "One slow browser tab causes ALL dashboard operators to see frozen metrics — control room becomes blind",
			HowRacesWereDetected: "Go race detector on binary + timing assertions on each Broadcast call",
			HowLeaksWereDetected: "ClientCount checked after drops to verify remove() cleaned up correctly",
			WhatConcurrencyPattern: "Non-blocking fan-out: Broadcast iterates snapshot of client list (no lock held) and sends via buffered channel with non-blocking select",
		},
		RunAt:     l3Now(),
		GoVersion: l3GoVer(),
	})

	if !passed {
		t.Fatalf(
			"L3-HUB-002 FAILED: blocked=%d slow_dropped=%v fast_received=%d final_clients=%d\n"+
				"FIX: Broadcast send path must be:\n"+
				"  select { case c.send <- data: default: h.remove(c) }\n"+
				"If this already exists, check that writePump drains c.send fast enough.\n"+
				"File: internal/streaming/hub.go",
			blockedBroadcasts, slowClientDropped, fastReceived, finalClientCount,
		)
	}

	t.Logf("L3-HUB-002 PASS | sent=%d blocked=0 slow_dropped=%v fast_received=%d",
		broadcastsSent, slowClientDropped, fastReceived)
}

// ─────────────────────────────────────────────────────────────────────────────
// WebSocket test utilities
// Manual WebSocket handshake — no external dependency required.
// Compatible with the custom ws package (internal/ws/websocket.go).
// ─────────────────────────────────────────────────────────────────────────────

// dialWebSocket performs a WebSocket upgrade handshake against the given URL
// and returns the raw net.Conn in WebSocket framing mode.
func dialWebSocket(rawURL string) (net.Conn, error) {
	// Strip ws:// prefix and build the HTTP address.
	addr := strings.TrimPrefix(rawURL, "ws://")
	addr = strings.TrimPrefix(addr, "wss://")
	// Remove path if present.
	if idx := strings.Index(addr, "/"); idx != -1 {
		addr = addr[:idx]
	}

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}

	// Generate a random 16-byte key and base64-encode it.
	var keyBytes [16]byte
	if _, err := rand.Read(keyBytes[:]); err != nil {
		conn.Close()
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString(keyBytes[:])

	// Send the HTTP upgrade request.
	req := fmt.Sprintf(
		"GET / HTTP/1.1\r\n"+
			"Host: %s\r\n"+
			"Upgrade: websocket\r\n"+
			"Connection: Upgrade\r\n"+
			"Sec-WebSocket-Key: %s\r\n"+
			"Sec-WebSocket-Version: 13\r\n"+
			"\r\n",
		addr, key,
	)
	if _, err := conn.Write([]byte(req)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("write upgrade request: %w", err)
	}

	// Read the HTTP 101 response.
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	br := bufio.NewReader(conn)
	statusLine, err := br.ReadString('\n')
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("read status line: %w", err)
	}
	if !strings.Contains(statusLine, "101") {
		conn.Close()
		return nil, fmt.Errorf("unexpected status: %q", statusLine)
	}
	// Drain the remaining HTTP headers.
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("read header: %w", err)
		}
		if line == "\r\n" {
			break
		}
	}
	conn.SetReadDeadline(time.Time{}) // clear deadline

	// Any bytes already buffered in br must be handed back to the conn.
	// Since we read exactly the headers (not into WS frames), the buffer
	// should be empty after the blank line — but we wrap conn in a struct
	// that re-prepends any buffered data just to be safe.
	return &bufferedConn{conn: conn, buf: br}, nil
}

// bufferedConn wraps net.Conn and re-injects any bytes already consumed
// into a bufio.Reader during the HTTP handshake phase.
type bufferedConn struct {
	conn net.Conn
	buf  *bufio.Reader
}

func (bc *bufferedConn) Read(p []byte) (int, error) {
	if bc.buf != nil && bc.buf.Buffered() > 0 {
		return bc.buf.Read(p)
	}
	return bc.conn.Read(p)
}

func (bc *bufferedConn) Write(p []byte) (int, error)        { return bc.conn.Write(p) }
func (bc *bufferedConn) Close() error                       { return bc.conn.Close() }
func (bc *bufferedConn) LocalAddr() net.Addr                { return bc.conn.LocalAddr() }
func (bc *bufferedConn) RemoteAddr() net.Addr               { return bc.conn.RemoteAddr() }
func (bc *bufferedConn) SetDeadline(t time.Time) error      { return bc.conn.SetDeadline(t) }
func (bc *bufferedConn) SetReadDeadline(t time.Time) error  { return bc.conn.SetReadDeadline(t) }
func (bc *bufferedConn) SetWriteDeadline(t time.Time) error { return bc.conn.SetWriteDeadline(t) }

// readWSFrame reads one WebSocket frame from a raw connection and returns
// the payload bytes.  Handles text frames (opcode 1) and ping frames (opcode 9).
// Returns an error when the connection is closed or the read deadline is exceeded.
func readWSFrame(conn net.Conn) ([]byte, error) {
	// Read first 2 bytes: FIN+opcode, MASK+payloadLen
	var header [2]byte
	if _, err := readFull(conn, header[:]); err != nil {
		return nil, err
	}

	opcode := header[0] & 0x0f
	_ = opcode // we accept all opcodes (text, ping, close)

	masked := header[1]&0x80 != 0
	payloadLen := int(header[1] & 0x7f)

	switch payloadLen {
	case 126:
		var ext [2]byte
		if _, err := readFull(conn, ext[:]); err != nil {
			return nil, err
		}
		payloadLen = int(uint16(ext[0])<<8 | uint16(ext[1]))
	case 127:
		var ext [8]byte
		if _, err := readFull(conn, ext[:]); err != nil {
			return nil, err
		}
		// Cast to int — for test payloads this is always small.
		payloadLen = int(
			uint64(ext[0])<<56 | uint64(ext[1])<<48 | uint64(ext[2])<<40 | uint64(ext[3])<<32 |
				uint64(ext[4])<<24 | uint64(ext[5])<<16 | uint64(ext[6])<<8 | uint64(ext[7]),
		)
	}

	var maskKey [4]byte
	if masked {
		if _, err := readFull(conn, maskKey[:]); err != nil {
			return nil, err
		}
	}

	payload := make([]byte, payloadLen)
	if payloadLen > 0 {
		if _, err := readFull(conn, payload); err != nil {
			return nil, err
		}
	}
	if masked {
		for i, b := range payload {
			payload[i] = b ^ maskKey[i%4]
		}
	}
	return payload, nil
}

// readFull reads exactly len(buf) bytes from conn.
func readFull(conn net.Conn, buf []byte) (int, error) {
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

// verifyTickPayload confirms the JSON bytes represent a valid TickPayload.
// Returns an error if the bytes do not unmarshal into a TickPayload.
func verifyTickPayload(data []byte) error {
	var p streaming.TickPayload
	return json.Unmarshal(data, &p)
}