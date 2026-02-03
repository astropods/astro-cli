package auth

import (
	"context"

	"github.com/google/go-containerregistry/pkg/authn"
)

// CraneAuthenticator implements authn.Authenticator for crane operations.
// It uses the astro server Bearer token for authentication.
type CraneAuthenticator struct {
	tokenManager *TokenManager
}

// NewCraneAuthenticator creates a new crane authenticator.
func NewCraneAuthenticator() *CraneAuthenticator {
	return &CraneAuthenticator{
		tokenManager: NewTokenManager(),
	}
}

// Authorization returns the authorization header for crane operations.
// For the astro server registry proxy, we use Bearer token authentication.
func (a *CraneAuthenticator) Authorization() (*authn.AuthConfig, error) {
	token, err := a.tokenManager.GetValidAccessToken(context.Background())
	if err != nil {
		return nil, err
	}

	// Return as Bearer token in the RegistryToken field
	// crane will use this as "Authorization: Bearer <token>"
	return &authn.AuthConfig{
		RegistryToken: token,
	}, nil
}

// GetCraneAuth returns an authn.Authenticator for use with crane.
func GetCraneAuth() authn.Authenticator {
	return NewCraneAuthenticator()
}

// GetCraneKeychain returns a keychain that provides astro auth for all registries.
// This is useful when the target is the astro server acting as a registry proxy.
func GetCraneKeychain() authn.Keychain {
	return &astroKeychain{}
}

// astroKeychain implements authn.Keychain for astro server authentication.
type astroKeychain struct{}

// Resolve returns the authenticator for any registry when using astro proxy.
func (k *astroKeychain) Resolve(resource authn.Resource) (authn.Authenticator, error) {
	return NewCraneAuthenticator(), nil
}
