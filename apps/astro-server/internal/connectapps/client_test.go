package connectapps

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	workos "github.com/workos/workos-go/v10"
)

func TestListPermissionsHidesSystemPermissions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/authorization/permissions" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[
			{"object":"permission","id":"p1","slug":"audiences:manage","name":"Manage audiences","description":"Add and remove members","system":false},
			{"object":"permission","id":"p2","slug":"widgets:read","name":"WorkOS internal","system":true},
			{"object":"permission","id":"p3","slug":"members:read","name":"Read members","system":false}
		],"list_metadata":{}}`))
	}))
	defer server.Close()

	client := New("sk_test", workos.WithBaseURL(server.URL))
	permissions, err := client.ListPermissions(context.Background())
	if err != nil {
		t.Fatalf("list permissions: %v", err)
	}

	if len(permissions) != 2 {
		t.Fatalf("got %d permissions, want the two non-system ones: %+v", len(permissions), permissions)
	}
	for _, p := range permissions {
		if p.Slug == "widgets:read" {
			t.Fatal("a system permission must not be offered as a scope")
		}
	}
	if permissions[0].Description != "Add and remove members" {
		t.Fatalf("description lost: %+v", permissions[0])
	}
}

func TestNewWithoutAPIKeyIsNil(t *testing.T) {
	if New("") != nil {
		t.Fatal("no API key must yield no client, so handlers report apps unavailable")
	}
}
