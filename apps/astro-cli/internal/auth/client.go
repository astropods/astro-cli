package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DeviceAuthorizationResponse is returned by the device authorization endpoint
type DeviceAuthorizationResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete,omitempty"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// TokenResponse is returned by the token endpoint on success
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	User         *User  `json:"user,omitempty"`
}

// User represents the authenticated user
type User struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
}

// TokenError represents an error from the token endpoint
type TokenError struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

// Error codes from OAuth device flow
const (
	ErrorAuthorizationPending = "authorization_pending"
	ErrorSlowDown             = "slow_down"
	ErrorAccessDenied         = "access_denied"
	ErrorExpiredToken         = "expired_token"
)

// Client handles WorkOS device authorization flow
type Client struct {
	clientID   string
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new WorkOS auth client
func NewClient() *Client {
	return &Client{
		clientID: WorkOSClientID,
		baseURL:  WorkOSBaseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// RequestDeviceAuthorization initiates the device authorization flow
func (c *Client) RequestDeviceAuthorization(ctx context.Context) (*DeviceAuthorizationResponse, error) {
	endpoint := fmt.Sprintf("%s/user_management/authorize/device", c.baseURL)

	data := url.Values{}
	data.Set("client_id", c.clientID)

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var tokenErr TokenError
		if err := json.NewDecoder(resp.Body).Decode(&tokenErr); err == nil {
			return nil, fmt.Errorf("authorization request failed: %s - %s", tokenErr.Error, tokenErr.ErrorDescription)
		}
		return nil, fmt.Errorf("authorization request failed with status %d", resp.StatusCode)
	}

	var authResp DeviceAuthorizationResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Set defaults if not provided
	if authResp.ExpiresIn == 0 {
		authResp.ExpiresIn = 300 // 5 minutes default
	}
	if authResp.Interval == 0 {
		authResp.Interval = 5 // 5 seconds default
	}

	return &authResp, nil
}

// PollForTokens polls the token endpoint until authentication completes or times out
func (c *Client) PollForTokens(ctx context.Context, deviceCode string, interval, expiresIn int) (*TokenResponse, error) {
	endpoint := fmt.Sprintf("%s/user_management/authenticate", c.baseURL)

	pollInterval := time.Duration(interval) * time.Second
	timeout := time.Duration(expiresIn) * time.Second
	deadline := time.Now().Add(timeout)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if time.Now().After(deadline) {
			return nil, errors.New("device authorization expired")
		}

		tokenResp, err := c.pollOnce(ctx, endpoint, deviceCode)
		if err != nil {
			var pollErr *PollError
			if errors.As(err, &pollErr) {
				switch pollErr.Code {
				case ErrorAuthorizationPending:
					// Continue polling
					time.Sleep(pollInterval)
					continue
				case ErrorSlowDown:
					// Increase interval by 5 seconds
					pollInterval += 5 * time.Second
					time.Sleep(pollInterval)
					continue
				case ErrorAccessDenied:
					return nil, errors.New("authorization was denied by the user")
				case ErrorExpiredToken:
					return nil, errors.New("device code has expired")
				}
			}
			return nil, err
		}

		return tokenResp, nil
	}
}

// PollError represents a recoverable polling error
type PollError struct {
	Code        string
	Description string
}

func (e *PollError) Error() string {
	if e.Description != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Description)
	}
	return e.Code
}

func (c *Client) pollOnce(ctx context.Context, endpoint, deviceCode string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("client_id", c.clientID)
	data.Set("device_code", deviceCode)
	data.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Handle error responses (400 with specific error codes)
	if resp.StatusCode == http.StatusBadRequest {
		var tokenErr TokenError
		if err := json.NewDecoder(resp.Body).Decode(&tokenErr); err != nil {
			return nil, fmt.Errorf("failed to decode error response: %w", err)
		}
		return nil, &PollError{
			Code:        tokenErr.Error,
			Description: tokenErr.ErrorDescription,
		}
	}

	if resp.StatusCode != http.StatusOK {
		var tokenErr TokenError
		if err := json.NewDecoder(resp.Body).Decode(&tokenErr); err == nil {
			return nil, fmt.Errorf("token request failed: %s - %s", tokenErr.Error, tokenErr.ErrorDescription)
		}
		return nil, fmt.Errorf("token request failed with status %d", resp.StatusCode)
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	return &tokenResp, nil
}

// RefreshAccessToken exchanges a refresh token for a new access token
func (c *Client) RefreshAccessToken(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	endpoint := fmt.Sprintf("%s/user_management/authenticate", c.baseURL)

	data := url.Values{}
	data.Set("client_id", c.clientID)
	data.Set("refresh_token", refreshToken)
	data.Set("grant_type", "refresh_token")

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		var tokenErr TokenError
		if err := json.Unmarshal(body, &tokenErr); err == nil {
			return nil, fmt.Errorf("token refresh failed: %s - %s", tokenErr.Error, tokenErr.ErrorDescription)
		}
		return nil, fmt.Errorf("token refresh failed with status %d", resp.StatusCode)
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	return &tokenResp, nil
}
