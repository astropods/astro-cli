package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken    = errors.New("invalid token")
	ErrTokenExpired    = errors.New("token expired")
	ErrJWKSFetchFailed = errors.New("failed to fetch JWKS")
	ErrKeyNotFound     = errors.New("signing key not found")
)

// JWTClaims represents the claims in a WorkOS access token
type JWTClaims struct {
	jwt.RegisteredClaims
	OrganizationID string   `json:"org_id,omitempty"`
	Role           string   `json:"role,omitempty"`
	Roles          []string `json:"roles,omitempty"`
	Permissions    []string `json:"permissions,omitempty"`
	Entitlements   []string `json:"entitlements,omitempty"`
	SessionID      string   `json:"sid,omitempty"`
	Actor          *Actor   `json:"act,omitempty"`
}

// Actor represents an impersonator in the access token
type Actor struct {
	Subject string `json:"sub,omitempty"`
}

// JWKSKey represents a single key in a JWKS response
type JWKSKey struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
	Alg string `json:"alg,omitempty"`
}

// JWKS represents a JSON Web Key Set
type JWKS struct {
	Keys []JWKSKey `json:"keys"`
}

// JWTValidator validates WorkOS access tokens
type JWTValidator struct {
	jwksURL     string
	issuer      string
	audience    string
	jwks        *JWKS
	jwksMu      sync.RWMutex
	lastFetched time.Time
	cacheTTL    time.Duration
	httpClient  *http.Client
}

// NewJWTValidator creates a new JWT validator
func NewJWTValidator(jwksURL, issuer, audience string) *JWTValidator {
	return &JWTValidator{
		jwksURL:  jwksURL,
		issuer:   issuer,
		audience: audience,
		cacheTTL: 1 * time.Hour,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// ValidateToken validates an access token and returns the claims
func (v *JWTValidator) ValidateToken(ctx context.Context, tokenString string) (*JWTClaims, error) {
	// Parse the token without validation first to get the key ID
	token, _, err := jwt.NewParser().ParseUnverified(tokenString, &JWTClaims{})
	if err != nil {
		return nil, fmt.Errorf("%w: failed to parse token: %v", ErrInvalidToken, err)
	}

	// Get the key ID from the token header
	kid, ok := token.Header["kid"].(string)
	if !ok {
		return nil, fmt.Errorf("%w: missing key ID in token header", ErrInvalidToken)
	}

	// Fetch the signing key
	key, err := v.getSigningKey(ctx, kid)
	if err != nil {
		return nil, err
	}

	// Parse and validate the token with the signing key
	claims := &JWTClaims{}
	validatedToken, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		// Validate the signing method
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return key, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	if !validatedToken.Valid {
		return nil, ErrInvalidToken
	}

	// Manually validate issuer (WorkOS uses format: https://api.workos.com/user_management/client_ID)
	if claims.Issuer != "" {
		// Accept either exact match or WorkOS user management issuer format
		validIssuer := claims.Issuer == v.issuer ||
			(v.issuer == "https://api.workos.com" &&
			 strings.HasPrefix(claims.Issuer, "https://api.workos.com/user_management/"))

		if !validIssuer {
			return nil, fmt.Errorf("%w: invalid issuer %q (expected %q)", ErrInvalidToken, claims.Issuer, v.issuer)
		}
	}

	// Validate audience - token must have audience if validator expects one
	if v.audience != "" {
		if len(claims.Audience) == 0 {
			return nil, fmt.Errorf("%w: missing audience claim", ErrInvalidToken)
		}
		validAudience := false
		for _, aud := range claims.Audience {
			if aud == v.audience {
				validAudience = true
				break
			}
		}
		if !validAudience {
			return nil, fmt.Errorf("%w: invalid audience", ErrInvalidToken)
		}
	}

	return claims, nil
}

// getSigningKey retrieves the signing key for a given key ID
func (v *JWTValidator) getSigningKey(ctx context.Context, kid string) (interface{}, error) {
	// Check if we need to refresh the JWKS
	v.jwksMu.RLock()
	needsRefresh := v.jwks == nil || time.Since(v.lastFetched) > v.cacheTTL
	v.jwksMu.RUnlock()

	if needsRefresh {
		if err := v.refreshJWKS(ctx); err != nil {
			// If we have cached keys, try to use them anyway
			v.jwksMu.RLock()
			if v.jwks == nil {
				v.jwksMu.RUnlock()
				return nil, err
			}
			v.jwksMu.RUnlock()
		}
	}

	// Find the key with the matching key ID
	v.jwksMu.RLock()
	defer v.jwksMu.RUnlock()

	for _, key := range v.jwks.Keys {
		if key.Kid == kid {
			return parseRSAPublicKey(key)
		}
	}

	return nil, ErrKeyNotFound
}

// refreshJWKS fetches the JWKS from the WorkOS endpoint
func (v *JWTValidator) refreshJWKS(ctx context.Context) error {
	v.jwksMu.Lock()
	defer v.jwksMu.Unlock()

	// Double-check after acquiring write lock
	if v.jwks != nil && time.Since(v.lastFetched) <= v.cacheTTL {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return fmt.Errorf("%w: failed to create request: %v", ErrJWKSFetchFailed, err)
	}

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrJWKSFetchFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: unexpected status code: %d", ErrJWKSFetchFailed, resp.StatusCode)
	}

	var jwks JWKS
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return fmt.Errorf("%w: failed to decode response: %v", ErrJWKSFetchFailed, err)
	}

	v.jwks = &jwks
	v.lastFetched = time.Now()
	return nil
}

// parseRSAPublicKey parses a JWK into an RSA public key
func parseRSAPublicKey(key JWKSKey) (interface{}, error) {
	if key.Kty != "RSA" {
		return nil, fmt.Errorf("unsupported key type: %s", key.Kty)
	}

	// Decode the modulus (n) and exponent (e) from base64url
	nBytes, err := base64URLDecode(key.N)
	if err != nil {
		return nil, fmt.Errorf("failed to decode modulus: %v", err)
	}

	eBytes, err := base64URLDecode(key.E)
	if err != nil {
		return nil, fmt.Errorf("failed to decode exponent: %v", err)
	}

	// Convert exponent bytes to int
	var e int
	for _, b := range eBytes {
		e = e<<8 + int(b)
	}

	// Convert modulus bytes to big.Int
	n := new(big.Int).SetBytes(nBytes)

	// Create the RSA public key
	return &rsa.PublicKey{
		N: n,
		E: e,
	}, nil
}

// base64URLDecode decodes a base64url encoded string
func base64URLDecode(s string) ([]byte, error) {
	// Add padding if necessary
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}
