package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// redirectTransport rewrites request host/scheme to a test server.
type redirectTransport struct {
	target *url.URL
}

func (rt *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r2 := req.Clone(req.Context())
	r2.URL.Host = rt.target.Host
	r2.URL.Scheme = rt.target.Scheme
	return http.DefaultTransport.RoundTrip(r2)
}

// newTestClient creates a Client that redirects all requests to srv.
func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	u, _ := url.Parse(srv.URL)
	c := New("test-token")
	c.httpClient = &http.Client{Transport: &redirectTransport{target: u}}
	return c
}

func TestFirstLine(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"single line", "single line"},
		{"first\nsecond\nthird", "first"},
		{"  trimmed  \n second", "trimmed"},
		{"", ""},
		{"\n\nsecond", ""},
	}
	for _, tc := range cases {
		got := firstLine(tc.input)
		if got != tc.want {
			t.Errorf("firstLine(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestClient_GetCommit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		resp := map[string]any{
			"commit": map[string]any{
				"message": "feat: add feature\n\nDetailed description here.",
				"author":  map[string]any{"name": "Alice"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	commit, err := c.GetCommit(context.Background(), "owner/repo", "abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commit.Message != "feat: add feature" {
		t.Errorf("Message = %q, want %q", commit.Message, "feat: add feature")
	}
	if commit.Author != "Alice" {
		t.Errorf("Author = %q, want %q", commit.Author, "Alice")
	}
}

func TestClient_GetCommit_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.GetCommit(context.Background(), "owner/repo", "abc123")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestClient_GetBranchHead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"commit": map[string]any{"sha": "deadbeef"},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	sha, err := c.GetBranchHead(context.Background(), "owner/repo", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sha != "deadbeef" {
		t.Errorf("SHA = %q, want %q", sha, "deadbeef")
	}
}

func TestClient_GetBranchHead_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.GetBranchHead(context.Background(), "owner/repo", "main")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestClient_ListRepos(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		perms := struct {
			Admin bool `json:"admin"`
		}{Admin: true}
		repos := []Repo{
			{FullName: "owner/repo-a", DefaultBranch: "main", Private: false, Permissions: perms},
			{FullName: "owner/repo-b", DefaultBranch: "main", Private: true, Permissions: perms},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(repos)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	repos, err := c.ListRepos(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("got %d repos, want 2", len(repos))
	}
	if repos[0].FullName != "owner/repo-a" {
		t.Errorf("repos[0].FullName = %q, want %q", repos[0].FullName, "owner/repo-a")
	}
	if !repos[1].Private {
		t.Error("repos[1].Private = false, want true")
	}
}

func TestClient_CreateWebhook(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "wrong method", http.StatusMethodNotAllowed)
			return
		}
		wh := Webhook{ID: 42, Events: []string{"push"}, Active: true}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(wh)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	id, err := c.CreateWebhook(context.Background(), CreateWebhookInput{
		RepoFullName: "owner/repo",
		PayloadURL:   "https://example.com/webhook",
		Secret:       "s3cr3t",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 42 {
		t.Errorf("webhook ID = %d, want 42", id)
	}
}

func TestClient_CreateWebhook_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unprocessable", http.StatusUnprocessableEntity)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.CreateWebhook(context.Background(), CreateWebhookInput{
		RepoFullName: "owner/repo",
		PayloadURL:   "https://example.com/webhook",
		Secret:       "s3cr3t",
	})
	if err == nil {
		t.Fatal("expected error for 422 response")
	}
}

func TestClient_DeleteWebhook_NoContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "wrong method", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if err := c.DeleteWebhook(context.Background(), "owner/repo", 42); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestClient_DeleteWebhook_NotFound(t *testing.T) {
	// GitHub returns 404 when webhook already deleted — should be treated as success.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if err := c.DeleteWebhook(context.Background(), "owner/repo", 99); err != nil {
		t.Errorf("expected 404 to be ignored, got error: %v", err)
	}
}

func TestClient_DeleteWebhook_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if err := c.DeleteWebhook(context.Background(), "owner/repo", 42); err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestClient_AuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode([]Repo{})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, _ = c.ListRepos(context.Background())

	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer test-token")
	}
}
