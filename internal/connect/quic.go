package connect

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/quic-go/quic-go"
)

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

// dialQUIC dials a QUIC connection and opens a single bidi stream as net.Conn.
// When verbose is non-nil, DNS resolution and TLS details are logged.
func dialQUIC(ctx context.Context, addr string, verbose func(string, ...any)) (net.Conn, error) {
	if verbose == nil {
		verbose = func(string, ...any) {}
	}

	// Resolve DNS before dialing so we can log the results.
	host, port, _ := net.SplitHostPort(addr)
	if host == "" {
		host = addr
	}

	start := time.Now()
	ips, err := net.DefaultResolver.LookupHost(ctx, host)
	dnsElapsed := time.Since(start)
	if err != nil {
		verbose("dns lookup failed for %s after %s: %v", host, dnsElapsed, err)
		return nil, fmt.Errorf("DNS resolve %s: %w", host, err)
	}
	verbose("dns %s → [%s] (%s, %d result(s))", host, strings.Join(ips, ", "), dnsElapsed, len(ips))

	// Log reverse DNS for the first IP (helps identify NLB vs Cloudflare etc.)
	if len(ips) > 0 {
		names, _ := net.DefaultResolver.LookupAddr(ctx, ips[0])
		if len(names) > 0 {
			verbose("reverse dns %s → %s", ips[0], strings.Join(names, ", "))
		}
	}

	tlsConf := &tls.Config{
		NextProtos:         []string{"astro-connect"},
		InsecureSkipVerify: true, //nolint:gosec // TODO: proper CA validation for production
	}

	verbose("QUIC dial to %s:%s (TLS ALPN=astro-connect)", host, port)
	dialStart := time.Now()
	conn, err := quic.DialAddr(ctx, addr, tlsConf, &quic.Config{
		MaxIdleTimeout:  120 * time.Second,
		KeepAlivePeriod: 30 * time.Second,
	})
	if err != nil {
		verbose("QUIC dial failed after %s: %v", time.Since(dialStart), err)
		return nil, fmt.Errorf("QUIC dial %s: %w", addr, err)
	}
	verbose("QUIC handshake completed in %s", time.Since(dialStart))

	// Log connection details
	verbose("  local=%s  remote=%s", conn.LocalAddr(), conn.RemoteAddr())
	cs := conn.ConnectionState().TLS
	verbose("  tls: version=%x  alpn=%s  cipher=%x  server_name=%s",
		cs.Version, cs.NegotiatedProtocol, cs.CipherSuite, cs.ServerName)
	if len(cs.PeerCertificates) > 0 {
		cert := cs.PeerCertificates[0]
		verbose("  cert: subject=%s  issuer=%s  dns=%v  expires=%s",
			cert.Subject.CommonName, cert.Issuer.CommonName,
			cert.DNSNames, cert.NotAfter.Format(time.RFC3339))
	}

	verbose("opening QUIC bidi stream...")
	streamStart := time.Now()
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		verbose("QUIC stream open failed after %s: %v", time.Since(streamStart), err)
		_ = conn.CloseWithError(1, "failed to open stream")
		return nil, fmt.Errorf("QUIC open stream %s: %w", addr, err)
	}
	verbose("QUIC stream opened (id=%d) in %s", stream.StreamID(), time.Since(streamStart))

	return &quicConn{conn: conn, stream: stream}, nil
}
