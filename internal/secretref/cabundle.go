package secretref

import (
	"bytes"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/x509roots/fallback/bundle"
)

// WriteCABundle writes the Mozilla roots plus the PEM certs in extraPEMPath to
// dest, and returns dest. SSL_CERT_FILE and REQUESTS_CA_BUNDLE replace a
// client's trust store, so a bundle without public roots breaks every host the
// proxy doesn't serve.
func WriteCABundle(dest, extraPEMPath string) (string, error) {
	extra, err := os.ReadFile(extraPEMPath) //nolint:gosec // caller-provided CA path
	if err != nil {
		return "", fmt.Errorf("read CA %s: %w", extraPEMPath, err)
	}
	if block, _ := pem.Decode(extra); block == nil || block.Type != "CERTIFICATE" {
		return "", fmt.Errorf("CA %s is not a PEM certificate", extraPEMPath)
	}

	var buf bytes.Buffer
	for root := range bundle.Roots() {
		if root.Constraint != nil {
			continue
		}
		if err := pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: root.Certificate}); err != nil {
			return "", err
		}
	}
	buf.Write(bytes.TrimSpace(extra))
	buf.WriteByte('\n')

	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return "", err
	}
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil { //nolint:gosec // public certificates
		return "", err
	}
	if err := os.Rename(tmp, dest); err != nil {
		return "", err
	}
	return dest, nil
}
