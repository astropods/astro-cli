package deploytoken

import (
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

const issuer = "astro-server"

type Claims struct {
	AccountID string `json:"account_id"`
	jwt.RegisteredClaims
}

// Sign creates a deployment-scoped token containing the deployment ID and account ID.
// The token has no expiry — it is valid for the lifetime of the deployment.
func Sign(deploymentID, accountID, secret string) (string, error) {
	claims := Claims{
		AccountID: accountID,
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

// Verify validates the token and returns the deployment ID and account ID.
func Verify(tokenStr, secret string) (deploymentID, accountID string, err error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	}, jwt.WithIssuedAt(), jwt.WithIssuer(issuer))
	if err != nil {
		return "", "", fmt.Errorf("invalid deploy token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return "", "", errors.New("invalid deploy token claims")
	}
	return claims.Subject, claims.AccountID, nil
}
