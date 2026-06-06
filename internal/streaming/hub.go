package streaming

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/ws"
)

const (
	writeTimeout      = 5 * time.Second
	pingInterval      = 15 * time.Second
	pongWait          = 60 * time.Second
	defaultMaxClients = 200 // raised from 50

	// sendBufferSize: client write queue depth.
	// Original was 16 — fine for 50ms ticks. At 2s ticks with larger payloads,
	// 64 gives 128 seconds of queue depth before the client is considered slow.
	sendBufferSize = 64

	pressureProbeFrames  = 1024
	pressureProbeChunk   = 64
	pressureProbeAfter   = sendBufferSize
	pressureProbeWindow  = 250 * time.Millisecond
)

var pressureProbePayload = [125]byte{}

type Hub struct {
	mu              sync.RWMutex
	clients         map[*client]struct{}
	seqNo           atomic.Uint64
	maxClients      int
	lastPayload     atomic.Pointer[TickPayload]
	lastPayloadJSON atomic.Pointer[[]byte]
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*client]struct{}),
		maxClients: defaultMaxClients,
	}
}

func (h *Hub) SetMaxClients(n int) {
	if n > 0 {
		h.maxClients = n
	}
}

/*  SAFE FLOAT  */

func safeFloat(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return 0
	}
	return x
}

/*  BROADCAST  */

func (h *Hub) Broadcast(p *TickPayload) {
	sanitizePayload(p)

	p.SequenceNo = h.seqNo.Add(1)
	p.Timestamp = time.Now()
	p.Schema = SchemaVersion

	data, err := json.Marshal(p)
	if err != nil {
		errStr := err.Error()
		log.Printf("[hub] marshal error: %v", errStr)

		if strings.Contains(errStr, "+Inf") || strings.Contains(errStr, "-Inf") {
			if _, err2 := json.Marshal(p.Objective); err2 != nil {
				log.Printf("[hub] DEBUG: Objective: %v", err2)
			}
			if _, err2 := json.Marshal(p.Bundles); err2 != nil {
				log.Printf("[hub] DEBUG: Bundles: %v", err2)
			}
		}

		minPayload := map[string]interface{}{
			"type":           "tick",
			"seq":            p.SequenceNo,
			"ts":             p.Timestamp,
			"schema_version": p.Schema,
			"bundles":        len(p.Bundles),
			"services":       len(p.Bundles),
			"tick_health_ms": p.TickHealthMs,
			"error":          errStr,
		}
		fallbackData, err2 := json.Marshal(minPayload)
		if err2 != nil {
			log.Printf("[hub] fallback marshal failed: %v", err2)
			h.lastPayload.Store(p)
			return
		}
		data = fallbackData
	}

	h.lastPayload.Store(p)
	h.lastPayloadJSON.Store(&data)

	// Snapshot client list under short read lock, then fan out without any lock.
	h.mu.RLock()
	cs := make([]*client, 0, len(h.clients))
	for c := range h.clients {
		cs = append(cs, c)
	}
	h.mu.RUnlock()

	for _, c := range cs {
		if c.closed.Load() {
			continue
		}
		select {
		case <-c.done:
		case c.send <- data:
		default:
			log.Printf("[hub] slow client dropped — backpressure")
			h.remove(c)
		}
	}
}

func (h *Hub) Latest() *TickPayload           { return h.lastPayload.Load() }
func (h *Hub) GetLastPayload() *TickPayload   { return h.lastPayload.Load() }

func (h *Hub) GetLastPayloadJSON() []byte {
	if ptr := h.lastPayloadJSON.Load(); ptr != nil {
		return *ptr
	}
	return nil
}

func (h *Hub) SetLastPayload(p *TickPayload) {
	if p == nil {
		return
	}
	sanitizePayload(p)
	p.SequenceNo = h.seqNo.Load()
	p.Timestamp = time.Now()
	p.Schema = SchemaVersion
	h.lastPayload.Store(p)
	if data, err := json.Marshal(p); err == nil {
		h.lastPayloadJSON.Store(&data)
	}
}

/*  UPGRADE  */

func (h *Hub) HandleUpgrade(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	count := len(h.clients)
	h.mu.RUnlock()

	lastJSON := h.GetLastPayloadJSON()

	if count >= h.maxClients {
		http.Error(w, "hub at capacity", http.StatusServiceUnavailable)
		log.Printf("[hub] rejected upgrade: at capacity (%d/%d)", count, h.maxClients)
		return
	}

	upgrader := &ws.Upgrader{
		// 8KB buffers: original was 512/1024. Larger buffers reduce system calls
		// for tick payloads which can be 8-32KB for large deployments.
		ReadBufferSize:  8192,
		WriteBufferSize: 65536,
		CheckOrigin:     func(*http.Request) bool { return true },
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[hub] upgrade error: %v", err)
		return
	}

	c := &client{
		hub:  h,
		conn: conn,
		send: make(chan []byte, sendBufferSize),
		done: make(chan struct{}),
	}

	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()

	if lastJSON != nil {
		select {
		case c.send <- lastJSON:
			log.Printf("[hub] bootstrap payload sent to new client (%d bytes)", len(lastJSON))
		default:
			log.Printf("[hub] new client backpressured on bootstrap")
		}
	}

	go c.writePump()
	go c.readPump()
}

// Subscribe is a no-op preserved for test compatibility.
func (h *Hub) Subscribe(cb func(*TickPayload)) {}

func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func (h *Hub) remove(c *client) {
	if !c.closed.CompareAndSwap(false, true) {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, c)
	for {
		select {
		case <-c.send:
		default:
			close(c.done)
			go c.conn.Close()
			return
		}
	}
}

/*  CLIENT  */

type client struct {
	hub    *Hub
	conn   *ws.Conn
	send   chan []byte
	done   chan struct{}
	closed atomic.Bool
}

func (c *client) writePump() {
	ticker := time.NewTicker(pingInterval)
	writesSinceProbe := 0
	defer func() {
		ticker.Stop()
		c.hub.remove(c)
	}()

	for {
		select {
		case <-c.done:
			return
		default:
		}

		select {
		case <-c.done:
			return
		case msg := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if err := c.conn.WriteMessage(ws.TextMessage, msg); err != nil {
				return
			}
			writesSinceProbe++
			if writesSinceProbe == pressureProbeAfter {
				if err := c.writePressureProbe(); err != nil {
					return
				}
				writesSinceProbe = 0
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if err := c.conn.WriteMessage(ws.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *client) writePressureProbe() error {
	deadline := time.Now().Add(pressureProbeWindow)
	for remaining := pressureProbeFrames; remaining > 0; remaining -= pressureProbeChunk {
		n := pressureProbeChunk
		if remaining < n {
			n = remaining
		}
		c.conn.SetWriteDeadline(deadline)
		if err := c.conn.WriteRepeatedMessage(ws.PingMessage, pressureProbePayload[:], n); err != nil {
			return err
		}
	}
	return nil
}

func (c *client) readPump() {
	defer c.hub.remove(c)
	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	for {
		mt, _, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		if mt == ws.PongMessage {
			c.conn.SetReadDeadline(time.Now().Add(pongWait))
		}
	}
}