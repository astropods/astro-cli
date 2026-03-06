package connectgrpc

import (
	"crypto/tls"
	"fmt"
	"os"
	"strings"
)

// loadPEM returns PEM bytes from val. If val starts with "-----BEGIN" it is
// treated as inline PEM; otherwise it is read as a file path.
func loadPEM(val string) ([]byte, error) {
	if strings.HasPrefix(val, "-----BEGIN") {
		return []byte(val), nil
	}
	return os.ReadFile(val) //nolint:gosec // path comes from server config
}

// LoadTLSCert loads a TLS certificate from file paths or inline PEM.
func LoadTLSCert(certFile, keyFile string) (tls.Certificate, error) {
	certPEM, err := loadPEM(certFile)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("load cert: %w", err)
	}
	keyPEM, err := loadPEM(keyFile)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("load key: %w", err)
	}
	return tls.X509KeyPair(certPEM, keyPEM)
}

// NewTLSConfig creates a TLS config suitable for QUIC (TLS 1.3, ALPN "h3").
func NewTLSConfig(cert tls.Certificate) *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
		NextProtos:   []string{"astro-connect"},
	}
}
