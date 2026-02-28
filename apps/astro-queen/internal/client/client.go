package client

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"

	adminv1 "github.com/postman/astro/packages/astro-proto/admin/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/postman/astro/apps/astro-queen/internal/config"
)

// Client wraps the AdminServiceClient with connection management.
type Client struct {
	conn adminv1.AdminServiceClient
	cc   *grpc.ClientConn
}

// New creates a new gRPC client from config.
// Uses mTLS if cert/key/CA files are configured, otherwise connects insecurely.
func New(cfg *config.Config) (*Client, error) {
	var opts []grpc.DialOption

	if cfg.CertFile != "" && cfg.KeyFile != "" && cfg.CAFile != "" {
		creds, err := loadClientTLS(cfg)
		if err != nil {
			return nil, fmt.Errorf("load TLS: %w", err)
		}
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// grpc.NewClient defaults to the "dns" resolver, which can produce zero
	// addresses for bare host:port targets on some systems. Use passthrough
	// to connect directly without service discovery.
	target := cfg.Server
	if !strings.Contains(target, "://") {
		target = "passthrough:///" + target
	}

	cc, err := grpc.NewClient(target, opts...)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", cfg.Server, err)
	}

	return &Client{
		conn: adminv1.NewAdminServiceClient(cc),
		cc:   cc,
	}, nil
}

// AdminService returns the underlying AdminServiceClient.
func (c *Client) AdminService() adminv1.AdminServiceClient {
	return c.conn
}

// Close closes the underlying connection.
func (c *Client) Close() error {
	return c.cc.Close()
}

func loadClientTLS(cfg *config.Config) (credentials.TransportCredentials, error) {
	cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load key pair: %w", err)
	}

	caBytes, err := os.ReadFile(cfg.CAFile)
	if err != nil {
		return nil, fmt.Errorf("read CA: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caBytes) {
		return nil, fmt.Errorf("parse CA certificate")
	}

	return credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS13,
	}), nil
}
