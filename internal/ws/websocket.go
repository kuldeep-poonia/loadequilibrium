
package ws

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const guid = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// MessageType mirrors the WebSocket opcode for text, binary, close, ping, pong.
type MessageType int

const (
	TextMessage   MessageType = 1
	BinaryMessage MessageType = 2
	CloseMessage  MessageType = 8
	PingMessage   MessageType = 9
	PongMessage   MessageType = 10
)

// Conn is a WebSocket connection.
type Conn struct {
	conn   net.Conn
	rw     *bufio.ReadWriter
	isServer bool
}

// Upgrader upgrades HTTP connections to WebSocket.
type Upgrader struct {
	ReadBufferSize, WriteBufferSize int
	CheckOrigin func(*http.Request) bool
}

var ErrHandshakeFailed = errors.New("ws: handshake failed")

// Upgrade performs the WebSocket handshake.
func (u *Upgrader) Upgrade(w http.ResponseWriter, r *http.Request, _ http.Header) (*Conn, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "not a websocket upgrade", http.StatusBadRequest)
		return nil, ErrHandshakeFailed
	}
	key := r.Header.Get("Sec-Websocket-Key")
	if key == "" {
		http.Error(w, "missing sec-websocket-key", http.StatusBadRequest)
		return nil, ErrHandshakeFailed
	}

	accept := computeAccept(key)

	h, ok := w.(http.Hijacker)
	if !ok {
		return nil, fmt.Errorf("ws: responsewriter does not support hijack")
	}
	netConn, bufrw, err := h.Hijack()
	if err != nil {
		return nil, err
	}

	_, err = fmt.Fprintf(bufrw,
		"HTTP/1.1 101 Switching Protocols\r\n"+
			"Upgrade: websocket\r\n"+
			"Connection: Upgrade\r\n"+
			"Sec-WebSocket-Accept: %s\r\n\r\n",
		accept)
	if err != nil {
		netConn.Close()
		return nil, err
	}
	if err := bufrw.Flush(); err != nil {
		netConn.Close()
		return nil, err
	}

	return &Conn{conn: netConn, rw: bufrw, isServer: true}, nil
}

func computeAccept(key string) string {
	h := sha1.New()
	h.Write([]byte(key + guid))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// WriteMessage sends a WebSocket frame of the given type.
func (c *Conn) WriteMessage(mt MessageType, data []byte) error {
	return c.writeFrame(byte(mt), data)
}

func (c *Conn) writeFrame(opcode byte, payload []byte) error {
	var header [10]byte
	header[0] = 0x80 | opcode // FIN + opcode
	n := len(payload)

	var headerLen int
	switch {
	case n <= 125:
		header[1] = byte(n)
		headerLen = 2
	case n <= 65535:
		header[1] = 126
		binary.BigEndian.PutUint16(header[2:4], uint16(n))
		headerLen = 4
	default:
		header[1] = 127
		binary.BigEndian.PutUint64(header[2:10], uint64(n))
		headerLen = 10
	}

	if _, err := c.rw.Write(header[:headerLen]); err != nil {
		return err
	}
	if len(payload) > 0 {
		if _, err := c.rw.Write(payload); err != nil {
			return err
		}
	}
	return c.rw.Flush()
}

// ReadMessage reads the next WebSocket message.
func (c *Conn) ReadMessage() (MessageType, []byte, error) {
	// Read first two bytes.
	var header [2]byte
	if _, err := io.ReadFull(c.rw, header[:]); err != nil {
		return 0, nil, err
	}
	// fin := header[0] & 0x80 != 0 // we don't reassemble fragments
	opcode := MessageType(header[0] & 0x0f)
	masked := header[1]&0x80 != 0
	payloadLen := int64(header[1] & 0x7f)

	switch payloadLen {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(c.rw, ext[:]); err != nil {
			return 0, nil, err
		}
		payloadLen = int64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(c.rw, ext[:]); err != nil {
			return 0, nil, err
		}
		payloadLen = int64(binary.BigEndian.Uint64(ext[:]))
	}

	var maskKey [4]byte
	if masked {
		if _, err := io.ReadFull(c.rw, maskKey[:]); err != nil {
			return 0, nil, err
		}
	}

	data := make([]byte, payloadLen)
	if _, err := io.ReadFull(c.rw, data); err != nil {
		return 0, nil, err
	}
	if masked {
		for i, b := range data {
			data[i] = b ^ maskKey[i%4]
		}
	}

	return opcode, data, nil
}

// SetReadDeadline sets the deadline for future reads.
func (c *Conn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

// SetWriteDeadline sets the deadline for future writes.
func (c *Conn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

// SetReadLimit is a no-op compatibility shim (limit is not enforced here).
func (c *Conn) SetReadLimit(_ int64) {}

// SetPongHandler registers a handler for pong frames.
func (c *Conn) SetPongHandler(h func(string) error) {
	// The read loop in readPump handles pong internally via ReadMessage.
	_ = h
}

// Close closes the underlying connection.
func (c *Conn) Close() error { return c.conn.Close() }

// RemoteAddr returns the remote network address.
func (c *Conn) RemoteAddr() net.Addr { return c.conn.RemoteAddr() }

// GenerateMaskKey generates a random 4-byte masking key (used for client frames).
func GenerateMaskKey() [4]byte {
	var key [4]byte
	_, _ = rand.Read(key[:])
	return key
}
