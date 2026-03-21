package streaming

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/ws"
)

const (
	writeTimeout   = 5 * time.Second
	pingInterval   = 15 * time.Second
	sendBufferSize = 16
	pongWait       = 60 * time.Second
	defaultMaxClients = 50
)

// Hub manages connected WebSocket clients and broadcasts tick payloads.
// Client count is capped to prevent cardinality explosion.
type Hub struct {
	mu         sync.RWMutex
	clients    map[*client]struct{}
	seqNo      atomic.Uint64
	maxClients int
}

// NewHub creates an empty Hub with the default client cap.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*client]struct{}),
		maxClients: defaultMaxClients,
	}
}

// SetMaxClients configures the maximum number of concurrent WebSocket clients.
func (h *Hub) SetMaxClients(n int) {
	if n > 0 {
		h.maxClients = n
	}
}

// Broadcast serialises the TickPayload and sends it to all connected clients.
// Slow clients whose send buffer is full are dropped immediately.
func (h *Hub) Broadcast(p *TickPayload) {
	p.SequenceNo = h.seqNo.Add(1)
	p.Timestamp = time.Now()
	p.Schema = SchemaVersion

	data, err := json.Marshal(p)
	if err != nil {
		log.Printf("[hub] marshal error: %v", err)
		return
	}

	h.mu.RLock()
	cs := make([]*client, 0, len(h.clients))
	for c := range h.clients {
		cs = append(cs, c)
	}
	h.mu.RUnlock()

	for _, c := range cs {
		select {
		case c.send <- data:
		default:
			log.Printf("[hub] slow client dropped — backpressure")
			h.remove(c)
		}
	}
}

// HandleUpgrade upgrades an HTTP connection to WebSocket and registers the client.
// Returns HTTP 503 when the hub is at capacity.
func (h *Hub) HandleUpgrade(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	count := len(h.clients)
	h.mu.RUnlock()

	if count >= h.maxClients {
		http.Error(w, "hub at capacity", http.StatusServiceUnavailable)
		log.Printf("[hub] rejected upgrade: at capacity (%d/%d)", count, h.maxClients)
		return
	}

	upgrader := &ws.Upgrader{
		ReadBufferSize:  512,
		WriteBufferSize: 32 * 1024,
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
	}

	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()

	go c.writePump()
	go c.readPump()
}

// ClientCount returns the number of currently connected clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func (h *Hub) remove(c *client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.send)
	}
	h.mu.Unlock()
}

// client represents one connected WebSocket session.
type client struct {
	hub  *Hub
	conn *ws.Conn
	send chan []byte
}

// writePump pumps messages from the send channel to the WebSocket connection.
func (c *client) writePump() {
	ticker := time.NewTicker(pingInterval)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if !ok {
				_ = c.conn.WriteMessage(ws.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(ws.TextMessage, msg); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if err := c.conn.WriteMessage(ws.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readPump detects client disconnection via read errors; handles pong frames.
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
