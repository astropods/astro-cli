package aigateway

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/astropods/astro/apps/astro-server/internal/envelope"
)

func testVault(t *testing.T) *envelope.Vault {
	t.Helper()
	client, err := envelope.NewLocalKMSClient()
	require.NoError(t, err)
	return envelope.NewVault(client, "arn:aws:kms:test:000:key/test")
}
