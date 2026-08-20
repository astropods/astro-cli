package authz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"testing"
)

func TestWorkOSGroupsLifecycle(t *testing.T) {
	t.Parallel()

	requests := make(chan string, 5)
	fga, closeServer := testWorkOSFGA(t, func(response http.ResponseWriter, request *http.Request) {
		requests <- request.Method + " " + request.URL.Path
		if request.Method == http.MethodDelete {
			response.WriteHeader(http.StatusNoContent)
			return
		}
		if request.Method == http.MethodPost {
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if request.URL.Path == "/organizations/org_123/groups" && body["name"] != "Platform" {
				t.Fatalf("create body = %v", body)
			}
			if request.URL.Path != "/organizations/org_123/groups" && body["organization_membership_id"] != "om_123" {
				t.Fatalf("member body = %v", body)
			}
		}
		writeWorkOSJSON(t, response, http.StatusOK, map[string]any{
			"id": "group_123", "organization_id": "org_123", "name": "Platform", "description": nil,
		})
	})
	defer closeServer()

	if _, err := fga.CreateGroup(context.Background(), "org_123", "Platform", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := fga.UpdateGroup(context.Background(), "org_123", "group_123", "Platform", ""); err != nil {
		t.Fatal(err)
	}
	if err := fga.AddGroupMember(context.Background(), "org_123", "group_123", "om_123"); err != nil {
		t.Fatal(err)
	}
	if err := fga.RemoveGroupMember(context.Background(), "org_123", "group_123", "om_123"); err != nil {
		t.Fatal(err)
	}
	if err := fga.DeleteGroup(context.Background(), "org_123", "group_123"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"POST /organizations/org_123/groups",
		"PATCH /organizations/org_123/groups/group_123",
		"POST /organizations/org_123/groups/group_123/organization-memberships",
		"DELETE /organizations/org_123/groups/group_123/organization-memberships/om_123",
		"DELETE /organizations/org_123/groups/group_123",
	}
	for _, expected := range want {
		if got := <-requests; got != expected {
			t.Fatalf("request = %q, want %q", got, expected)
		}
	}
}

func TestWorkOSGroupsPaginationDoesNotDropOrDuplicateItems(t *testing.T) {
	t.Parallel()

	fga, closeServer := testWorkOSFGA(t, func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/organizations/org_123/groups" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.URL.Query().Get("after") == "page_2" {
			writeGroupList(t, response, []string{"group_4", "group_5"}, "")
			return
		}
		writeGroupList(t, response, []string{"group_1", "group_2", "group_3"}, "page_2")
	})
	defer closeServer()

	var got []string
	cursor := ""
	for {
		page, err := fga.ListGroups(context.Background(), "org_123", PageRequest{After: cursor, Limit: 2})
		if err != nil {
			t.Fatalf("ListGroups() error = %v", err)
		}
		for _, group := range page.Groups {
			got = append(got, group.ID)
		}
		cursor = page.NextCursor
		if cursor == "" {
			break
		}
	}
	if want := []string{"group_1", "group_2", "group_3", "group_4", "group_5"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("groups = %v, want %v", got, want)
	}
}

func TestWorkOSGroupMemberPagination(t *testing.T) {
	t.Parallel()

	fga, closeServer := testWorkOSFGA(t, func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/organizations/org_123/groups/group_123/organization-memberships" {
			t.Fatalf("path = %q", request.URL.Path)
		}
		writeWorkOSJSON(t, response, http.StatusOK, map[string]any{
			"data":          []map[string]any{{"id": "om_1", "user_id": "user_1"}, {"id": "om_2", "user_id": "user_2"}},
			"list_metadata": map[string]any{"before": nil, "after": nil},
		})
	})
	defer closeServer()

	page, err := fga.ListGroupMembers(context.Background(), "org_123", "group_123", PageRequest{Limit: 2})
	want := []GroupMember{{MembershipID: "om_1", UserID: "user_1"}, {MembershipID: "om_2", UserID: "user_2"}}
	if err != nil || !reflect.DeepEqual(page.Members, want) {
		t.Fatalf("ListGroupMembers() page=%+v error=%v", page, err)
	}
}

func TestWorkOSGroupsRejectForgedCursorWithoutRequest(t *testing.T) {
	t.Parallel()

	cursor, err := encodeWorkOSPageCursor(workOSPageCursor{Skip: 3, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	fga := &WorkOSFGA{}
	_, err = fga.ListGroups(context.Background(), "org_123", PageRequest{After: cursor, Limit: 2})
	if !errors.Is(err, ErrInvalidPageCursor) {
		t.Fatalf("ListGroups() error=%v, want ErrInvalidPageCursor", err)
	}
}

func TestFakeGroupsRejectsUnexpectedCalls(t *testing.T) {
	t.Parallel()

	fake := &FakeGroups{}
	calls := []func() error{
		func() error {
			_, err := fake.ListGroups(context.Background(), "org", PageRequest{Limit: 1})
			return err
		},
		func() error { _, err := fake.CreateGroup(context.Background(), "org", "name", ""); return err },
		func() error { _, err := fake.GetGroup(context.Background(), "org", "group"); return err },
		func() error { _, err := fake.UpdateGroup(context.Background(), "org", "group", "name", ""); return err },
		func() error { return fake.DeleteGroup(context.Background(), "org", "group") },
		func() error {
			_, err := fake.ListGroupMembers(context.Background(), "org", "group", PageRequest{Limit: 1})
			return err
		},
		func() error { return fake.AddGroupMember(context.Background(), "org", "group", "om") },
		func() error { return fake.RemoveGroupMember(context.Background(), "org", "group", "om") },
	}
	for i, call := range calls {
		if err := call(); err == nil {
			t.Fatalf("call %d returned nil error", i)
		}
	}
}

func writeGroupList(t *testing.T, response http.ResponseWriter, ids []string, after string) {
	t.Helper()
	data := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		data = append(data, map[string]any{
			"id": id, "organization_id": "org_123", "name": fmt.Sprintf("Group %s", id), "description": nil,
		})
	}
	var next any
	if after != "" {
		next = after
	}
	writeWorkOSJSON(t, response, http.StatusOK, map[string]any{
		"data": data, "list_metadata": map[string]any{"before": nil, "after": next},
	})
}
