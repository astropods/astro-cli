package deploytoken

import (
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

// G1 - sign/verify round-trips claims; iss carries the server URL.
func TestSignVerifyRoundtrip(t *testing.T) {
	tok, err := Sign("dep-1", "https://astropods.com", []string{"web"}, "secret")
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	dep, anyAdapters, err := Verify(tok, "secret")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if dep != "dep-1" {
		t.Errorf("deployment_id: got %q, want dep-1", dep)
	}
	if len(anyAdapters) != 1 || anyAdapters[0] != "web" {
		t.Errorf("anyone_adapters: got %v, want [web]", anyAdapters)
	}
	// Confirm iss made it into the token by parsing the claims directly.
	parsed, _, _ := jwt.NewParser().ParseUnverified(tok, &Claims{})
	gotIss := parsed.Claims.(*Claims).Issuer
	if gotIss != "https://astropods.com" {
		t.Errorf("iss: got %q, want https://astropods.com", gotIss)
	}
}

// G7 - empty anyone_adapters is preserved as nil/empty.
func TestSignVerify_EmptyAnyoneAdapters(t *testing.T) {
	tok, err := Sign("dep-1", "https://astropods.com", nil, "secret")
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	_, anyAdapters, err := Verify(tok, "secret")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if len(anyAdapters) != 0 {
		t.Errorf("expected empty anyone_adapters, got %v", anyAdapters)
	}
}

// G2 - wrong secret rejects.
func TestVerify_WrongSecret(t *testing.T) {
	tok, _ := Sign("dep-1", "https://x", nil, "secret-A")
	if _, _, err := Verify(tok, "secret-B"); err == nil {
		t.Fatal("expected error for wrong secret")
	}
}

// G3 - any iss value passes verification now (signature alone is the
// trust anchor). The messaging side reads iss for URL discovery, not
// authenticity.
func TestVerify_AnyIssuerAccepted(t *testing.T) {
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:  "https://anyserver.example.com",
			Subject: "dep-1",
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("secret"))
	if err != nil {
		t.Fatalf("manual sign: %v", err)
	}
	if _, _, err := Verify(tok, "secret"); err != nil {
		t.Fatalf("expected verify success regardless of iss value, got %v", err)
	}
}

// G4 - missing sub claim rejects.
func TestVerify_MissingSub(t *testing.T) {
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{Issuer: "https://astropods.com"},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("secret"))
	if err != nil {
		t.Fatalf("manual sign: %v", err)
	}
	if _, _, err := Verify(tok, "secret"); err == nil {
		t.Fatal("expected error for missing sub")
	}
}

// G5 - tokens with extra (unknown) claims are still accepted; we don't
// scrub fields we don't recognize. This guards against a regression where
// a legacy account_id claim from older deployments would be rejected.
func TestVerify_ExtraClaimsIgnored(t *testing.T) {
	type legacy struct {
		AccountID string `json:"account_id"`
		Claims
	}
	c := legacy{
		AccountID: "legacy-account",
		Claims: Claims{
			RegisteredClaims: jwt.RegisteredClaims{Issuer: "https://astropods.com", Subject: "dep-1"},
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString([]byte("secret"))
	if err != nil {
		t.Fatalf("manual sign: %v", err)
	}
	dep, _, err := Verify(tok, "secret")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if dep != "dep-1" {
		t.Errorf("expected dep-1, got %q", dep)
	}
}

// G8 - tampering with the payload after signing invalidates the token.
func TestVerify_TamperedClaims(t *testing.T) {
	tok, _ := Sign("dep-1", "https://astropods.com", nil, "secret")
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 JWT parts, got %d", len(parts))
	}
	// Replace the payload segment with a known-different one.
	tampered := parts[0] + ".eyJzdWIiOiJkZXAtMiJ9." + parts[2]
	if _, _, err := Verify(tampered, "secret"); err == nil {
		t.Fatal("expected error for tampered token")
	}
}

// G2/G3 - garbage input rejects.
func TestVerify_Garbage(t *testing.T) {
	if _, _, err := Verify("not.a.jwt", "secret"); err == nil {
		t.Fatal("expected error for garbage")
	}
	if _, _, err := Verify("", "secret"); err == nil {
		t.Fatal("expected error for empty token")
	}
}
