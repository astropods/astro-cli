package connectgrpc

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/postman/astro/apps/astro-server/internal/logger"
	"github.com/quic-go/quic-go"
)

// quicListener wraps a QUIC listener to satisfy net.Listener for gRPC.
// Each accepted QUIC connection opens a single bidirectional stream that
// is presented as a net.Conn.
//
// QUIC connections are accepted in a background goroutine so that a slow
// or stale connection (one that never opens a stream) cannot block the
// gRPC Serve() accept loop and starve other clients.
type quicListener struct {
	ql     *quic.Listener
	addr   net.Addr
	log    *logger.Logger
	connCh chan net.Conn
	errCh  chan error
}

// ListenQUIC creates a QUIC listener on the given UDP address.
// If the AWS_LBC_QUIC_SERVER_ID env var is set (injected by the AWS Load
// Balancer Controller), the listener embeds that server ID in every QUIC
// connection ID so the NLB can route packets to the correct target.
func ListenQUIC(addr string, tlsConf *tls.Config, log *logger.Logger) (net.Listener, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, err
	}

	quicConf := &quic.Config{
		MaxIdleTimeout:  120 * time.Second,
		KeepAlivePeriod: 30 * time.Second,
	}

	// Use quic.Transport so we can set a custom ConnectionIDGenerator
	// when running behind an AWS NLB with QUIC-enabled target groups.
	tr := &quic.Transport{Conn: udpConn}

	if serverID, ok := parseNLBServerID(); ok {
		log.Info("QUIC NLB server ID configured", "server_id", hex.EncodeToString(serverID))
		tr.ConnectionIDGenerator = &nlbConnIDGenerator{serverID: serverID}
	}

	ql, err := tr.Listen(tlsConf, quicConf)
	if err != nil {
		_ = udpConn.Close()
		return nil, err
	}

	log.Debug("QUIC listener ready", "addr", udpConn.LocalAddr().String())
	l := &quicListener{
		ql:     ql,
		addr:   udpConn.LocalAddr(),
		log:    log,
		connCh: make(chan net.Conn, 16),
		errCh:  make(chan error, 1),
	}
	go l.acceptLoop()
	return l, nil
}

// acceptLoop runs in the background, accepting QUIC connections and waiting
// for their first stream. This prevents a stale connection from blocking the
// gRPC Serve() accept loop.
func (l *quicListener) acceptLoop() {
	for {
		conn, err := l.ql.Accept(context.Background())
		if err != nil {
			l.log.Error("QUIC accept failed", "error", err)
			l.errCh <- err
			return
		}
		go l.handleNewConn(conn)
	}
}

func (l *quicListener) handleNewConn(conn *quic.Conn) {
	l.log.Debug("QUIC connection accepted", "remote", conn.RemoteAddr().String())

	streamCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	stream, err := conn.AcceptStream(streamCtx)
	cancel()
	if err != nil {
		l.log.Warn("QUIC stream accept failed, closing connection",
			"remote", conn.RemoteAddr().String(), "error", err)
		_ = conn.CloseWithError(1, "stream accept timeout")
		return
	}
	l.log.Debug("QUIC stream opened",
		"remote", conn.RemoteAddr().String(), "stream_id", stream.StreamID())
	l.connCh <- &quicConn{conn: conn, stream: stream}
}

func (l *quicListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.connCh:
		return conn, nil
	case err := <-l.errCh:
		return nil, err
	}
}

func (l *quicListener) Close() error {
	return l.ql.Close()
}

func (l *quicListener) Addr() net.Addr {
	return l.addr
}

// ---------------------------------------------------------------------------
// NLB QUIC Connection ID Generator
// ---------------------------------------------------------------------------

// parseNLBServerID reads the AWS_LBC_QUIC_SERVER_ID env var (hex-encoded)
// injected by the AWS Load Balancer Controller into QUIC-enabled pods.
func parseNLBServerID() ([]byte, bool) {
	raw := os.Getenv("AWS_LBC_QUIC_SERVER_ID")
	if raw == "" {
		return nil, false
	}
	id, err := hex.DecodeString(raw)
	if err != nil || len(id) == 0 {
		return nil, false
	}
	return id, true
}

// nlbConnIDGenerator produces QUIC connection IDs that embed the NLB server
// ID as a prefix. The NLB extracts this prefix from short-header (1-RTT)
// packets to route them to the correct target.
type nlbConnIDGenerator struct {
	serverID []byte // typically 3 bytes from AWS LBC
}

func (g *nlbConnIDGenerator) GenerateConnectionID() (quic.ConnectionID, error) {
	// Total connection ID: server ID prefix + 8 random bytes
	buf := make([]byte, len(g.serverID)+8)
	copy(buf, g.serverID)
	if _, err := rand.Read(buf[len(g.serverID):]); err != nil {
		return quic.ConnectionID{}, fmt.Errorf("generate connection ID: %w", err)
	}
	return quic.ConnectionIDFromBytes(buf), nil
}

func (g *nlbConnIDGenerator) ConnectionIDLen() int {
	return len(g.serverID) + 8
}

// ---------------------------------------------------------------------------
// quicConn — net.Conn wrapper around a QUIC stream
// ---------------------------------------------------------------------------

type quicConn struct {
	conn   *quic.Conn
	stream *quic.Stream
}

func (c *quicConn) Read(b []byte) (int, error)  { return c.stream.Read(b) }
func (c *quicConn) Write(b []byte) (int, error) { return c.stream.Write(b) }

func (c *quicConn) Close() error {
	c.stream.CancelRead(0)
	return c.stream.Close()
}

func (c *quicConn) LocalAddr() net.Addr  { return c.conn.LocalAddr() }
func (c *quicConn) RemoteAddr() net.Addr { return c.conn.RemoteAddr() }

func (c *quicConn) SetDeadline(t time.Time) error {
	if err := c.stream.SetReadDeadline(t); err != nil {
		return err
	}
	return c.stream.SetWriteDeadline(t)
}

func (c *quicConn) SetReadDeadline(t time.Time) error  { return c.stream.SetReadDeadline(t) }
func (c *quicConn) SetWriteDeadline(t time.Time) error { return c.stream.SetWriteDeadline(t) }
