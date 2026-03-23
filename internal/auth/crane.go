package auth

import (
	"context"

	"github.com/google/go-containerregistry/pkg/authn"
)

// craneAuthenticator implements authn.Authenticator for crane operations.
// It uses the astro server Bearer token for authentication.
type craneAuthenticator struct {
	tokenManager *TokenManager
}

// Authorization returns the authorization header for crane operations.
func (a *craneAuthenticator) Authorization() (*authn.AuthConfig, error) {
	token, err := a.tokenManager.GetValidAccessToken(context.Background())
	if err != nil {
		return nil, err
	}
	return &authn.AuthConfig{RegistryToken: token}, nil
}

// GetCraneAuth returns an authn.Authenticator for use with crane.
func GetCraneAuth(binaryName string) authn.Authenticator {
	return &craneAuthenticator{tokenManager: NewTokenManager(binaryName)}
}

// staticTokenAuthenticator implements authn.Authenticator with a fixed token.
type staticTokenAuthenticator struct {
	token string
}

func (a *staticTokenAuthenticator) Authorization() (*authn.AuthConfig, error) {
	return &authn.AuthConfig{RegistryToken: a.token}, nil
}

// GetCraneAuthWithToken returns an authn.Authenticator that uses the provided token directly.
func GetCraneAuthWithToken(token string) authn.Authenticator {
	return &staticTokenAuthenticator{token: token}
}
