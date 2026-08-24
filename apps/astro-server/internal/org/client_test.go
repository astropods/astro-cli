package org

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/workos/workos-go/v6/pkg/usermanagement"
	"github.com/workos/workos-go/v6/pkg/workos_errors"
)

func TestListAllMembershipsFollowsPagination(t *testing.T) {
	page := func(ids []string, ownerID, after string) string {
		var rows []string
		for _, id := range ids {
			slug := "member"
			if id == ownerID {
				slug = "owner"
			}
			rows = append(rows, fmt.Sprintf(
				`{"id":%q,"user_id":%q,"organization_id":"org_1","role":{"slug":%q},"status":"active"}`,
				"om_"+id, "user_"+id, slug))
		}
		return fmt.Sprintf(`{"data":[%s],"list_metadata":{"before":"","after":%q}}`,
			strings.Join(rows, ","), after)
	}

	var requested []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		after := r.URL.Query().Get("after")
		requested = append(requested, after)
		w.Header().Set("Content-Type", "application/json")
		switch after {
		case "":
			fmt.Fprint(w, page([]string{"a", "b"}, "", "om_b"))
		case "om_b":
			fmt.Fprint(w, page([]string{"c"}, "c", ""))
		default:
			t.Errorf("unexpected cursor %q", after)
		}
	}))
	defer srv.Close()

	c := &Client{um: &usermanagement.Client{
		APIKey:     "sk_test",
		Endpoint:   srv.URL,
		HTTPClient: srv.Client(),
		JSONEncode: json.Marshal,
	}}

	mems, err := c.ListAllMemberships(context.Background(), "org_1")
	if err != nil {
		t.Fatalf("ListAllMemberships: %v", err)
	}
	if len(mems) != 3 {
		t.Fatalf("got %d memberships, want 3 across both pages", len(mems))
	}
	if len(requested) != 2 || requested[1] != "om_b" {
		t.Fatalf("cursors requested = %v, want the second page to be fetched", requested)
	}
	if mems[2].RoleSlug != "owner" {
		t.Fatalf("owner on the second page = %q, want it visible to callers counting owners", mems[2].RoleSlug)
	}
}

func TestClassifyOrganizationError(t *testing.T) {
	notFound := workos_errors.HTTPError{Code: http.StatusNotFound}
	if err := classifyOrganizationError(notFound); !errors.Is(err, ErrOrganizationNotFound) {
		t.Fatalf("classifyOrganizationError() = %v, want ErrOrganizationNotFound", err)
	}

	serverErr := workos_errors.HTTPError{Code: http.StatusInternalServerError}
	if err := classifyOrganizationError(serverErr); errors.Is(err, ErrOrganizationNotFound) {
		t.Fatalf("classifyOrganizationError() = %v, did not want ErrOrganizationNotFound", err)
	}
}

func TestListMembershipsPagePreservesCursorAndRoles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/user_management/organization_memberships" {
			t.Fatalf("path = %q", request.URL.Path)
		}
		if got := request.URL.Query().Get("organization_id"); got != "org_123" {
			t.Fatalf("organization_id = %q", got)
		}
		if got := request.URL.Query().Get("after"); got != "om_before" {
			t.Fatalf("after = %q", got)
		}
		response.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(response).Encode(map[string]any{
			"data": []map[string]any{{
				"id": "om_123", "user_id": "user_123", "organization_id": "org_123",
				"role":   map[string]string{"slug": "admin"},
				"roles":  []map[string]string{{"slug": "admin"}, {"slug": "support"}},
				"status": "active",
			}},
			"list_metadata": map[string]any{"before": "om_before", "after": "om_after"},
		}); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	userManagement := usermanagement.NewClient("sk_test")
	userManagement.Endpoint = server.URL
	client := &Client{um: userManagement}
	page, err := client.ListMembershipsPage(context.Background(), "org_123", ListOpts{Limit: 25, After: "om_before"})
	if err != nil {
		t.Fatalf("ListMembershipsPage() error = %v", err)
	}
	if page.NextCursor != "om_after" || len(page.Memberships) != 1 {
		t.Fatalf("ListMembershipsPage() = %+v", page)
	}
	got := page.Memberships[0]
	if got.RoleSlug != "admin" || len(got.RoleSlugs) != 2 || got.RoleSlugs[1] != "support" {
		t.Fatalf("membership roles = %q / %v", got.RoleSlug, got.RoleSlugs)
	}
}
