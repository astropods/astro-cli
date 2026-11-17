package hostproxy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"os"
	"path/filepath"

	"golang.org/x/crypto/x509roots/fallback/bundle"
)

// WriteCABundle writes the Mozilla roots plus extra (DER certificates) into
// dir and returns the file's path. The name is derived from the content, so
// projects with different CAs never share a file, and identical ones do.
// SSL_CERT_FILE and REQUESTS_CA_BUNDLE replace a client's trust store, so a
// bundle without public roots breaks every host the proxy doesn't serve.
func WriteCABundle(dir string, extra [][]byte) (string, error) {
	var buf bytes.Buffer
	for root := range bundle.Roots() {
		if root.Constraint != nil {
			continue
		}
		if err := pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: root.Certificate}); err != nil {
			return "", err
		}
	}
	for _, der := range extra {
		if err := pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
			return "", err
		}
	}

	sum := sha256.Sum256(buf.Bytes())
	dest := filepath.Join(dir, "ca-bundle-"+hex.EncodeToString(sum[:])[:16]+".pem")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, ".ca-bundle-*.tmp")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // gone after a successful rename
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil { //nolint:gosec // public certificates
		return "", err
	}
	if err := os.Rename(tmp.Name(), dest); err != nil {
		return "", err
	}
	return dest, nil
}
