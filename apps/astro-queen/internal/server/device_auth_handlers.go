package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
)

type deviceAuthResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete,omitempty"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`  //nolint:gosec
	RefreshToken string `json:"refresh_token"` //nolint:gosec
	ExpiresIn    int    `json:"expires_in"`
}

type tokenError struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

func (s *Server) getAuthConfig(r *http.Request) (clientID, baseURL string, err error) {
	resp, err := s.admin.GetAuthConfig(r.Context(), &adminv1.GetAuthConfigRequest{})
	if err != nil {
		return "", "", fmt.Errorf("get auth config: %w", err)
	}
	return resp.WorkOSClientID, resp.WorkOSBaseURL, nil
}

// handleDeviceAuthStart initiates the WorkOS device authorization flow.
// POST /api/auth/device → returns {device_code, user_code, verification_uri_complete, ...}
func (s *Server) handleDeviceAuthStart(w http.ResponseWriter, r *http.Request) {
	clientID, baseURL, err := s.getAuthConfig(r)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}

	data := url.Values{"client_id": {clientID}}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
		baseURL+"/user_management/authorize/device",
		strings.NewReader(data.Encode()))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req) //nolint:gosec
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		var te tokenError
		if err := json.NewDecoder(resp.Body).Decode(&te); err == nil {
			writeErr(w, resp.StatusCode, fmt.Sprintf("%s: %s", te.Error, te.ErrorDescription))
			return
		}
		writeErr(w, resp.StatusCode, "device authorization failed")
		return
	}

	var authResp deviceAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if authResp.ExpiresIn == 0 {
		authResp.ExpiresIn = 300
	}
	if authResp.Interval == 0 {
		authResp.Interval = 5
	}
	writeJSON(w, http.StatusOK, authResp)
}

// handleDeviceAuthPoll polls WorkOS for token completion.
// POST /api/auth/device/poll {device_code} → returns tokens or {status: "authorization_pending"}
func (s *Server) handleDeviceAuthPoll(w http.ResponseWriter, r *http.Request) {
	clientID, baseURL, err := s.getAuthConfig(r)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}

	var body struct {
		DeviceCode string `json:"device_code"`
	}
	if err := readJSON(r, &body); err != nil || body.DeviceCode == "" {
		writeErr(w, http.StatusBadRequest, "device_code is required")
		return
	}

	data := url.Values{
		"client_id":   {clientID},
		"device_code": {body.DeviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
		baseURL+"/user_management/authenticate",
		strings.NewReader(data.Encode()))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req) //nolint:gosec
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close() //nolint:errcheck

	// 400 = pending/slow_down/denied/expired — forward the status
	if resp.StatusCode == http.StatusBadRequest {
		var te tokenError
		if err := json.NewDecoder(resp.Body).Decode(&te); err == nil {
			writeJSON(w, http.StatusOK, map[string]string{
				"status": te.Error,
				"error":  te.ErrorDescription,
			})
			return
		}
		writeErr(w, http.StatusBadGateway, "unexpected error from WorkOS")
		return
	}

	if resp.StatusCode != http.StatusOK {
		writeErr(w, resp.StatusCode, "token request failed")
		return
	}

	var tokenResp tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":        "complete",
		"access_token":  tokenResp.AccessToken,
		"refresh_token": tokenResp.RefreshToken,
	})
}
