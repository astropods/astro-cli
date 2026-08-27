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
	"time"

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

func TestWorkOSFGARegisterResourceWithParent(t *testing.T) {
	t.Parallel()

	fga, closeServer := testWorkOSFGA(t, func(response http.ResponseWriter, request *http.Request) {
		assertRequest(t, request, http.MethodPost, "/authorization/resources", map[string]any{
			"external_id":                 "blueprint_123",
			"name":                        "Support agent",
			"organization_id":             "org_123",
			"parent_resource_external_id": "account_123",
			"parent_resource_type_slug":   "account",
			"resource_type_slug":          "blueprint",
		})
		writeWorkOSJSON(t, response, http.StatusOK, map[string]any{
			"id":                 "resource_123",
			"external_id":        "blueprint_123",
			"name":               "Support agent",
			"organization_id":    "org_123",
			"resource_type_slug": "blueprint",
			"description":        nil,
			"parent_resource_id": "account_resource_123",
		})
	})
	defer closeServer()

	err := fga.RegisterResourceWithParent(
		context.Background(),
		"org_123",
		BlueprintResource("blueprint_123"),
		AccountResource("account_123"),
		"Support agent",
	)
	if err != nil {
		t.Fatalf("RegisterResourceWithParent() error = %v", err)
	}
}

func TestWorkOSFGARegisterResourceWithParentIsIdempotent(t *testing.T) {
	t.Parallel()

	fga, closeServer := testWorkOSFGA(t, func(response http.ResponseWriter, _ *http.Request) {
		writeWorkOSJSON(t, response, http.StatusConflict, map[string]string{
			"code":    "resource_exists",
			"message": "resource already exists",
		})
	})
	defer closeServer()

	err := fga.RegisterResourceWithParent(
		context.Background(),
		"org_123",
		DeploymentResource("dep_123"),
		AccountResource("account_123"),
		"Support agent",
	)
	if err != nil {
		t.Fatalf("RegisterResourceWithParent() error = %v", err)
	}
}

func TestWorkOSFGAGetResource(t *testing.T) {
	t.Parallel()

	fga, closeServer := testWorkOSFGA(t, func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/authorization/organizations/org_123/resources/deployment/dep_123" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		writeWorkOSJSON(t, response, http.StatusOK, map[string]any{
			"id": "authz_resource_123", "external_id": "dep_123", "name": "Support agent",
			"organization_id": "org_123", "resource_type_slug": "deployment",
			"parent_resource_id": "authz_account_123", "description": nil,
			"created_at": "2026-08-25T12:00:00Z", "updated_at": "2026-08-25T12:00:00Z",
		})
	})
	defer closeServer()

	resource, err := fga.GetResource(context.Background(), "org_123", DeploymentResource("dep_123"))
	if err != nil {
		t.Fatalf("GetResource() error = %v", err)
	}
	if resource.ID != "authz_resource_123" || resource.Resource != DeploymentResource("dep_123") || resource.ParentResourceID != "authz_account_123" {
		t.Fatalf("GetResource() = %+v", resource)
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

func TestWorkOSFGAListsAuthorizationResourcesForOrganization(t *testing.T) {
	t.Parallel()

	fga, closeServer := testWorkOSFGA(t, func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/authorization/resources" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		if got := request.URL.Query().Get("organization_id"); got != "org_123" {
			t.Fatalf("organization_id = %q, want org_123", got)
		}
		writeWorkOSJSON(t, response, http.StatusOK, map[string]any{
			"data": []map[string]any{{
				"id": "authz_resource_123", "external_id": "dep_123", "name": "Support agent",
				"organization_id": "org_123", "resource_type_slug": "deployment",
				"parent_resource_id": nil, "description": nil,
				"created_at": "2026-08-25T12:00:00Z", "updated_at": "2026-08-25T12:00:00Z",
			}},
			"list_metadata": map[string]any{"before": nil, "after": nil},
		})
	})
	defer closeServer()

	resources, err := fga.ListAuthorizationResourcesForOrganization(context.Background(), "org_123")
	if err != nil || len(resources) != 1 {
		t.Fatalf("ListAuthorizationResourcesForOrganization() = (%+v, %v)", resources, err)
	}
	if got := resources[0]; got.ID != "authz_resource_123" || got.Resource != DeploymentResource("dep_123") {
		t.Fatalf("resource = %+v", got)
	}
}

func TestWorkOSFGADeletesAuthorizationResourceWithoutCascade(t *testing.T) {
	t.Parallel()

	fga, closeServer := testWorkOSFGA(t, func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodDelete || request.URL.Path != "/authorization/resources/authz_resource_123" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		if got := request.URL.Query().Get("cascade_delete"); got != "false" {
			t.Fatalf("cascade_delete = %q, want false", got)
		}
		response.WriteHeader(http.StatusNoContent)
	})
	defer closeServer()

	if err := fga.DeleteAuthorizationResource(context.Background(), "authz_resource_123"); err != nil {
		t.Fatalf("DeleteAuthorizationResource() error = %v", err)
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
			name:       "get not found",
			statusCode: http.StatusNotFound,
			want:       ErrResourceNotFound,
			wantPrefix: "get WorkOS resource deployment:dep_123: workos: 404",
			call: func(fga *WorkOSFGA) error {
				_, err := fga.GetResource(context.Background(), "org_123", DeploymentResource("dep_123"))
				return err
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
				"role":                       map[string]string{"slug": "deployment-maintainer"},
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
				"role":     map[string]string{"slug": "deployment-maintainer"},
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
					"role_slug":            "deployment-maintainer",
				})
				writeWorkOSJSON(t, response, http.StatusOK, test.response)
			})
			defer closeServer()

			if err := fga.AssignRole(context.Background(), test.subject, RoleDeploymentMaintainer, DeploymentResource("dep_123")); err != nil {
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

	err := fga.AssignRole(context.Background(), MembershipAssignmentSubject("om_123"), RoleDeploymentMaintainer, DeploymentResource("dep_123"))
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
					"role_slug":            "deployment-viewer",
				})
				response.WriteHeader(http.StatusNoContent)
			})
			defer closeServer()

			if err := fga.RemoveRole(context.Background(), test.subject, RoleDeploymentViewer, DeploymentResource("dep_123")); err != nil {
				t.Fatalf("RemoveRole() error = %v", err)
			}
		})
	}
}

func TestWorkOSFGARemoveRoleClassifiesNotFound(t *testing.T) {
	t.Parallel()

	fga, closeServer := testWorkOSFGA(t, func(response http.ResponseWriter, _ *http.Request) {
		writeWorkOSJSON(t, response, http.StatusNotFound, map[string]string{
			"code":    "role_assignment_not_found",
			"message": "role assignment not found",
		})
	})
	defer closeServer()

	err := fga.RemoveRole(context.Background(), MembershipAssignmentSubject("om_123"), RoleDeploymentViewer, DeploymentResource("dep_123"))
	if !errors.Is(err, ErrRoleAssignmentNotFound) {
		t.Fatalf("error = %v, want errors.Is(_, ErrRoleAssignmentNotFound)", err)
	}
	var apiErr *workos.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want wrapped *workos.APIError", err)
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

func TestWorkOSFGAListResources(t *testing.T) {
	t.Parallel()
	var organizationLookups atomic.Int32
	lookupStarted := make(chan struct{}, 1)
	releaseLookup := make(chan struct{})
	fga, closeServer := testWorkOSFGA(t, func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		query := request.URL.Query()
		switch request.URL.Path {
		case "/authorization/resources":
			organizationLookups.Add(1)
			lookupStarted <- struct{}{}
			<-releaseLookup
			if query.Get("organization_id") != "org_123" || query.Get("resource_type_slug") != "organization" {
				t.Fatalf("organization query = %v", query)
			}
			writeWorkOSJSON(t, response, http.StatusOK, map[string]any{
				"data": []map[string]any{{
					"id": "authz_resource_org_123", "external_id": "workos-generated-root",
					"resource_type_slug": "organization", "organization_id": "org_123",
				}},
				"list_metadata": map[string]any{"before": nil, "after": nil},
			})
		case "/authorization/organization_memberships/om_123/resources", "/authorization/organization_memberships/om_456/resources":
			if query.Get("permission_slug") != "deployment:read" || query.Get("parent_resource_id") != "authz_resource_org_123" {
				t.Fatalf("membership resources query = %v", query)
			}
			writeWorkOSJSON(t, response, http.StatusOK, map[string]any{
				"data":          []map[string]any{{"external_id": "dep_123", "resource_type_slug": "deployment", "organization_id": "org_123"}},
				"list_metadata": map[string]any{"before": nil, "after": nil},
			})
		default:
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
	})
	defer closeServer()

	want := []ResourceRef{DeploymentResource("dep_123")}
	results := make(chan []ResourceRef, 2)
	errs := make(chan error, 2)
	for _, membershipID := range []string{"om_123", "om_456"} {
		go func() {
			resources, err := fga.ListResources(context.Background(), membershipID, ActionDeploymentRead, ResourceRef{Type: ResourceOrganization, ExternalID: "org_123"})
			results <- resources
			errs <- err
		}()
	}
	<-lookupStarted
	time.Sleep(50 * time.Millisecond)
	close(releaseLookup)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("ListResources() error = %v", err)
		}
		if resources := <-results; !reflect.DeepEqual(resources, want) {
			t.Fatalf("ListResources() = %v, want %v", resources, want)
		}
	}
	if got := organizationLookups.Load(); got != 1 {
		t.Fatalf("organization resource lookups = %d, want 1", got)
	}
}

func TestWorkOSFGAListResourcesRejectsMissingOrganizationResource(t *testing.T) {
	t.Parallel()
	fga, closeServer := testWorkOSFGA(t, func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/authorization/resources" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		writeWorkOSJSON(t, response, http.StatusOK, map[string]any{
			"data":          []map[string]any{},
			"list_metadata": map[string]any{"before": nil, "after": nil},
		})
	})
	defer closeServer()

	_, err := fga.ListResources(context.Background(), "om_123", ActionDeploymentRead, ResourceRef{Type: ResourceOrganization, ExternalID: "org_123"})
	if !errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("ListResources() error = %v, want ErrResourceNotFound", err)
	}
}

func TestWorkOSFGAListRoleAssignments(t *testing.T) {
	t.Parallel()
	fga, closeServer := testWorkOSFGA(t, func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/authorization/organizations/org_123/resources/deployment/dep_123/role_assignments" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		writeWorkOSJSON(t, response, http.StatusOK, map[string]any{
			"data": []map[string]any{{
				"id": "ra_123", "organization_membership_id": "om_123",
				"role":     map[string]string{"slug": "deployment-viewer"},
				"source":   map[string]any{"type": "direct", "group_role_assignment_id": nil},
				"resource": workOSResourceResponse(),
			}},
			"list_metadata": map[string]any{"before": nil, "after": nil},
		})
	})
	defer closeServer()

	assignments, err := fga.ListRoleAssignments(context.Background(), "org_123", DeploymentResource("dep_123"))
	if err != nil || len(assignments) != 1 {
		t.Fatalf("ListRoleAssignments() = %v, %v", assignments, err)
	}
	if assignments[0].Subject != MembershipAssignmentSubject("om_123") || assignments[0].Role != RoleDeploymentViewer || assignments[0].Source != AssignmentSourceDirect {
		t.Fatalf("assignment = %#v", assignments[0])
	}
}

func TestWorkOSFGAListMembershipsUsesEffectiveAccess(t *testing.T) {
	t.Parallel()
	fga, closeServer := testWorkOSFGA(t, func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/authorization/organizations/org_123/resources/deployment/dep_123/organization_memberships" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		if got := request.URL.Query().Get("permission_slug"); got != "deployment:read" {
			t.Fatalf("permission_slug = %q", got)
		}
		if got := request.URL.Query().Get("assignment"); got != "indirect" {
			t.Fatalf("assignment = %q, want indirect", got)
		}
		writeWorkOSJSON(t, response, http.StatusOK, map[string]any{
			"data":          []map[string]any{{"id": "om_direct"}, {"id": "om_inherited"}},
			"list_metadata": map[string]any{"before": nil, "after": nil},
		})
	})
	defer closeServer()

	memberships, err := fga.ListMemberships(context.Background(), "org_123", DeploymentResource("dep_123"), ActionDeploymentRead)
	if err != nil {
		t.Fatalf("ListMemberships() error = %v", err)
	}
	if want := []string{"om_direct", "om_inherited"}; !reflect.DeepEqual(memberships, want) {
		t.Fatalf("ListMemberships() = %v, want %v", memberships, want)
	}
}

func TestWorkOSFGAListGroupRoleAssignments(t *testing.T) {
	t.Parallel()
	fga, closeServer := testWorkOSFGA(t, func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/authorization/groups/group_123/role_assignments" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		writeWorkOSJSON(t, response, http.StatusOK, map[string]any{
			"data": []map[string]any{{
				"id": "gra_123", "group_id": "group_123",
				"role":     map[string]string{"slug": "deployment-maintainer"},
				"resource": map[string]string{"external_id": "dep_123", "resource_type_slug": "deployment"},
			}},
			"list_metadata": map[string]any{"before": nil, "after": nil},
		})
	})
	defer closeServer()

	assignments, err := fga.ListGroupRoleAssignments(context.Background(), "group_123")
	if err != nil || len(assignments) != 1 {
		t.Fatalf("ListGroupRoleAssignments() = %v, %v", assignments, err)
	}
	want := RoleAssignment{
		ID:       "gra_123",
		Subject:  GroupAssignmentSubject("group_123"),
		Role:     RoleDeploymentMaintainer,
		Source:   AssignmentSourceDirect,
		Resource: DeploymentResource("dep_123"),
	}
	if !reflect.DeepEqual(assignments[0], want) {
		t.Fatalf("assignment = %#v, want %#v", assignments[0], want)
	}
}

func TestWorkOSFGAListAssignmentsClassifiesMissingResource(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		call func(*WorkOSFGA) error
	}{
		{name: "effective membership", call: func(fga *WorkOSFGA) error {
			_, err := fga.ListMemberships(context.Background(), "org_123", DeploymentResource("dep_123"), ActionDeploymentRead)
			return err
		}},
		{name: "membership", call: func(fga *WorkOSFGA) error {
			_, err := fga.ListRoleAssignments(context.Background(), "org_123", DeploymentResource("dep_123"))
			return err
		}},
		{name: "group", call: func(fga *WorkOSFGA) error {
			_, err := fga.ListGroupRoleAssignments(context.Background(), "group_123")
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fga, closeServer := testWorkOSFGA(t, func(response http.ResponseWriter, _ *http.Request) {
				writeWorkOSJSON(t, response, http.StatusNotFound, map[string]string{
					"code": "entity_not_found", "message": "resource not found",
				})
			})
			defer closeServer()
			if err := test.call(fga); !errors.Is(err, ErrResourceNotFound) {
				t.Fatalf("error = %v, want ErrResourceNotFound", err)
			}
		})
	}
}

func TestWorkOSFGARejectsMalformedRoleAssignments(t *testing.T) {
	t.Parallel()

	userAssignment := func() map[string]any {
		return map[string]any{
			"id": "ra_123", "organization_membership_id": "om_123",
			"role":     map[string]string{"slug": "deployment-viewer"},
			"source":   map[string]any{"type": "direct", "group_role_assignment_id": nil},
			"resource": workOSResourceResponse(),
		}
	}
	groupAssignment := func() map[string]any {
		return map[string]any{
			"id": "gra_123", "group_id": "group_123",
			"role":     map[string]string{"slug": "deployment-maintainer"},
			"resource": map[string]string{"external_id": "dep_123", "resource_type_slug": "deployment"},
		}
	}

	tests := []struct {
		name       string
		path       string
		assignment func() map[string]any
		missing    string
		call       func(*WorkOSFGA) error
		want       string
	}{
		{
			name: "membership id", path: "/authorization/organizations/org_123/resources/deployment/dep_123/role_assignments",
			assignment: userAssignment, missing: "organization_membership_id",
			call: func(fga *WorkOSFGA) error {
				_, err := fga.ListRoleAssignments(context.Background(), "org_123", DeploymentResource("dep_123"))
				return err
			},
			want: "assignment \"ra_123\" is missing organization membership, role, source, or resource",
		},
		{
			name: "membership role", path: "/authorization/organizations/org_123/resources/deployment/dep_123/role_assignments",
			assignment: userAssignment, missing: "role",
			call: func(fga *WorkOSFGA) error {
				_, err := fga.ListRoleAssignments(context.Background(), "org_123", DeploymentResource("dep_123"))
				return err
			},
			want: "assignment \"ra_123\" is missing organization membership, role, source, or resource",
		},
		{
			name: "membership source", path: "/authorization/organizations/org_123/resources/deployment/dep_123/role_assignments",
			assignment: userAssignment, missing: "source",
			call: func(fga *WorkOSFGA) error {
				_, err := fga.ListRoleAssignments(context.Background(), "org_123", DeploymentResource("dep_123"))
				return err
			},
			want: "assignment \"ra_123\" is missing organization membership, role, source, or resource",
		},
		{
			name: "membership resource", path: "/authorization/organizations/org_123/resources/deployment/dep_123/role_assignments",
			assignment: userAssignment, missing: "resource",
			call: func(fga *WorkOSFGA) error {
				_, err := fga.ListRoleAssignments(context.Background(), "org_123", DeploymentResource("dep_123"))
				return err
			},
			want: "assignment \"ra_123\" is missing organization membership, role, source, or resource",
		},
		{
			name: "group role", path: "/authorization/groups/group_123/role_assignments",
			assignment: groupAssignment, missing: "role",
			call: func(fga *WorkOSFGA) error {
				_, err := fga.ListGroupRoleAssignments(context.Background(), "group_123")
				return err
			},
			want: "assignment \"gra_123\" is missing role or resource",
		},
		{
			name: "group resource", path: "/authorization/groups/group_123/role_assignments",
			assignment: groupAssignment, missing: "resource",
			call: func(fga *WorkOSFGA) error {
				_, err := fga.ListGroupRoleAssignments(context.Background(), "group_123")
				return err
			},
			want: "assignment \"gra_123\" is missing role or resource",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fga, closeServer := testWorkOSFGA(t, func(response http.ResponseWriter, request *http.Request) {
				if request.URL.Path != test.path {
					t.Fatalf("path = %s", request.URL.Path)
				}
				assignment := test.assignment()
				assignment[test.missing] = nil
				writeWorkOSJSON(t, response, http.StatusOK, map[string]any{
					"data": []map[string]any{assignment}, "list_metadata": map[string]any{"before": nil, "after": nil},
				})
			})
			defer closeServer()

			if err := test.call(fga); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
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
				return fga.AssignRole(context.Background(), AssignmentSubject{Type: "user", ID: "user_123"}, RoleDeploymentMaintainer, DeploymentResource("dep_123"))
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
				return fga.RemoveRole(context.Background(), AssignmentSubject{Type: "user", ID: "user_123"}, RoleDeploymentMaintainer, DeploymentResource("dep_123"))
			},
		},
		{
			name: "remove empty role",
			call: func(fga *WorkOSFGA) error {
				return fga.RemoveRole(context.Background(), MembershipAssignmentSubject("om_123"), "", DeploymentResource("dep_123"))
			},
		},
		{
			name: "list resources with empty membership id",
			call: func(fga *WorkOSFGA) error {
				_, err := fga.ListResources(context.Background(), "", ActionDeploymentRead, ResourceRef{Type: ResourceOrganization, ExternalID: "org_123"})
				return err
			},
		},
		{
			name: "list role assignments with empty organization id",
			call: func(fga *WorkOSFGA) error {
				_, err := fga.ListRoleAssignments(context.Background(), "", DeploymentResource("dep_123"))
				return err
			},
		},
		{
			name: "list group role assignments with empty group id",
			call: func(fga *WorkOSFGA) error {
				_, err := fga.ListGroupRoleAssignments(context.Background(), "")
				return err
			},
		},
		{
			name: "list memberships with empty organization id",
			call: func(fga *WorkOSFGA) error {
				_, err := fga.ListMemberships(context.Background(), "", DeploymentResource("dep_123"), ActionDeploymentRead)
				return err
			},
		},
		{
			name: "list memberships with empty action",
			call: func(fga *WorkOSFGA) error {
				_, err := fga.ListMemberships(context.Background(), "org_123", DeploymentResource("dep_123"), "")
				return err
			},
		},
		{
			name: "list memberships with empty resource",
			call: func(fga *WorkOSFGA) error {
				_, err := fga.ListMemberships(context.Background(), "org_123", ResourceRef{}, ActionDeploymentRead)
				return err
			},
		},
		{
			name: "list resources with empty parent",
			call: func(fga *WorkOSFGA) error {
				_, err := fga.ListResources(context.Background(), "om_123", ActionDeploymentRead, ResourceRef{})
				return err
			},
		},
		{
			name: "list resources with empty action",
			call: func(fga *WorkOSFGA) error {
				_, err := fga.ListResources(context.Background(), "om_123", "", ResourceRef{Type: ResourceOrganization, ExternalID: "org_123"})
				return err
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

	if err := (&FakeFGA{}).RegisterResourceWithParent(context.Background(), "org_123", DeploymentResource("dep_123"), AccountResource("account_123"), "Support agent"); err == nil {
		t.Fatal("RegisterResourceWithParent() error = nil, want unexpected-call error")
	}
	if _, err := (&FakeFGA{}).GetResource(context.Background(), "org_123", DeploymentResource("dep_123")); err == nil {
		t.Fatal("GetResource() error = nil, want unexpected-call error")
	}
	if _, err := (&FakeFGA{}).Check(context.Background(), "om_123", ActionDeploymentRead, DeploymentResource("dep_123")); err == nil {
		t.Fatal("Check() error = nil, want unexpected-call error")
	}
	if _, err := (&FakeFGA{}).ListEffectivePermissions(context.Background(), "om_123", DeploymentResource("dep_123")); err == nil {
		t.Fatal("ListEffectivePermissions() error = nil, want unexpected-call error")
	}
	if _, err := (&FakeFGA{}).ListResources(context.Background(), "om_123", ActionDeploymentRead, ResourceRef{Type: ResourceOrganization, ExternalID: "org_123"}); err == nil {
		t.Fatal("ListResources() error = nil, want unexpected-call error")
	}
	if _, err := (&FakeFGA{}).ListRoleAssignments(context.Background(), "org_123", DeploymentResource("dep_123")); err == nil {
		t.Fatal("ListRoleAssignments() error = nil, want unexpected-call error")
	}
	if _, err := (&FakeFGA{}).ListGroupRoleAssignments(context.Background(), "group_123"); err == nil {
		t.Fatal("ListGroupRoleAssignments() error = nil, want unexpected-call error")
	}
	if _, err := (&FakeFGA{}).ListMemberships(context.Background(), "org_123", DeploymentResource("dep_123"), ActionDeploymentRead); err == nil {
		t.Fatal("ListMemberships() error = nil, want unexpected-call error")
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
