// Package specsign signs and verifies deployment specs using HMAC-SHA256.
//
// The template endpoint signs the full spec it returns. The deploy endpoint
// verifies the signature — if valid, the spec is trusted as server-generated
// and no re-generation or field-level enforcement is needed.
//
// The target fields (account, display_name, deployment_id) are zeroed before
// signing/verifying because the client sets them after receiving the template.
package specsign

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	spec "github.com/astropods/astro/packages/astro-spec"
)

// NewKey generates a random 32-byte HMAC key.
func NewKey() []byte {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic("specsign: failed to generate random key: " + err.Error())
	}
	return key
}

// Sign computes an HMAC-SHA256 signature over the deployment spec.
// Target fields (account, display_name, deployment_id) are excluded
// because the client may set them after receiving the template.
func Sign(key []byte, ds *spec.AstroDeploymentSpec) string {
	payload := canonicalize(ds)
	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify checks the HMAC-SHA256 signature. Returns true if the spec is
// unmodified from what the template endpoint produced (ignoring target fields).
func Verify(key []byte, ds *spec.AstroDeploymentSpec, signature string) bool {
	expected, err := hex.DecodeString(signature)
	if err != nil || len(expected) == 0 {
		return false
	}
	payload := canonicalize(ds)
	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	return hmac.Equal(mac.Sum(nil), expected)
}

// canonicalize produces a deterministic JSON representation of the spec
// with target fields zeroed so they don't affect the signature.
func canonicalize(ds *spec.AstroDeploymentSpec) []byte {
	// Copy the spec and zero client-owned target fields.
	cp := *ds
	cp.Target.Account = ""
	cp.Target.DisplayName = ""
	cp.Target.DeploymentID = ""
	b, err := json.Marshal(cp)
	if err != nil {
		// AstroDeploymentSpec is always marshalable; this is defensive.
		panic("specsign: failed to marshal spec: " + err.Error())
	}
	return b
}
