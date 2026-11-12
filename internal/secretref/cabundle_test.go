package secretref

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteCABundle_HoldsPublicRootsAndTheExtraCA(t *testing.T) {
	extra := writeTestCA(t, t.TempDir())
	dest := filepath.Join(t.TempDir(), "nested", "ca-bundle.pem")

	got, err := WriteCABundle(dest, extra)

	require.NoError(t, err)
	assert.Equal(t, dest, got)
	data, err := os.ReadFile(dest)
	require.NoError(t, err)

	var subjects []string
	for block, rest := pem.Decode(data); block != nil; block, rest = pem.Decode(rest) {
		cert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err, "every block in the bundle is a parseable certificate")
		subjects = append(subjects, cert.Subject.CommonName)
	}
	assert.Greater(t, len(subjects), 100, "the bundle carries the Mozilla roots, not just the extra CA")
	assert.Contains(t, subjects, "passport-proxy-test")
	assert.Contains(t, subjects, "ISRG Root X1", "a widely used public root is present")
}

func TestWriteCABundle_RejectsNonCertificateInput(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(bad, []byte("not a cert"), 0o600))

	_, err := WriteCABundle(filepath.Join(t.TempDir(), "out.pem"), bad)

	assert.Error(t, err)
}

func TestWriteCABundle_MissingInput(t *testing.T) {
	_, err := WriteCABundle(filepath.Join(t.TempDir(), "out.pem"), filepath.Join(t.TempDir(), "missing.pem"))

	assert.Error(t, err)
}
