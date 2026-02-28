package admingrpc

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"

	"google.golang.org/grpc/credentials"
)

// TLSConfig holds TLS certificate material for the gRPC server.
// Each field accepts either a file path or inline PEM content
// (detected by a "-----BEGIN" prefix).
type TLSConfig struct {
	CertFile string
	KeyFile  string
	CAFile   string
}

// loadPEM returns PEM bytes from val. If val starts with "-----BEGIN" it is
// treated as inline PEM; otherwise it is read as a file path.
func loadPEM(val string) ([]byte, error) {
	if strings.HasPrefix(val, "-----BEGIN") {
		return []byte(val), nil
	}
	return os.ReadFile(val) //nolint:gosec // path comes from server config, not user input
}

// ServerCredentials builds mTLS server credentials from cert/key/CA.
// Values can be file paths or inline PEM. Returns nil when any value is empty
// (no TLS — for development only).
func ServerCredentials(cfg TLSConfig) (credentials.TransportCredentials, error) {
	if cfg.CertFile == "" || cfg.KeyFile == "" || cfg.CAFile == "" {
		return nil, nil
	}

	certPEM, err := loadPEM(cfg.CertFile)
	if err != nil {
		return nil, fmt.Errorf("load server cert: %w", err)
	}
	keyPEM, err := loadPEM(cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load server key: %w", err)
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse server key pair: %w", err)
	}

	caBytes, err := loadPEM(cfg.CAFile)
	if err != nil {
		return nil, fmt.Errorf("load CA: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caBytes) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
		MinVersion:   tls.VersionTLS13,
	}

	return credentials.NewTLS(tlsCfg), nil
}

// ClientCredentials builds mTLS client credentials from cert/key/CA.
// Values can be file paths or inline PEM. Returns nil when any value is empty
// (insecure — for development only).
func ClientCredentials(cfg TLSConfig) (credentials.TransportCredentials, error) {
	if cfg.CertFile == "" || cfg.KeyFile == "" || cfg.CAFile == "" {
		return nil, nil
	}

	certPEM, err := loadPEM(cfg.CertFile)
	if err != nil {
		return nil, fmt.Errorf("load client cert: %w", err)
	}
	keyPEM, err := loadPEM(cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load client key: %w", err)
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse client key pair: %w", err)
	}

	caBytes, err := loadPEM(cfg.CAFile)
	if err != nil {
		return nil, fmt.Errorf("load CA: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caBytes) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS13,
	}

	return credentials.NewTLS(tlsCfg), nil
}
