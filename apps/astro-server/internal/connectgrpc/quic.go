package connectgrpc

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
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
		log.Info("QUIC-LB server ID configured",
			"server_id_hex", hex.EncodeToString(serverID[:]),
			"cid_len", quicLBCIDLen)
		tr.ConnectionIDGenerator = &quicLBConnIDGenerator{serverID: serverID}
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

// parseNLBServerID reads the AWS_LBC_QUIC_SERVER_ID env var (base64-encoded,
// 8 bytes) injected by the AWS Load Balancer Controller into QUIC-enabled pods.
func parseNLBServerID() ([8]byte, bool) {
	raw := os.Getenv("AWS_LBC_QUIC_SERVER_ID")
	if raw == "" {
		return [8]byte{}, false
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(decoded) != 8 {
		return [8]byte{}, false
	}
	var id [8]byte
	copy(id[:], decoded)
	return id, true
}

// quicLBConnIDGenerator produces QUIC connection IDs following the QUIC-LB
// spec used by AWS NLB for connection-ID-based routing.
//
// CID format (13 bytes):
//
//	Byte 0:      config rotation (3 bits) | length/random (5 bits)
//	Bytes 1-8:   Server ID (from AWS_LBC_QUIC_SERVER_ID)
//	Bytes 9-12:  Random nonce
//
// The NLB reads the DCID from short-header (1-RTT) packets, extracts
// bytes 1-8 as the server ID, and routes to the matching target.
type quicLBConnIDGenerator struct {
	serverID [8]byte
}

const quicLBCIDLen = 13 // 1 header + 8 server ID + 4 nonce

func (g *quicLBConnIDGenerator) GenerateConnectionID() (quic.ConnectionID, error) {
	cid := make([]byte, quicLBCIDLen)
	cid[0] = 0x00 // config rotation = 0, plaintext scheme
	copy(cid[1:9], g.serverID[:])
	if _, err := rand.Read(cid[9:]); err != nil {
		return quic.ConnectionID{}, fmt.Errorf("generate connection ID: %w", err)
	}
	return quic.ConnectionIDFromBytes(cid), nil
}

func (g *quicLBConnIDGenerator) ConnectionIDLen() int {
	return quicLBCIDLen
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
