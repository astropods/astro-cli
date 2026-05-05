package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Docker Registry v2 token-auth flow. See:
//   https://distribution.github.io/distribution/spec/auth/token/
//   https://distribution.github.io/distribution/spec/auth/jwt/
//   docs/03-architecture/registry-token-auth.md

// ResourceAccess is one entry in the JWT `access` claim per the Distribution spec.
// Type is "repository", Name is "<account>/<image>", Actions is a subset of {"pull","push","delete"}.
//
// AccountID is an Astro extension carrying the resolved account UUID for the
// namespace in Name. It lets the registry proxy do the ECR path rewrite
// without a per-request DB lookup. Other Distribution clients ignore it.
type ResourceAccess struct {
	Type      string   `json:"type"`
	Name      string   `json:"name"`
	Actions   []string `json:"actions"`
	AccountID string   `json:"account_id,omitempty"`
}

// RegistryTokenClaims is the claim set on a registry-signed scope token.
type RegistryTokenClaims struct {
	jwt.RegisteredClaims
	Access []ResourceAccess `json:"access"`
}

// RegistryTokenSigner mints and verifies registry-scope tokens.
// HS256 with a single shared secret — same binary signs and verifies.
type RegistryTokenSigner struct {
	secret  []byte
	issuer  string
	service string
	ttl     time.Duration
}

// NewRegistryTokenSigner constructs a signer. service is the value advertised
// in WWW-Authenticate and asserted as the `aud` claim.
func NewRegistryTokenSigner(secret, issuer, service string, ttl time.Duration) *RegistryTokenSigner {
	return &RegistryTokenSigner{
		secret:  []byte(secret),
		issuer:  issuer,
		service: service,
		ttl:     ttl,
	}
}

// Issue mints a registry-scope token for the given subject and access set.
// access may be empty — clients still receive a token, but `/v2/*` requests
// will be rejected on scope mismatch (per spec, the server returns the
// intersection of requested and permitted scopes, never an error).
func (s *RegistryTokenSigner) Issue(subject string, access []ResourceAccess) (token string, expiresIn int, issuedAt time.Time, err error) {
	if len(s.secret) == 0 {
		return "", 0, time.Time{}, errors.New("registry token secret not configured")
	}

	jti, err := randomHex(16)
	if err != nil {
		return "", 0, time.Time{}, fmt.Errorf("failed to generate jti: %w", err)
	}

	now := time.Now()
	exp := now.Add(s.ttl)

	claims := RegistryTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   subject,
			Audience:  jwt.ClaimStrings{s.service},
			ExpiresAt: jwt.NewNumericDate(exp),
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        jti,
		},
		Access: access,
	}

	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := t.SignedString(s.secret)
	if err != nil {
		return "", 0, time.Time{}, fmt.Errorf("failed to sign registry token: %w", err)
	}

	return signed, int(s.ttl.Seconds()), now, nil
}

// Verify parses and validates a registry-scope token.
func (s *RegistryTokenSigner) Verify(tokenString string) (*RegistryTokenClaims, error) {
	if len(s.secret) == 0 {
		return nil, errors.New("registry token secret not configured")
	}

	claims := &RegistryTokenClaims{}
	parsed, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	},
		jwt.WithIssuer(s.issuer),
		jwt.WithAudience(s.service),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	if !parsed.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// AccessFor returns the access entry granting the given action on the named
// repository, or nil if none. "push" implies "pull" per Distribution spec.
func (c *RegistryTokenClaims) AccessFor(repository, action string) *ResourceAccess {
	for i, a := range c.Access {
		if a.Type != "repository" || a.Name != repository {
			continue
		}
		for _, granted := range a.Actions {
			if granted == action || (action == "pull" && granted == "push") {
				return &c.Access[i]
			}
		}
	}
	return nil
}

// HasAccess is a convenience wrapper over AccessFor.
func (c *RegistryTokenClaims) HasAccess(repository, action string) bool {
	return c.AccessFor(repository, action) != nil
}

// PeekIssuer returns the iss claim of a token without verifying its signature.
// Used to route tokens to the correct verifier (WorkOS vs registry).
func PeekIssuer(tokenString string) (string, error) {
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	claims := jwt.RegisteredClaims{}
	if _, _, err := parser.ParseUnverified(tokenString, &claims); err != nil {
		return "", err
	}
	return claims.Issuer, nil
}

func randomHex(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
