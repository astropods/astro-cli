package authz

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	workos "github.com/workos/workos-go/v10"
)

func TestWorkOSFGARegisterResource(t *testing.T) {
	t.Parallel()

	fga, closeServer := testWorkOSFGA(t, func(response http.ResponseWriter, request *http.Request) {
		assertRequest(t, request, http.MethodPost, "/authorization/resources", map[string]any{
			"external_id":        "dep_123",
			"name":               "Support agent",
			"organization_id":    "org_123",
			"resource_type_slug": "deployment",
		})
		writeWorkOSJSON(t, response, http.StatusOK, map[string]any{
			"id":                 "resource_123",
			"external_id":        "dep_123",
			"name":               "Support agent",
			"organization_id":    "org_123",
			"resource_type_slug": "deployment",
			"description":        nil,
			"parent_resource_id": nil,
		})
	})
	defer closeServer()

	if err := fga.RegisterResource(context.Background(), "org_123", DeploymentResource("dep_123"), "Support agent"); err != nil {
		t.Fatalf("RegisterResource() error = %v", err)
	}
}

func TestWorkOSFGAUpdateResourceName(t *testing.T) {
	t.Parallel()

	fga, closeServer := testWorkOSFGA(t, func(response http.ResponseWriter, request *http.Request) {
		assertRequest(t, request, http.MethodPatch, "/authorization/organizations/org_123/resources/deployment/dep_123", map[string]any{
			"name": "Renamed support agent",
		})
		writeWorkOSJSON(t, response, http.StatusOK, map[string]any{
			"id":                 "resource_123",
			"external_id":        "dep_123",
			"name":               "Renamed support agent",
			"organization_id":    "org_123",
			"resource_type_slug": "deployment",
			"description":        nil,
			"parent_resource_id": nil,
		})
	})
	defer closeServer()

	if err := fga.UpdateResourceName(context.Background(), "org_123", DeploymentResource("dep_123"), "Renamed support agent"); err != nil {
		t.Fatalf("UpdateResourceName() error = %v", err)
	}
}

func TestWorkOSFGADeleteResourceCascadesAssignments(t *testing.T) {
	t.Parallel()

	fga, closeServer := testWorkOSFGA(t, func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodDelete || request.URL.Path != "/authorization/organizations/org_123/resources/deployment/dep_123" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		if got := request.URL.Query().Get("cascade_delete"); got != "true" {
			t.Fatalf("cascade_delete = %q, want true", got)
		}
		response.WriteHeader(http.StatusNoContent)
	})
	defer closeServer()

	if err := fga.DeleteResource(context.Background(), "org_123", DeploymentResource("dep_123")); err != nil {
		t.Fatalf("DeleteResource() error = %v", err)
	}
}

func TestWorkOSFGAClassifiesResourceErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		want       error
		wantPrefix string
		call       func(*WorkOSFGA) error
	}{
		{
			name:       "register conflict",
			statusCode: http.StatusConflict,
			want:       ErrResourceExists,
			wantPrefix: "register WorkOS resource deployment:dep_123: workos: 409",
			call: func(fga *WorkOSFGA) error {
				return fga.RegisterResource(context.Background(), "org_123", DeploymentResource("dep_123"), "Support agent")
			},
		},
		{
			name:       "delete not found",
			statusCode: http.StatusNotFound,
			want:       ErrResourceNotFound,
			wantPrefix: "delete WorkOS resource deployment:dep_123: workos: 404",
			call: func(fga *WorkOSFGA) error {
				return fga.DeleteResource(context.Background(), "org_123", DeploymentResource("dep_123"))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fga, closeServer := testWorkOSFGA(t, func(response http.ResponseWriter, _ *http.Request) {
				writeWorkOSJSON(t, response, test.statusCode, map[string]string{
					"code":    "resource_error",
					"message": "resource operation failed",
				})
			})
			defer closeServer()

			err := test.call(fga)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want errors.Is(_, %v)", err, test.want)
			}
			if !strings.HasPrefix(err.Error(), test.wantPrefix) {
				t.Fatalf("error = %q, want prefix %q", err, test.wantPrefix)
			}
			var apiErr *workos.APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("error = %v, want wrapped *workos.APIError", err)
			}
			if apiErr.StatusCode != test.statusCode {
				t.Fatalf("status code = %d, want %d", apiErr.StatusCode, test.statusCode)
			}
		})
	}
}

func TestWorkOSFGAAssignRoleSupportsMembershipsAndGroups(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		subject  AssignmentSubject
		wantPath string
		response map[string]any
	}{
		{
			name:     "membership",
			subject:  MembershipAssignmentSubject("om_123"),
			wantPath: "/authorization/organization_memberships/om_123/role_assignments",
			response: map[string]any{
				"id":                         "role_assignment_123",
				"organization_membership_id": "om_123",
				"role":                       map[string]string{"slug": "deployment-editor"},
				"resource":                   workOSResourceResponse(),
			},
		},
		{
			name:     "group",
			subject:  GroupAssignmentSubject("group_123"),
			wantPath: "/authorization/groups/group_123/role_assignments",
			response: map[string]any{
				"id":       "group_role_assignment_123",
				"group_id": "group_123",
				"role":     map[string]string{"slug": "deployment-editor"},
				"resource": workOSResourceResponse(),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fga, closeServer := testWorkOSFGA(t, func(response http.ResponseWriter, request *http.Request) {
				assertRequest(t, request, http.MethodPost, test.wantPath, map[string]any{
					"resource_external_id": "dep_123",
					"resource_type_slug":   "deployment",
					"role_slug":            "deployment-editor",
				})
				writeWorkOSJSON(t, response, http.StatusOK, test.response)
			})
			defer closeServer()

			if err := fga.AssignRole(context.Background(), test.subject, RoleDeploymentEditor, DeploymentResource("dep_123")); err != nil {
				t.Fatalf("AssignRole() error = %v", err)
			}
		})
	}
}

func TestWorkOSFGAAssignRoleClassifiesConflict(t *testing.T) {
	t.Parallel()

	fga, closeServer := testWorkOSFGA(t, func(response http.ResponseWriter, _ *http.Request) {
		writeWorkOSJSON(t, response, http.StatusConflict, map[string]string{
			"code":    "role_assignment_exists",
			"message": "role assignment already exists",
		})
	})
	defer closeServer()

	err := fga.AssignRole(context.Background(), MembershipAssignmentSubject("om_123"), RoleDeploymentEditor, DeploymentResource("dep_123"))
	if !errors.Is(err, ErrRoleAssignmentExists) {
		t.Fatalf("error = %v, want errors.Is(_, ErrRoleAssignmentExists)", err)
	}
	var apiErr *workos.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want wrapped *workos.APIError", err)
	}
}

func TestWorkOSFGARemoveRoleSupportsMembershipsAndGroups(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		subject  AssignmentSubject
		wantPath string
	}{
		{"membership", MembershipAssignmentSubject("om_123"), "/authorization/organization_memberships/om_123/role_assignments"},
		{"group", GroupAssignmentSubject("group_123"), "/authorization/groups/group_123/role_assignments"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fga, closeServer := testWorkOSFGA(t, func(response http.ResponseWriter, request *http.Request) {
				assertRequest(t, request, http.MethodDelete, test.wantPath, map[string]any{
					"resource_external_id": "dep_123",
					"resource_type_slug":   "deployment",
					"role_slug":            "deployment-reader",
				})
				response.WriteHeader(http.StatusNoContent)
			})
			defer closeServer()

			if err := fga.RemoveRole(context.Background(), test.subject, RoleDeploymentReader, DeploymentResource("dep_123")); err != nil {
				t.Fatalf("RemoveRole() error = %v", err)
			}
		})
	}
}

func TestWorkOSFGACheck(t *testing.T) {
	t.Parallel()

	for _, authorized := range []bool{true, false} {
		t.Run(map[bool]string{true: "authorized", false: "denied"}[authorized], func(t *testing.T) {
			t.Parallel()
			fga, closeServer := testWorkOSFGA(t, func(response http.ResponseWriter, request *http.Request) {
				assertRequest(t, request, http.MethodPost, "/authorization/organization_memberships/om_123/check", map[string]any{
					"permission_slug":      "deployment:read",
					"resource_external_id": "dep_123",
					"resource_type_slug":   "deployment",
				})
				writeWorkOSJSON(t, response, http.StatusOK, map[string]bool{"authorized": authorized})
			})
			defer closeServer()

			allowed, err := fga.Check(context.Background(), "om_123", ActionDeploymentRead, DeploymentResource("dep_123"))
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			if allowed != authorized {
				t.Fatalf("Check() = %v, want %v", allowed, authorized)
			}
		})
	}
}

func TestWorkOSFGAListEffectivePermissions(t *testing.T) {
	t.Parallel()

	fga, closeServer := testWorkOSFGA(t, func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/authorization/organization_memberships/om_123/resources/deployment/dep_123/permissions" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		writeWorkOSJSON(t, response, http.StatusOK, map[string]any{
			"data": []map[string]any{
				{"slug": "deployment:read", "resource_type_slug": "deployment"},
				{"slug": "deployment:operate", "resource_type_slug": "deployment"},
			},
			"list_metadata": map[string]any{"before": nil, "after": nil},
		})
	})
	defer closeServer()

	permissions, err := fga.ListEffectivePermissions(context.Background(), "om_123", DeploymentResource("dep_123"))
	if err != nil {
		t.Fatalf("ListEffectivePermissions() error = %v", err)
	}
	want := []Action{ActionDeploymentRead, ActionDeploymentOperate}
	if !reflect.DeepEqual(permissions, want) {
		t.Fatalf("ListEffectivePermissions() = %v, want %v", permissions, want)
	}
}

func TestWorkOSFGAValidationDoesNotCallWorkOS(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	fga, closeServer := testWorkOSFGA(t, func(response http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		response.WriteHeader(http.StatusInternalServerError)
	})
	defer closeServer()

	tests := []struct {
		name string
		call func(*WorkOSFGA) error
	}{
		{
			name: "register resource with empty organization id",
			call: func(fga *WorkOSFGA) error {
				return fga.RegisterResource(context.Background(), "", DeploymentResource("dep_123"), "Support agent")
			},
		},
		{
			name: "register resource with empty resource type",
			call: func(fga *WorkOSFGA) error {
				return fga.RegisterResource(context.Background(), "org_123", ResourceRef{ExternalID: "dep_123"}, "Support agent")
			},
		},
		{
			name: "register resource with empty resource external id",
			call: func(fga *WorkOSFGA) error {
				return fga.RegisterResource(context.Background(), "org_123", ResourceRef{Type: ResourceDeployment}, "Support agent")
			},
		},
		{
			name: "register resource with empty name",
			call: func(fga *WorkOSFGA) error {
				return fga.RegisterResource(context.Background(), "org_123", DeploymentResource("dep_123"), "")
			},
		},
		{
			name: "update resource name with empty organization id",
			call: func(fga *WorkOSFGA) error {
				return fga.UpdateResourceName(context.Background(), "", DeploymentResource("dep_123"), "Support agent")
			},
		},
		{
			name: "update resource name with empty resource type",
			call: func(fga *WorkOSFGA) error {
				return fga.UpdateResourceName(context.Background(), "org_123", ResourceRef{ExternalID: "dep_123"}, "Support agent")
			},
		},
		{
			name: "update resource name with empty resource external id",
			call: func(fga *WorkOSFGA) error {
				return fga.UpdateResourceName(context.Background(), "org_123", ResourceRef{Type: ResourceDeployment}, "Support agent")
			},
		},
		{
			name: "update resource with empty name",
			call: func(fga *WorkOSFGA) error {
				return fga.UpdateResourceName(context.Background(), "org_123", DeploymentResource("dep_123"), "")
			},
		},
		{
			name: "delete resource with empty organization id",
			call: func(fga *WorkOSFGA) error {
				return fga.DeleteResource(context.Background(), "", DeploymentResource("dep_123"))
			},
		},
		{
			name: "delete resource with empty resource type",
			call: func(fga *WorkOSFGA) error {
				return fga.DeleteResource(context.Background(), "org_123", ResourceRef{ExternalID: "dep_123"})
			},
		},
		{
			name: "delete resource with empty resource external id",
			call: func(fga *WorkOSFGA) error {
				return fga.DeleteResource(context.Background(), "org_123", ResourceRef{Type: ResourceDeployment})
			},
		},
		{
			name: "assign role with unsupported subject type",
			call: func(fga *WorkOSFGA) error {
				return fga.AssignRole(context.Background(), AssignmentSubject{Type: "user", ID: "user_123"}, RoleDeploymentEditor, DeploymentResource("dep_123"))
			},
		},
		{
			name: "assign empty role",
			call: func(fga *WorkOSFGA) error {
				return fga.AssignRole(context.Background(), MembershipAssignmentSubject("om_123"), "", DeploymentResource("dep_123"))
			},
		},
		{
			name: "remove role with unsupported subject type",
			call: func(fga *WorkOSFGA) error {
				return fga.RemoveRole(context.Background(), AssignmentSubject{Type: "user", ID: "user_123"}, RoleDeploymentEditor, DeploymentResource("dep_123"))
			},
		},
		{
			name: "remove empty role",
			call: func(fga *WorkOSFGA) error {
				return fga.RemoveRole(context.Background(), MembershipAssignmentSubject("om_123"), "", DeploymentResource("dep_123"))
			},
		},
		{
			name: "check with empty membership id",
			call: func(fga *WorkOSFGA) error {
				_, err := fga.Check(context.Background(), "", ActionDeploymentRead, DeploymentResource("dep_123"))
				return err
			},
		},
		{
			name: "check with empty action",
			call: func(fga *WorkOSFGA) error {
				_, err := fga.Check(context.Background(), "om_123", "", DeploymentResource("dep_123"))
				return err
			},
		},
		{
			name: "check with empty resource type",
			call: func(fga *WorkOSFGA) error {
				_, err := fga.Check(context.Background(), "om_123", ActionDeploymentRead, ResourceRef{ExternalID: "dep_123"})
				return err
			},
		},
		{
			name: "check with empty resource external id",
			call: func(fga *WorkOSFGA) error {
				_, err := fga.Check(context.Background(), "om_123", ActionDeploymentRead, ResourceRef{Type: ResourceDeployment})
				return err
			},
		},
		{
			name: "list effective permissions with empty membership id",
			call: func(fga *WorkOSFGA) error {
				_, err := fga.ListEffectivePermissions(context.Background(), "", DeploymentResource("dep_123"))
				return err
			},
		},
		{
			name: "list effective permissions with empty resource",
			call: func(fga *WorkOSFGA) error {
				_, err := fga.ListEffectivePermissions(context.Background(), "om_123", ResourceRef{})
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(fga); err == nil {
				t.Fatal("error = nil, want validation error")
			}
		})
	}

	if got := requests.Load(); got != 0 {
		t.Fatalf("WorkOS requests = %d, want 0", got)
	}
}

func TestFakeFGARejectsUnexpectedCalls(t *testing.T) {
	t.Parallel()

	if err := (&FakeFGA{}).UpdateResourceName(context.Background(), "org_123", DeploymentResource("dep_123"), "Support agent"); err == nil {
		t.Fatal("UpdateResourceName() error = nil, want unexpected-call error")
	}
	if _, err := (&FakeFGA{}).Check(context.Background(), "om_123", ActionDeploymentRead, DeploymentResource("dep_123")); err == nil {
		t.Fatal("Check() error = nil, want unexpected-call error")
	}
	if _, err := (&FakeFGA{}).ListEffectivePermissions(context.Background(), "om_123", DeploymentResource("dep_123")); err == nil {
		t.Fatal("ListEffectivePermissions() error = nil, want unexpected-call error")
	}
}

func testWorkOSFGA(t *testing.T, handler http.HandlerFunc) (*WorkOSFGA, func()) {
	t.Helper()
	server := httptest.NewServer(handler)
	client := workos.NewClient("sk_test", workos.WithBaseURL(server.URL), workos.WithMaxRetries(0))
	return newWorkOSFGA(client), server.Close
}

func assertRequest(t *testing.T, request *http.Request, method, path string, wantBody map[string]any) {
	t.Helper()
	if request.Method != method || request.URL.Path != path {
		t.Fatalf("request = %s %s, want %s %s", request.Method, request.URL.Path, method, path)
	}
	var gotBody map[string]any
	if err := json.NewDecoder(request.Body).Decode(&gotBody); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if !reflect.DeepEqual(gotBody, wantBody) {
		t.Fatalf("request body = %#v, want %#v", gotBody, wantBody)
	}
}

func writeWorkOSJSON(t *testing.T, response http.ResponseWriter, status int, value any) {
	t.Helper()
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	if err := json.NewEncoder(response).Encode(value); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func workOSResourceResponse() map[string]string {
	return map[string]string{
		"id":                 "resource_123",
		"external_id":        "dep_123",
		"resource_type_slug": "deployment",
	}
}
