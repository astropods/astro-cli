// Package sandbox runs agent sandboxes locally for `ast dev`.
package sandbox

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// The deployed control plane issues each deployment an HS256 token, and the
// agent SDK reads the control plane's base URL from that token's `iss` claim.
// An agent therefore needs no second environment variable to find the control
// plane. `ast dev` mints the same shape, with `iss` addressing the local
// broker, so the SDK works unchanged.
//
// The secret is generated per run and stays in the process, so a token from one
// `ast dev` cannot be replayed against the next.

// TokenEnvVar is the variable the agent SDK reads.
// #nosec G101 -- the name of an environment variable, not a credential.
const TokenEnvVar = "ASTRO_AUTHZ_TOKEN"

var (
	ErrMissingToken = errors.New("missing deploy token")
	ErrInvalidToken = errors.New("invalid deploy token")
)

// NewSecret returns a signing secret for one `ast dev` run.
func NewSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate a signing secret: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// SignToken mints the token the agent receives. brokerURL is the address the
// agent reaches the broker on, and becomes the `iss` claim.
func SignToken(deploymentID, brokerURL, secret string) (string, error) {
	claims := jwt.RegisteredClaims{
		Issuer:  brokerURL,
		Subject: deploymentID,
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("sign the deploy token: %w", err)
	}
	return signed, nil
}

// VerifyToken checks the signature and returns the deployment ID. The
// signature is the whole proof: only the holder of the secret can mint a token.
func VerifyToken(raw, secret string) (string, error) {
	token, err := jwt.ParseWithClaims(raw, &jwt.RegisteredClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}

	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok || !token.Valid {
		return "", ErrInvalidToken
	}
	if claims.Subject == "" {
		return "", fmt.Errorf("%w: no sub claim", ErrInvalidToken)
	}
	return claims.Subject, nil
}

// DeploymentFromRequest reads and verifies the bearer token on a request. The
// extraction and both refusals match the deployed control plane, so an author
// sees the same failure locally.
func DeploymentFromRequest(r *http.Request, secret string) (string, error) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || token == "" {
		return "", ErrMissingToken
	}
	return VerifyToken(token, secret)
}

// WriteAuthError writes the body the deployed control plane writes, so a client
// reading the error string sees one value in both places.
func WriteAuthError(w http.ResponseWriter, err error) {
	message := ErrInvalidToken.Error()
	if errors.Is(err, ErrMissingToken) {
		message = ErrMissingToken.Error()
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = fmt.Fprintf(w, `{"error":%q}`, message)
}
