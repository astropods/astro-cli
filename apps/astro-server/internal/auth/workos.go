package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/workos/workos-go/v6/pkg/usermanagement"
)

// WorkOSClient wraps the WorkOS user management SDK
type WorkOSClient struct {
	clientID    string
	redirectURI string
	frontendURL string
	client      *usermanagement.Client
}

// NewWorkOSClient creates a new WorkOS client
func NewWorkOSClient(apiKey, clientID, redirectURI, frontendURL string) *WorkOSClient {
	usermanagement.SetAPIKey(apiKey)

	return &WorkOSClient{
		clientID:    clientID,
		redirectURI: redirectURI,
		frontendURL: frontendURL,
		client:      usermanagement.NewClient(apiKey),
	}
}

type AuthorizationURLOpts struct {
	ScreenHint      usermanagement.ScreenHint
	InvitationToken string
}

// GetAuthorizationURL generates the WorkOS authorization URL for AuthKit.
func (c *WorkOSClient) GetAuthorizationURL(state string, opts AuthorizationURLOpts) (string, error) {
	wopts := usermanagement.GetAuthorizationURLOpts{
		ClientID:    c.clientID,
		RedirectURI: c.redirectURI,
		Provider:    "authkit",
		State:       state,
	}
	if opts.ScreenHint != "" {
		wopts.ScreenHint = opts.ScreenHint
	}
	authURL, err := usermanagement.GetAuthorizationURL(wopts)
	if err != nil {
		return "", fmt.Errorf("failed to generate authorization URL: %w", err)
	}
	// The v6 SDK does not expose InvitationToken on GetAuthorizationURLOpts, so
	// append it manually — WorkOS's authorization endpoint accepts it as a query param.
	if opts.InvitationToken != "" {
		q := authURL.Query()
		q.Set("invitation_token", opts.InvitationToken)
		authURL.RawQuery = q.Encode()
	}
	return authURL.String(), nil
}

// AuthenticateWithCode exchanges an authorization code for user credentials
func (c *WorkOSClient) AuthenticateWithCode(ctx context.Context, code string) (*AuthResult, error) {
	response, err := c.client.AuthenticateWithCode(ctx, usermanagement.AuthenticateWithCodeOpts{
		ClientID: c.clientID,
		Code:     code,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate with code: %w", err)
	}

	// Extract session ID from the access token JWT claims
	sessionID := extractSessionIDFromToken(response.AccessToken)

	return &AuthResult{
		User:           UserFromWorkOS(response.User),
		AccessToken:    response.AccessToken,
		RefreshToken:   response.RefreshToken,
		OrganizationID: response.OrganizationID,
		SessionID:      sessionID,
	}, nil
}

// AuthenticateWithRefreshToken refreshes an access token
func (c *WorkOSClient) AuthenticateWithRefreshToken(ctx context.Context, refreshToken string) (*RefreshResult, error) {
	return c.AuthenticateWithRefreshTokenForOrg(ctx, refreshToken, "")
}

// AuthenticateWithRefreshTokenForOrg refreshes an access token scoped to a specific organization.
// If organizationID is empty, behaves like AuthenticateWithRefreshToken.
func (c *WorkOSClient) AuthenticateWithRefreshTokenForOrg(ctx context.Context, refreshToken, organizationID string) (*RefreshResult, error) {
	opts := usermanagement.AuthenticateWithRefreshTokenOpts{
		ClientID:     c.clientID,
		RefreshToken: refreshToken,
	}
	if organizationID != "" {
		opts.OrganizationID = organizationID
	}

	response, err := c.client.AuthenticateWithRefreshToken(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}

	return &RefreshResult{
		AccessToken:  response.AccessToken,
		RefreshToken: response.RefreshToken,
	}, nil
}

// GetUser retrieves a user by ID
func (c *WorkOSClient) GetUser(ctx context.Context, userID string) (*User, error) {
	user, err := c.client.GetUser(ctx, usermanagement.GetUserOpts{
		User: userID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return UserFromWorkOS(user), nil
}

// UpdateUser updates a user's profile fields
func (c *WorkOSClient) UpdateUser(ctx context.Context, userID string, firstName, lastName string) (*User, error) {
	user, err := c.client.UpdateUser(ctx, usermanagement.UpdateUserOpts{
		User:      userID,
		FirstName: firstName,
		LastName:  lastName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	return UserFromWorkOS(user), nil
}

// GetLogoutURL returns the WorkOS logout URL with the default frontend URL
func (c *WorkOSClient) GetLogoutURL(sessionID string) (string, error) {
	return c.GetLogoutURLWithReturnTo(sessionID, c.frontendURL)
}

// GetLogoutURLWithReturnTo returns the WorkOS logout URL with a custom return URL
func (c *WorkOSClient) GetLogoutURLWithReturnTo(sessionID, returnTo string) (string, error) {
	logoutURL, err := usermanagement.GetLogoutURL(usermanagement.GetLogoutURLOpts{
		SessionID: sessionID,
		ReturnTo:  returnTo,
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate logout URL: %w", err)
	}
	return logoutURL.String(), nil
}

// RevokeSession revokes a session
func (c *WorkOSClient) RevokeSession(ctx context.Context, sessionID string) error {
	return c.client.RevokeSession(ctx, usermanagement.RevokeSessionOpts{
		SessionID: sessionID,
	})
}

// GetJWKSURL returns the JWKS URL for validating access tokens
func (c *WorkOSClient) GetJWKSURL() (string, error) {
	jwksURL, err := usermanagement.GetJWKSURL(c.clientID)
	if err != nil {
		return "", err
	}
	return jwksURL.String(), nil
}

// BuildCallbackErrorURL constructs a URL with error parameters for frontend redirect
func (c *WorkOSClient) BuildCallbackErrorURL(errorCode, errorDesc string) string {
	u, _ := url.Parse(c.frontendURL)
	q := u.Query()
	q.Set("error", errorCode)
	q.Set("error_description", errorDesc)
	u.RawQuery = q.Encode()
	return u.String()
}

// BuildCallbackSuccessURL constructs a success URL for frontend redirect
func (c *WorkOSClient) BuildCallbackSuccessURL() string {
	return c.frontendURL
}

// AuthResult represents the result of authentication
type AuthResult struct {
	User           *User
	AccessToken    string //nolint:gosec
	RefreshToken   string //nolint:gosec
	OrganizationID string
	SessionID      string
}

// RefreshResult represents the result of a token refresh
type RefreshResult struct {
	AccessToken  string //nolint:gosec
	RefreshToken string //nolint:gosec
}

// TokenClaims represents the JWT claims we extract from access tokens
type TokenClaims struct {
	SessionID                string   `json:"sid"`
	OrganizationMembershipID string   `json:"organization_membership_id,omitempty"`
	Role                     string   `json:"role"`
	Permissions              []string `json:"permissions"`
}

// ExtractTokenClaims decodes JWT payload claims without signature validation.
// This is safe because we only call it on tokens freshly received from WorkOS.
func ExtractTokenClaims(token string) TokenClaims {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return TokenClaims{}
	}

	// Decode the payload (second part)
	payload := parts[1]
	// Add padding if necessary
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}

	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		return TokenClaims{}
	}

	var claims TokenClaims
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return TokenClaims{}
	}

	return claims
}

// extractSessionIDFromToken extracts the session ID from a JWT access token
// The session ID is stored in the "sid" claim
func extractSessionIDFromToken(token string) string {
	return ExtractTokenClaims(token).SessionID
}
