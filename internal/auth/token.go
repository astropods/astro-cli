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

var (
	ErrSessionEnded   = errors.New("session has ended")
	ErrNoRefreshToken = errors.New("no refresh token available")

	errCredentialsNotSaved = errors.New("failed to save refreshed credentials")
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

func (m *TokenManager) GetValidAccessToken(ctx context.Context) (string, error) {
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

	if m.shouldRefresh(profile) {
		if profile.RefreshToken == "" {
			return "", ErrNoRefreshToken
		}
		return m.refreshPersonalToken(ctx, false)
	}

	return profile.AccessToken, nil
}

// shouldRefresh checks if the token should be refreshed
func (m *TokenManager) shouldRefresh(profile *Profile) bool {
	return tokenNeedsRefresh(profile.AccessToken, profile.ExpiresAt)
}

func tokenNeedsRefresh(accessToken string, expiresAt time.Time) bool {
	// Zero expiry means unknown — refresh to be safe (e.g. corrupted storage, old credentials)
	if expiresAt.IsZero() {
		return true
	}
	threshold := time.Now().Add(RefreshThreshold)
	if threshold.After(expiresAt) {
		return true
	}
	// Also honor JWT exp — stored ExpiresAt can drift from the bearer after upgrades
	// or when org-scoped refreshes rotate the refresh token without updating profile metadata.
	if accessToken != "" {
		if jwtExp, err := ParseJWTExpiry(accessToken); err == nil && threshold.After(jwtExp) {
			return true
		}
	}
	return false
}

// ForceRefreshAccessToken unconditionally exchanges the refresh token for a new access token.
func (m *TokenManager) ForceRefreshAccessToken(ctx context.Context) (string, error) {
	return m.forceRefresh(ctx)
}

func (m *TokenManager) refreshPersonalToken(ctx context.Context, force bool) (string, error) {
	token, err := m.rotate(ctx,
		func(p *Profile) (string, bool) {
			return p.AccessToken, !force && p.AccessToken != "" && !m.shouldRefresh(p)
		},
		func(refreshToken string) (*TokenResponse, error) {
			return m.client.RefreshAccessToken(ctx, refreshToken)
		},
		func(p *Profile, resp *TokenResponse) {
			p.AccessToken = resp.AccessToken
			p.ExpiresAt = tokenExpiry(resp)
		},
	)
	if err != nil {
		return "", err
	}
	return token, nil
}

// rotate holds the credentials lock because each exchange invalidates the refresh token other processes have read.
func (m *TokenManager) rotate(
	ctx context.Context,
	reuse func(*Profile) (string, bool),
	exchange func(refreshToken string) (*TokenResponse, error),
	apply func(*Profile, *TokenResponse),
) (string, error) {
	unlock := m.storage.lock(ctx)
	defer unlock()

	var rejected string
	var rejectErr error
	for {
		creds, err := m.storage.LoadCredentials()
		if err != nil {
			return "", fmt.Errorf("not authenticated: %w", err)
		}
		profile, ok := creds.Profiles[creds.CurrentProfile]
		if !ok {
			return "", errors.New("not authenticated: no current profile found")
		}
		if token, ok := reuse(profile); ok {
			return token, nil
		}
		if profile.RefreshToken == "" {
			return "", ErrNoRefreshToken
		}
		if rejectErr != nil && profile.RefreshToken == rejected {
			return "", rejectErr
		}

		resp, err := exchange(profile.RefreshToken)
		if err != nil {
			if rejectErr != nil || !isRejectedGrant(err) {
				return "", err
			}
			rejected, rejectErr = profile.RefreshToken, err
			continue
		}

		if resp.RefreshToken != "" {
			profile.RefreshToken = resp.RefreshToken
		}
		apply(profile, resp)
		if err := m.storage.SaveCredentials(creds); err != nil {
			return resp.AccessToken, fmt.Errorf("%w: %w", errCredentialsNotSaved, err)
		}
		return resp.AccessToken, nil
	}
}

func isRejectedGrant(err error) bool {
	return errors.Is(err, ErrSessionEnded) || errors.Is(err, errInvalidGrant)
}

func tokenExpiry(resp *TokenResponse) time.Time {
	if resp.ExpiresIn > 0 {
		return time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second)
	}
	if exp, err := ParseJWTExpiry(resp.AccessToken); err == nil {
		return exp
	}
	return time.Now().Add(5 * time.Minute)
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
		return fmt.Errorf("not authenticated. Run '%s login' to authenticate", m.storage.binaryName)
	}
	return nil
}

// GetOrgScopedAccessToken returns a token for the organization, cached until it is within RefreshThreshold of expiry.
func (m *TokenManager) GetOrgScopedAccessToken(ctx context.Context, organizationID string) (string, error) {
	return m.orgScopedAccessToken(ctx, organizationID, false)
}

// ForceRefreshOrgScopedAccessToken bypasses the org token cache.
func (m *TokenManager) ForceRefreshOrgScopedAccessToken(ctx context.Context, organizationID string) (string, error) {
	return m.orgScopedAccessToken(ctx, organizationID, true)
}

func (m *TokenManager) orgScopedAccessToken(ctx context.Context, organizationID string, force bool) (string, error) {
	reuse := func(p *Profile) (string, bool) {
		t := p.OrgTokens[organizationID]
		if force || t == nil || t.AccessToken == "" || tokenNeedsRefresh(t.AccessToken, t.ExpiresAt) {
			return "", false
		}
		return t.AccessToken, true
	}

	profile, err := m.storage.GetCurrentProfile()
	if err != nil {
		return "", fmt.Errorf("not authenticated: %w", err)
	}
	if token, ok := reuse(profile); ok {
		return token, nil
	}
	if profile.RefreshToken == "" {
		return "", ErrNoRefreshToken
	}

	token, err := m.rotate(ctx, reuse,
		func(refreshToken string) (*TokenResponse, error) {
			return m.client.RefreshAccessTokenForOrg(ctx, refreshToken, organizationID)
		},
		func(p *Profile, resp *TokenResponse) {
			if p.OrgTokens == nil {
				p.OrgTokens = make(map[string]*OrgToken)
			}
			p.OrgTokens[organizationID] = &OrgToken{AccessToken: resp.AccessToken, ExpiresAt: tokenExpiry(resp)}
		},
	)
	// The token is valid even when it could not be saved.
	if err != nil && !errors.Is(err, errCredentialsNotSaved) {
		return "", err
	}
	return token, nil
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

func (m *TokenManager) forceRefresh(ctx context.Context) (string, error) {
	profile, err := m.storage.GetCurrentProfile()
	if err != nil {
		return "", fmt.Errorf("not authenticated: %w", err)
	}

	if profile.RefreshToken == "" {
		return "", ErrNoRefreshToken
	}

	return m.refreshPersonalToken(ctx, true)
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
