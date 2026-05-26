package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	// RefreshThreshold is the time before expiry when we should refresh the token
	RefreshThreshold = 5 * time.Minute
)

// TokenManager handles token lifecycle
type TokenManager struct {
	storage *Storage
	client  *Client
}

// NewTokenManager creates a new token manager
func NewTokenManager(binaryName string) *TokenManager {
	return &TokenManager{
		storage: NewStorage(binaryName),
		client:  NewClient(),
	}
}

// GetValidAccessToken returns a valid access token, refreshing if necessary
func (m *TokenManager) GetValidAccessToken(ctx context.Context) (string, error) {
	// Check for environment variable override
	if token := GetEnvAccessToken(); token != "" {
		return token, nil
	}

	profile, err := m.storage.GetCurrentProfile()
	if err != nil {
		return "", fmt.Errorf("not authenticated: %w", err)
	}

	if profile.AccessToken == "" {
		return "", errors.New("not authenticated: no access token found")
	}

	// Check if token needs refresh
	if m.shouldRefresh(profile) {
		if profile.RefreshToken == "" {
			return "", errors.New("token expired and no refresh token available")
		}

		newProfile, err := m.refreshToken(ctx, profile)
		if err != nil {
			// Return more specific error for refresh failures
			return "", fmt.Errorf("token expired and refresh failed: %w. Run 'ast login' to re-authenticate", err)
		}
		profile = newProfile
	}

	return profile.AccessToken, nil
}

// shouldRefresh checks if the token should be refreshed
func (m *TokenManager) shouldRefresh(profile *Profile) bool {
	// Zero expiry means unknown — refresh to be safe (e.g. corrupted storage, old credentials)
	if profile.ExpiresAt.IsZero() {
		return true
	}
	threshold := time.Now().Add(RefreshThreshold)
	if threshold.After(profile.ExpiresAt) {
		return true
	}
	// Also honor JWT exp — stored ExpiresAt can drift from the bearer after upgrades
	// or when org-scoped refreshes rotate the refresh token without updating profile metadata.
	if profile.AccessToken != "" {
		if jwtExp, err := ParseJWTExpiry(profile.AccessToken); err == nil && threshold.After(jwtExp) {
			return true
		}
	}
	return false
}

// ForceRefreshAccessToken unconditionally exchanges the refresh token for a new access token.
func (m *TokenManager) ForceRefreshAccessToken(ctx context.Context) (string, error) {
	return m.forceRefresh(ctx)
}

// refreshToken refreshes the access token using the refresh token
func (m *TokenManager) refreshToken(ctx context.Context, profile *Profile) (*Profile, error) {
	tokenResp, err := m.client.RefreshAccessToken(ctx, profile.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("refresh request failed: %w", err)
	}

	// Update profile with new tokens
	profile.AccessToken = tokenResp.AccessToken
	if tokenResp.RefreshToken != "" {
		profile.RefreshToken = tokenResp.RefreshToken
	}

	// Update expiry time
	if tokenResp.ExpiresIn > 0 {
		profile.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	} else {
		// Parse expiry from JWT if expires_in not provided
		if exp, err := ParseJWTExpiry(tokenResp.AccessToken); err == nil {
			profile.ExpiresAt = exp
		} else {
			// Fallback to 5 minutes if we can't parse
			profile.ExpiresAt = time.Now().Add(5 * time.Minute)
		}
	}

	// Save updated profile
	creds, err := m.storage.LoadCredentials()
	if err != nil {
		return nil, err
	}

	if err := m.storage.SaveProfile(creds.CurrentProfile, profile); err != nil {
		return nil, fmt.Errorf("failed to save refreshed credentials: %w", err)
	}

	return profile, nil
}

// GetCurrentUser returns the currently authenticated user
func (m *TokenManager) GetCurrentUser() (*StoredUser, error) {
	profile, err := m.storage.GetCurrentProfile()
	if err != nil {
		return nil, err
	}

	if profile.User == nil {
		return nil, errors.New("no user information available")
	}

	return profile.User, nil
}

// IsAuthenticated checks if the user is authenticated (has valid or refreshable credentials)
func (m *TokenManager) IsAuthenticated() bool {
	// Check environment variable first
	if GetEnvAccessToken() != "" {
		return true
	}

	// Check if we have valid credentials or can refresh them
	profile, err := m.storage.GetCurrentProfile()
	if err != nil {
		return false
	}

	// Have a valid (unexpired) access token
	if profile.AccessToken != "" && time.Now().Before(profile.ExpiresAt) {
		return true
	}

	// Access token expired but have a refresh token we can use
	if profile.RefreshToken != "" {
		return true
	}

	return false
}

// RequireAuth returns an error if the user is not authenticated
func (m *TokenManager) RequireAuth() error {
	if !m.IsAuthenticated() {
		return errors.New("not authenticated. Run 'ast login' to authenticate")
	}
	return nil
}

// GetOrgScopedAccessToken returns an access token scoped to the given WorkOS organization.
// The org-scoped access token is returned for immediate use but NOT saved to the profile
// (the stored profile keeps the unscoped personal token). However, if WorkOS rotates the
// refresh token during this call, we must persist the new refresh token — otherwise the
// stored one becomes stale and subsequent refreshes will fail.
func (m *TokenManager) GetOrgScopedAccessToken(ctx context.Context, organizationID string) (string, error) {
	profile, err := m.storage.GetCurrentProfile()
	if err != nil {
		return "", fmt.Errorf("not authenticated: %w", err)
	}

	if profile.RefreshToken == "" {
		return "", errors.New("no refresh token available. Run 'ast login' to re-authenticate")
	}

	tokenResp, err := m.client.RefreshAccessTokenForOrg(ctx, profile.RefreshToken, organizationID)
	if err != nil {
		return "", fmt.Errorf("failed to get org-scoped token: %w", err)
	}

	// Persist rotated refresh token so future refreshes don't use a stale token.
	if tokenResp.RefreshToken != "" {
		profile.RefreshToken = tokenResp.RefreshToken
		creds, err := m.storage.LoadCredentials()
		if err == nil {
			_ = m.storage.SaveProfile(creds.CurrentProfile, profile)
		}
	}

	return tokenResp.AccessToken, nil
}

// AddAuthHeader adds the authorization header to an existing request
func AddAuthHeader(ctx context.Context, req *http.Request, binaryName string) error {
	manager := NewTokenManager(binaryName)
	token, err := manager.GetValidAccessToken(ctx)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	return nil
}

// RefreshAndUpdateHeader forces a token refresh and updates the request's Authorization header.
// Use this after receiving a 401 to retry with a fresh token.
func RefreshAndUpdateHeader(ctx context.Context, req *http.Request, binaryName string) error {
	manager := NewTokenManager(binaryName)
	token, err := manager.forceRefresh(ctx)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	return nil
}

// forceRefresh unconditionally refreshes the access token
func (m *TokenManager) forceRefresh(ctx context.Context) (string, error) {
	profile, err := m.storage.GetCurrentProfile()
	if err != nil {
		return "", fmt.Errorf("not authenticated: %w", err)
	}

	if profile.RefreshToken == "" {
		return "", errors.New("no refresh token available. Run 'ast login' to re-authenticate")
	}

	newProfile, err := m.refreshToken(ctx, profile)
	if err != nil {
		return "", fmt.Errorf("token refresh failed: %w", err)
	}
	return newProfile.AccessToken, nil
}

// ParseJWTExpiry extracts the expiry time from a JWT token
func ParseJWTExpiry(tokenString string) (time.Time, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return time.Time{}, errors.New("invalid JWT format")
	}

	// Decode the payload (second part)
	payload := parts[1]
	// Add padding if needed
	if pad := len(payload) % 4; pad > 0 {
		payload += strings.Repeat("=", 4-pad)
	}

	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		// Try standard encoding
		decoded, err = base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return time.Time{}, fmt.Errorf("failed to decode JWT payload: %w", err)
		}
	}

	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return time.Time{}, fmt.Errorf("failed to parse JWT claims: %w", err)
	}

	if claims.Exp == 0 {
		return time.Time{}, errors.New("no exp claim in JWT")
	}

	return time.Unix(claims.Exp, 0), nil
}
