package aigateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGenerateKey_RequiresAccountIDInUserAndTeam(t *testing.T) {
	// Load-bearing invariant per docs/plans/ai-gateway-astro-server.md §2:
	// user_id and team_id MUST be the account-id. The client asserts both
	// non-empty before serializing — guards against a refactor that
	// accidentally drops the field.
	c := NewClient("http://example", "mk")

	if _, err := c.GenerateKey(context.Background(), KeyRequest{TeamID: "acct-1"}); err == nil {
		t.Error("expected error when UserID empty")
	} else if !strings.Contains(err.Error(), "UserID") {
		t.Errorf("expected UserID error, got %v", err)
	}
	if _, err := c.GenerateKey(context.Background(), KeyRequest{UserID: "acct-1"}); err == nil {
		t.Error("expected error when TeamID empty")
	} else if !strings.Contains(err.Error(), "TeamID") {
		t.Errorf("expected TeamID error, got %v", err)
	}
}

func TestGenerateKey_SendsAccountIDAsUserAndTeam(t *testing.T) {
	// End-to-end check that what we put in KeyRequest.UserID/TeamID is what
	// hits the wire. Pinning this guards against silent attribution drift.
	var captured KeyRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/key/generate" {
			http.Error(w, "wrong path", http.StatusBadRequest)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(KeyResponse{Key: "sk-astro-xxx", KeyID: "tok-123"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "mk")
	resp, err := c.GenerateKey(context.Background(), KeyRequest{
		UserID:   "acct-42",
		TeamID:   "acct-42",
		Metadata: map[string]any{"cluster_id": "preview-a"},
	})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	if captured.UserID != "acct-42" {
		t.Errorf("user_id on wire: got %q, want %q", captured.UserID, "acct-42")
	}
	if captured.TeamID != "acct-42" {
		t.Errorf("team_id on wire: got %q, want %q", captured.TeamID, "acct-42")
	}
	if got, _ := captured.Metadata["cluster_id"].(string); got != "preview-a" {
		t.Errorf("metadata.cluster_id: got %v, want %q", captured.Metadata["cluster_id"], "preview-a")
	}
	if resp.Key != "sk-astro-xxx" || resp.KeyID != "tok-123" {
		t.Errorf("response parse: got %+v", resp)
	}
}

func TestDeleteKey_TreatsNotFoundAsSuccess(t *testing.T) {
	// Retries on transient failures shouldn't fail on the second pass when
	// the upstream already deleted the key.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "mk")
	if err := c.DeleteKey(context.Background(), "tok-gone"); err != nil {
		t.Errorf("404 should be treated as success, got %v", err)
	}
}

func TestDeleteKey_PropagatesOtherErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"server error"}`, http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "mk")
	err := c.DeleteKey(context.Background(), "tok-1")
	if err == nil {
		t.Fatal("expected error from 500")
	}
	var he *httpError
	if !errors.As(err, &he) || he.Status != http.StatusInternalServerError {
		t.Errorf("expected 500 httpError, got %v", err)
	}
}
