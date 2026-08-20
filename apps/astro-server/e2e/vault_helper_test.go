//go:build integration || k8s

package e2e

import (
	"context"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/envelope"
)

func testVault(t *testing.T) *envelope.Vault {
	t.Helper()
	client, err := envelope.NewLocalKMSClient()
	if err != nil {
		t.Fatalf("local kms: %v", err)
	}
	return envelope.NewVault(client, "arn:aws:kms:test:000:key/test")
}

func testEncryptor(t *testing.T) *envelope.Encryptor {
	t.Helper()
	enc, err := testVault(t).Encryptor(context.Background())
	if err != nil {
		t.Fatalf("mint data key: %v", err)
	}
	return enc
}
