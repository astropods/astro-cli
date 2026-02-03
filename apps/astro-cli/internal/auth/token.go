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
func NewTokenManager() *TokenManager {
	return &TokenManager{
		storage: NewStorage(),
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
			return "", fmt.Errorf("token expired and refresh failed: %w. Run 'astro login' to re-authenticate", err)
		}
		profile = newProfile
	}

	return profile.AccessToken, nil
}

// shouldRefresh checks if the token should be refreshed
func (m *TokenManager) shouldRefresh(profile *Profile) bool {
	if profile.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().Add(RefreshThreshold).After(profile.ExpiresAt)
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
		if exp, err := parseJWTExpiry(tokenResp.AccessToken); err == nil {
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
		return errors.New("not authenticated. Run 'astro login' to authenticate")
	}
	return nil
}

// AuthenticatedClient returns an HTTP client that adds auth headers
type AuthenticatedClient struct {
	tokenManager *TokenManager
	httpClient   *http.Client
}

// NewAuthenticatedClient creates a new authenticated HTTP client
func NewAuthenticatedClient() *AuthenticatedClient {
	return &AuthenticatedClient{
		tokenManager: NewTokenManager(),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Do executes an HTTP request with authentication
func (c *AuthenticatedClient) Do(req *http.Request) (*http.Response, error) {
	token, err := c.tokenManager.GetValidAccessToken(req.Context())
	if err != nil {
		return nil, fmt.Errorf("authentication required: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	return c.httpClient.Do(req)
}

// AddAuthHeader adds the authorization header to an existing request
func AddAuthHeader(ctx context.Context, req *http.Request) error {
	manager := NewTokenManager()
	token, err := manager.GetValidAccessToken(ctx)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	return nil
}

// GetAuthHeader returns the authorization header value
func GetAuthHeader(ctx context.Context) (string, error) {
	manager := NewTokenManager()
	token, err := manager.GetValidAccessToken(ctx)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Bearer %s", token), nil
}

// parseJWTExpiry extracts the expiry time from a JWT token
func parseJWTExpiry(tokenString string) (time.Time, error) {
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
