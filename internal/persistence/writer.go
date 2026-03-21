// Package persistence provides async write-behind snapshot persistence to
// PostgreSQL. When DATABASE_URL is empty, the Writer is a no-op so the
// engine runs fully without any database dependency.
//
// A minimal PostgreSQL simple-query-protocol driver is included here so the
// binary requires zero external packages.
package persistence

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/loadequilibrium/loadequilibrium/internal/modelling"
	"github.com/loadequilibrium/loadequilibrium/internal/optimisation"
	"github.com/loadequilibrium/loadequilibrium/internal/reasoning"
	"github.com/loadequilibrium/loadequilibrium/internal/simulation"
	"github.com/loadequilibrium/loadequilibrium/internal/topology"
)

// ── Driver registration (once) ────────────────────────────────────────────

var registerOnce sync.Once

func registerDriver() {
	registerOnce.Do(func() {
		sql.Register("pgmin", &pgDriver{})
	})
}

// ── Domain types ──────────────────────────────────────────────────────────

// Snapshot is the persisted aggregate of a single engine tick.
type Snapshot struct {
	TickAt    time.Time
	Bundles   map[string]*modelling.ServiceModelBundle
	Topo      topology.GraphSnapshot
	Objective optimisation.ObjectiveScore
	Events    []reasoning.Event
	SimResult *simulation.SimulationResult
}

// ── Writer ────────────────────────────────────────────────────────────────

// Writer asynchronously writes engine snapshots to PostgreSQL.
// Database operations are fully off the hot path; the engine is never blocked.
type Writer struct {
	db    *sql.DB
	queue chan Snapshot
	done  chan struct{}
}

// NewWriter opens a PostgreSQL connection and starts the background drain goroutine.
// Returns nil (with a log line) when the database is unreachable.
func NewWriter(dsn string, bufSize int) *Writer {
	registerDriver()

	db, err := sql.Open("pgmin", dsn)
	if err != nil {
		log.Printf("[persistence] open error: %v", err)
		return nil
	}
	db.SetMaxOpenConns(3)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		log.Printf("[persistence] ping failed: %v", err)
		_ = db.Close()
		return nil
	}

	w := &Writer{
		db:    db,
		queue: make(chan Snapshot, bufSize),
		done:  make(chan struct{}),
	}

	if err := w.migrate(); err != nil {
		log.Printf("[persistence] migration error: %v", err)
		_ = db.Close()
		return nil
	}

	go w.drain()
	return w
}

// Enqueue submits a snapshot for async persistence.
// Drops silently if the internal queue is full — the engine is never blocked.
func (w *Writer) Enqueue(s Snapshot) {
	if w == nil {
		return
	}
	select {
	case w.queue <- s:
	default:
	}
}

// Close drains pending snapshots and shuts the writer down cleanly.
func (w *Writer) Close() {
	if w == nil {
		return
	}
	close(w.queue)
	<-w.done
	_ = w.db.Close()
}

func (w *Writer) drain() {
	defer close(w.done)
	for snap := range w.queue {
		if err := w.write(snap); err != nil {
			log.Printf("[persistence] write error: %v", err)
		}
	}
}

func (w *Writer) write(s Snapshot) error {
	bundlesJSON, _ := json.Marshal(s.Bundles)
	topoJSON, _ := json.Marshal(s.Topo)
	objJSON, _ := json.Marshal(s.Objective)
	eventsJSON, _ := json.Marshal(s.Events)
	var simJSON []byte
	if s.SimResult != nil {
		simJSON, _ = json.Marshal(s.SimResult)
	}
	_, err := w.db.Exec(
		`INSERT INTO engine_snapshots
			(tick_at, bundles, topology, objective, events, sim_result)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		s.TickAt,
		string(bundlesJSON),
		string(topoJSON),
		string(objJSON),
		string(eventsJSON),
		string(simJSON),
	)
	return err
}

func (w *Writer) migrate() error {
	_, err := w.db.Exec(`
		CREATE TABLE IF NOT EXISTS engine_snapshots (
			id         BIGSERIAL PRIMARY KEY,
			tick_at    TIMESTAMPTZ NOT NULL,
			bundles    JSONB,
			topology   JSONB,
			objective  JSONB,
			events     JSONB,
			sim_result JSONB
		);
		CREATE INDEX IF NOT EXISTS idx_engine_snapshots_tick
			ON engine_snapshots (tick_at DESC);
	`)
	return err
}

// ── Minimal PostgreSQL wire-protocol driver (stdlib only) ─────────────────
// Implements driver.Driver + driver.Conn + driver.Stmt for the simple-query
// protocol (Q messages). Sufficient for INSERT and DDL; no full query support.

type pgDriver struct{}

func (d *pgDriver) Open(dsn string) (driver.Conn, error) {
	cfg := parseDSN(dsn)
	addr := cfg["host"] + ":" + cfg["port"]
	nc, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("pgmin: dial %s: %w", addr, err)
	}
	c := &pgConn{nc: nc}
	if err := c.startup(cfg); err != nil {
		_ = nc.Close()
		return nil, err
	}
	return c, nil
}

// parseDSN handles both postgres://user:pass@host:port/db and key=value forms.
func parseDSN(dsn string) map[string]string {
	cfg := map[string]string{
		"host": "localhost", "port": "5432",
		"user": "postgres", "password": "", "dbname": "postgres",
	}
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		rest := strings.SplitN(dsn, "://", 2)[1]
		// Strip query string.
		rest = strings.SplitN(rest, "?", 2)[0]
		if at := strings.LastIndex(rest, "@"); at >= 0 {
			userInfo := rest[:at]
			rest = rest[at+1:]
			if c := strings.Index(userInfo, ":"); c >= 0 {
				cfg["user"] = userInfo[:c]
				cfg["password"] = userInfo[c+1:]
			} else {
				cfg["user"] = userInfo
			}
		}
		if slash := strings.Index(rest, "/"); slash >= 0 {
			cfg["dbname"] = rest[slash+1:]
			rest = rest[:slash]
		}
		if c := strings.LastIndex(rest, ":"); c >= 0 {
			cfg["host"] = rest[:c]
			cfg["port"] = rest[c+1:]
		} else if rest != "" {
			cfg["host"] = rest
		}
		return cfg
	}
	for _, pair := range strings.Fields(dsn) {
		if kv := strings.SplitN(pair, "=", 2); len(kv) == 2 {
			cfg[kv[0]] = kv[1]
		}
	}
	return cfg
}

// pgConn is a single PostgreSQL connection using the simple-query protocol.
type pgConn struct{ nc net.Conn }

func (c *pgConn) Close() error { return c.nc.Close() }

func (c *pgConn) Begin() (driver.Tx, error) {
	return nil, fmt.Errorf("pgmin: transactions not supported")
}

func (c *pgConn) Prepare(query string) (driver.Stmt, error) {
	return &pgStmt{conn: c, query: query}, nil
}

// startup sends the startup message and reads until ReadyForQuery.
func (c *pgConn) startup(cfg map[string]string) error {
	// Startup message: int32(length) + int32(version=196608) + key\0val\0... + \0
	params := "user\x00" + cfg["user"] + "\x00" +
		"database\x00" + cfg["dbname"] + "\x00\x00"
	body := make([]byte, 4+len(params))
	// Protocol version 3.0 = 0x00030000
	body[0], body[1], body[2], body[3] = 0, 3, 0, 0
	copy(body[4:], params)
	length := uint32(len(body) + 4)
	msg := make([]byte, 4+len(body))
	msg[0] = byte(length >> 24); msg[1] = byte(length >> 16)
	msg[2] = byte(length >> 8);  msg[3] = byte(length)
	copy(msg[4:], body)
	if _, err := c.nc.Write(msg); err != nil {
		return err
	}
	return c.readUntilReady(cfg["password"])
}

func (c *pgConn) readUntilReady(password string) error {
	for {
		mt, data, err := c.readMsg()
		if err != nil {
			return err
		}
		switch mt {
		case 'R': // AuthenticationRequest
			if len(data) < 4 {
				return fmt.Errorf("pgmin: short auth")
			}
			authType := int32(data[0])<<24 | int32(data[1])<<16 | int32(data[2])<<8 | int32(data[3])
			if authType == 0 {
				continue // AuthenticationOk
			}
			if authType == 3 { // CleartextPassword
				pw := password + "\x00"
				m := make([]byte, 1+4+len(pw))
				m[0] = 'p'
				l := 4 + len(pw)
				m[1] = byte(l >> 24); m[2] = byte(l >> 16)
				m[3] = byte(l >> 8);  m[4] = byte(l)
				copy(m[5:], pw)
				if _, err := c.nc.Write(m); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("pgmin: unsupported auth %d", authType)
		case 'Z': // ReadyForQuery
			return nil
		case 'E':
			return fmt.Errorf("pgmin: startup error: %s", pgErrMsg(data))
		case 'S', 'K': // ParameterStatus, BackendKeyData — ignore
		}
	}
}

// simpleQuery sends a Q (simple query) message and drains the response.
func (c *pgConn) simpleQuery(sql string) error {
	q := sql + "\x00"
	l := 4 + len(q)
	msg := make([]byte, 1+4+len(q))
	msg[0] = 'Q'
	msg[1] = byte(l >> 24); msg[2] = byte(l >> 16)
	msg[3] = byte(l >> 8);  msg[4] = byte(l)
	copy(msg[5:], q)
	if _, err := c.nc.Write(msg); err != nil {
		return err
	}
	for {
		mt, data, err := c.readMsg()
		if err != nil {
			return err
		}
		switch mt {
		case 'Z': // ReadyForQuery
			return nil
		case 'E':
			return fmt.Errorf("pgmin: query error: %s", pgErrMsg(data))
		case 'C', 'T', 'D', 'I', 'N', 'S': // CommandComplete, RowDesc, DataRow, EmptyQuery, Notice, ParameterStatus
		}
	}
}

// readMsg reads one backend message (1-byte type + 4-byte length + payload).
func (c *pgConn) readMsg() (byte, []byte, error) {
	hdr := make([]byte, 5)
	if _, err := io.ReadFull(c.nc, hdr); err != nil {
		return 0, nil, err
	}
	n := int(hdr[1])<<24 | int(hdr[2])<<16 | int(hdr[3])<<8 | int(hdr[4])
	n -= 4 // length field includes itself
	if n < 0 {
		n = 0
	}
	data := make([]byte, n)
	if n > 0 {
		if _, err := io.ReadFull(c.nc, data); err != nil {
			return 0, nil, err
		}
	}
	return hdr[0], data, nil
}

func pgErrMsg(data []byte) string {
	for i := 0; i < len(data)-1; {
		field := data[i]; i++
		end := i
		for end < len(data) && data[end] != 0 { end++ }
		if field == 'M' {
			return string(data[i:end])
		}
		i = end + 1
	}
	return "unknown"
}

// pgStmt implements driver.Stmt using simple-query with literal substitution.
type pgStmt struct {
	conn  *pgConn
	query string
}

func (s *pgStmt) Close() error  { return nil }
func (s *pgStmt) NumInput() int { return strings.Count(s.query, "$") }

func (s *pgStmt) Exec(args []driver.Value) (driver.Result, error) {
	q := substituteArgs(s.query, args)
	if err := s.conn.simpleQuery(q); err != nil {
		return nil, err
	}
	return driver.RowsAffected(1), nil
}

func (s *pgStmt) Query(_ []driver.Value) (driver.Rows, error) {
	return nil, fmt.Errorf("pgmin: Query rows not supported")
}

// substituteArgs replaces $1..$N with safely-escaped literal values.
func substituteArgs(query string, args []driver.Value) string {
	for i, arg := range args {
		ph := "$" + strconv.Itoa(i+1)
		var lit string
		switch v := arg.(type) {
		case nil:
			lit = "NULL"
		case int64:
			lit = strconv.FormatInt(v, 10)
		case float64:
			lit = strconv.FormatFloat(v, 'f', -1, 64)
		case bool:
			if v { lit = "TRUE" } else { lit = "FALSE" }
		case []byte:
			lit = pgLiteral(string(v))
		case string:
			lit = pgLiteral(v)
		case time.Time:
			lit = pgLiteral(v.UTC().Format(time.RFC3339Nano))
		default:
			lit = pgLiteral(fmt.Sprintf("%v", v))
		}
		query = strings.Replace(query, ph, lit, 1)
	}
	return query
}

func pgLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
