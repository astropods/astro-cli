package pipes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestClient creates a Client that points at the given test server.
func newTestClient(server *httptest.Server) *Client {
	return &Client{
		apiKey:     "test-key",
		endpoint:   server.URL,
		httpClient: server.Client(),
		sdk:        nil, // not used by DeleteConnection
	}
}

func TestDeleteConnection_Success(t *testing.T) {
	var capturedReq *http.Request

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = r
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := newTestClient(srv)
	err := client.DeleteConnection(context.Background(), DeleteConnectionInput{
		Provider: "github",
		UserID:   "user-1",
	})
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	if capturedReq.Method != http.MethodDelete {
		t.Errorf("expected method DELETE, got %s", capturedReq.Method)
	}

	wantPath := "/user_management/users/user-1/connected_accounts/github"
	if capturedReq.URL.Path != wantPath {
		t.Errorf("expected path %q, got %q", wantPath, capturedReq.URL.Path)
	}

	authHeader := capturedReq.Header.Get("Authorization")
	if authHeader != "Bearer test-key" {
		t.Errorf("expected Authorization header 'Bearer test-key', got %q", authHeader)
	}
}

func TestDeleteConnection_WithOrganizationID(t *testing.T) {
	var capturedReq *http.Request

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = r
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := newTestClient(srv)
	err := client.DeleteConnection(context.Background(), DeleteConnectionInput{
		Provider:       "github",
		UserID:         "user-1",
		OrganizationID: "org-1",
	})
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	rawURL := capturedReq.URL.String()
	if !strings.Contains(rawURL, "organization_id=org-1") {
		t.Errorf("expected query param organization_id=org-1 in URL %q", rawURL)
	}
}

func TestDeleteConnection_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"not_found"}`)) //nolint:errcheck
	}))
	defer srv.Close()

	client := newTestClient(srv)
	err := client.DeleteConnection(context.Background(), DeleteConnectionInput{
		Provider: "github",
		UserID:   "user-1",
	})
	if err == nil {
		t.Fatal("expected non-nil error for 400 response, got nil")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("expected error to contain '400', got: %v", err)
	}
}

func TestDeleteConnection_500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := newTestClient(srv)
	err := client.DeleteConnection(context.Background(), DeleteConnectionInput{
		Provider: "github",
		UserID:   "user-1",
	})
	if err == nil {
		t.Fatal("expected non-nil error for 500 response, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected error to contain '500', got: %v", err)
	}
}
