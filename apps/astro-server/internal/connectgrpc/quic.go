package connectgrpc

import (
	"context"
	"crypto/tls"
	"net"
	"time"

	"github.com/quic-go/quic-go"
)

// quicListener wraps a QUIC listener to satisfy net.Listener for gRPC.
// Each accepted QUIC connection opens a single bidirectional stream that
// is presented as a net.Conn.
type quicListener struct {
	ql   *quic.Listener
	addr net.Addr
}

// ListenQUIC creates a QUIC listener on the given UDP address.
func ListenQUIC(addr string, tlsConf *tls.Config) (net.Listener, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, err
	}

	ql, err := quic.Listen(udpConn, tlsConf, &quic.Config{
		MaxIdleTimeout:  120 * time.Second,
		KeepAlivePeriod: 30 * time.Second,
	})
	if err != nil {
		_ = udpConn.Close()
		return nil, err
	}

	return &quicListener{ql: ql, addr: udpConn.LocalAddr()}, nil
}

func (l *quicListener) Accept() (net.Conn, error) {
	conn, err := l.ql.Accept(context.Background())
	if err != nil {
		return nil, err
	}
	stream, err := conn.AcceptStream(context.Background())
	if err != nil {
		return nil, err
	}
	return &quicConn{conn: conn, stream: stream}, nil
}

func (l *quicListener) Close() error {
	return l.ql.Close()
}

func (l *quicListener) Addr() net.Addr {
	return l.addr
}

// quicConn wraps a QUIC stream as a net.Conn for gRPC transport.
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
