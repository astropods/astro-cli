package opauth

import (
	"context"
	"fmt"

	"github.com/1password/onepassword-sdk-go"
)

const (
	refClientCert = "op://Astro/queen-bee-client/client-cert"
	refClientKey  = "op://Astro/queen-bee-client/client-key"
	refCACert     = "op://Astro/queen-bee-client/ca-cert"
)

// FetchCerts resolves mTLS certificates from 1Password using the desktop app integration.
func FetchCerts(ctx context.Context, accountName string) (clientCert, clientKey, caCert string, err error) {
	client, err := onepassword.NewClient(ctx, onepassword.WithDesktopAppIntegration(accountName))
	if err != nil {
		return "", "", "", fmt.Errorf("1password client: %w", err)
	}

	clientCert, err = client.Secrets().Resolve(ctx, refClientCert)
	if err != nil {
		return "", "", "", fmt.Errorf("resolve client cert: %w", err)
	}

	clientKey, err = client.Secrets().Resolve(ctx, refClientKey)
	if err != nil {
		return "", "", "", fmt.Errorf("resolve client key: %w", err)
	}

	caCert, err = client.Secrets().Resolve(ctx, refCACert)
	if err != nil {
		return "", "", "", fmt.Errorf("resolve ca cert: %w", err)
	}

	return clientCert, clientKey, caCert, nil
}
