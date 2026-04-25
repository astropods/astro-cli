// Package deploytoken signs and verifies the JWT that astro-server issues to
// each deployment's messaging container.
//
// The token answers two questions for the messaging container:
//  1. "Which deployment am I?" via the `sub` claim — used as the credential
//     when calling the server-side authorize endpoint.
//  2. "Which adapters are publicly open?" via `anyone_adapters` — lets the
//     container short-circuit public traffic without an authorize round-trip.
//
// All other state needed at request time (owning account, account/user grants)
// is looked up server-side by deployment_id, so the token never carries
// mutable references that could go stale on owner transfer or grant edits.
package deploytoken

import (
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

const issuer = "astro-server"

// Claims is the payload of a deploy token.
type Claims struct {
	// AnyoneAdapters lists the adapters that have an `anyone` grant at issuance.
	// The messaging container uses this to short-circuit public traffic.
	AnyoneAdapters []string `json:"anyone_adapters,omitempty"`
	jwt.RegisteredClaims
}

// Sign issues a deploy token for the given deployment with the supplied list of
// publicly-open adapters. anyoneAdapters may be nil/empty.
func Sign(deploymentID string, anyoneAdapters []string, secret string) (string, error) {
	claims := Claims{
		AnyoneAdapters: anyoneAdapters,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:  issuer,
			Subject: deploymentID,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("sign deploy token: %w", err)
	}
	return signed, nil
}

// Verify validates the token and returns the deployment ID and the
// anyone_adapters claim. Returns an error on any signature, issuer, or
// structural problem.
func Verify(tokenStr, secret string) (deploymentID string, anyoneAdapters []string, err error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	}, jwt.WithIssuedAt(), jwt.WithIssuer(issuer))
	if err != nil {
		return "", nil, fmt.Errorf("invalid deploy token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return "", nil, errors.New("invalid deploy token claims")
	}
	if claims.Subject == "" {
		return "", nil, errors.New("deploy token missing sub claim")
	}
	return claims.Subject, claims.AnyoneAdapters, nil
}
