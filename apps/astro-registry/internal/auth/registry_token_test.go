package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	testRegistrySecret  = "test-secret-please-rotate"
	testRegistryIssuer  = "astro-registry"
	testRegistryService = "astro-registry"
)

func newTestSigner() *RegistryTokenSigner {
	return NewRegistryTokenSigner(testRegistrySecret, testRegistryIssuer, testRegistryService, time.Hour)
}

func TestRegistryTokenSigner_IssueVerifyRoundtrip(t *testing.T) {
	t.Parallel()

	s := newTestSigner()
	access := []ResourceAccess{{
		Type:      "repository",
		Name:      "saswatds/myapp",
		Actions:   []string{"pull", "push"},
		AccountID: "01kggdgfrw46qcsnxeqbr1hr1z",
	}}

	tok, expiresIn, issuedAt, err := s.Issue("user_123", access)
	if err != nil {
		t.Fatalf("Issue failed: %v", err)
	}
	if tok == "" {
		t.Fatal("expected non-empty token")
	}
	if expiresIn != 3600 {
		t.Errorf("expected expires_in=3600, got %d", expiresIn)
	}
	if time.Since(issuedAt) > 5*time.Second || time.Since(issuedAt) < -5*time.Second {
		t.Errorf("issuedAt drift too large: %v", time.Since(issuedAt))
	}

	claims, err := s.Verify(tok)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if claims.Subject != "user_123" {
		t.Errorf("expected subject user_123, got %q", claims.Subject)
	}
	if claims.Issuer != testRegistryIssuer {
		t.Errorf("expected issuer %q, got %q", testRegistryIssuer, claims.Issuer)
	}
	foundAud := false
	for _, a := range claims.Audience {
		if a == testRegistryService {
			foundAud = true
		}
	}
	if !foundAud {
		t.Errorf("expected audience to include %q, got %v", testRegistryService, claims.Audience)
	}
	if len(claims.Access) != 1 {
		t.Fatalf("expected 1 access entry, got %d", len(claims.Access))
	}
	if claims.Access[0].Name != "saswatds/myapp" {
		t.Errorf("expected access name saswatds/myapp, got %q", claims.Access[0].Name)
	}
	if claims.Access[0].AccountID != "01kggdgfrw46qcsnxeqbr1hr1z" {
		t.Errorf("expected account_id roundtrip, got %q", claims.Access[0].AccountID)
	}
}

func TestRegistryTokenSigner_RejectsWrongSecret(t *testing.T) {
	t.Parallel()

	s := newTestSigner()
	tok, _, _, err := s.Issue("user_123", nil)
	if err != nil {
		t.Fatalf("Issue failed: %v", err)
	}

	wrong := NewRegistryTokenSigner("different-secret", testRegistryIssuer, testRegistryService, time.Hour)
	if _, err := wrong.Verify(tok); err == nil {
		t.Fatal("expected verify to fail with wrong secret")
	} else if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

func TestRegistryTokenSigner_RejectsExpiredToken(t *testing.T) {
	t.Parallel()

	// Past TTL → token already expired by the time we verify.
	s := NewRegistryTokenSigner(testRegistrySecret, testRegistryIssuer, testRegistryService, -1*time.Minute)
	tok, _, _, err := s.Issue("user_123", nil)
	if err != nil {
		t.Fatalf("Issue failed: %v", err)
	}

	_, err = s.Verify(tok)
	if !errors.Is(err, ErrTokenExpired) {
		t.Errorf("expected ErrTokenExpired, got %v", err)
	}
}

func TestRegistryTokenSigner_RejectsWrongIssuer(t *testing.T) {
	t.Parallel()

	s := newTestSigner()
	tok, _, _, err := s.Issue("user_123", nil)
	if err != nil {
		t.Fatalf("Issue failed: %v", err)
	}

	other := NewRegistryTokenSigner(testRegistrySecret, "some-other-issuer", testRegistryService, time.Hour)
	if _, err := other.Verify(tok); err == nil {
		t.Fatal("expected verify to fail with wrong issuer")
	}
}

func TestRegistryTokenSigner_RejectsWrongAlg(t *testing.T) {
	t.Parallel()

	// Forge a "none"-alg token claiming to be from us. Verify must reject it.
	claims := RegistryTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    testRegistryIssuer,
			Subject:   "attacker",
			Audience:  jwt.ClaimStrings{testRegistryService},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	t2 := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	forged, err := t2.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("forge failed: %v", err)
	}

	s := newTestSigner()
	if _, err := s.Verify(forged); err == nil {
		t.Fatal("expected verify to reject none-alg token")
	} else if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

func TestRegistryTokenClaims_HasAccess(t *testing.T) {
	t.Parallel()

	claims := &RegistryTokenClaims{
		Access: []ResourceAccess{
			{Type: "repository", Name: "ns/img", Actions: []string{"pull", "push"}},
			{Type: "repository", Name: "ns/readonly", Actions: []string{"pull"}},
		},
	}

	tests := []struct {
		repo   string
		action string
		want   bool
		desc   string
	}{
		{"ns/img", "pull", true, "exact pull"},
		{"ns/img", "push", true, "exact push"},
		{"ns/readonly", "pull", true, "readonly grants pull"},
		{"ns/readonly", "push", false, "readonly does not grant push"},
		{"other/img", "pull", false, "wrong repo"},
		{"ns/img", "delete", false, "no delete grant"},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			if got := claims.HasAccess(tc.repo, tc.action); got != tc.want {
				t.Errorf("HasAccess(%q, %q) = %v, want %v", tc.repo, tc.action, got, tc.want)
			}
		})
	}

	pushOnly := &RegistryTokenClaims{
		Access: []ResourceAccess{{Type: "repository", Name: "ns/img", Actions: []string{"push"}}},
	}
	if !pushOnly.HasAccess("ns/img", "pull") {
		t.Error("push-only grant should imply pull")
	}
	if !pushOnly.HasAccess("ns/img", "push") {
		t.Error("push-only grant should grant push")
	}
}

func TestRegistryTokenClaims_AccessForReturnsEntry(t *testing.T) {
	t.Parallel()

	claims := &RegistryTokenClaims{
		Access: []ResourceAccess{{
			Type:      "repository",
			Name:      "ns/img",
			Actions:   []string{"pull", "push"},
			AccountID: "01k...",
		}},
	}
	entry := claims.AccessFor("ns/img", "push")
	if entry == nil {
		t.Fatal("expected entry, got nil")
	}
	if entry.AccountID != "01k..." {
		t.Errorf("expected account_id=01k..., got %q", entry.AccountID)
	}

	if claims.AccessFor("ns/missing", "pull") != nil {
		t.Error("expected nil for missing repo")
	}
}

func TestPeekIssuer(t *testing.T) {
	t.Parallel()

	s := newTestSigner()
	tok, _, _, err := s.Issue("user_123", nil)
	if err != nil {
		t.Fatalf("Issue failed: %v", err)
	}

	iss, err := PeekIssuer(tok)
	if err != nil {
		t.Fatalf("PeekIssuer failed: %v", err)
	}
	if iss != testRegistryIssuer {
		t.Errorf("expected issuer %q, got %q", testRegistryIssuer, iss)
	}

	if _, err := PeekIssuer("not.a.jwt"); err == nil {
		t.Fatal("expected error on garbage input")
	}
}
