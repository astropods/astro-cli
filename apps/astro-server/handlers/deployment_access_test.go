package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

type fakeDeploymentAccessService struct {
	list   func(context.Context, authz.ResourceRef) ([]authz.AccessAssignment, []authz.AccessIntent, error)
	assign func(context.Context, authz.ResourceRef, authz.AssignmentSubjectType, string, authz.AccessLevel) (authz.AccessIntent, bool, error)
	remove func(context.Context, authz.ResourceRef, authz.AssignmentSubjectType, string) (authz.AccessIntent, bool, error)
}

type recordingDeploymentAccessQueue struct {
	keys []authz.AccessIntentKey
}

func (q *recordingDeploymentAccessQueue) InsertResourceAccessFGAReconcileJob(_ context.Context, key authz.AccessIntentKey) error {
	q.keys = append(q.keys, key)
	return nil
}

type recordingDeploymentAccessAuditStore struct {
	events []auditlog.Event
}

func (s *recordingDeploymentAccessAuditStore) LogAsync(_ *logger.Logger, event auditlog.Event) {
	s.events = append(s.events, event)
}

func (f fakeDeploymentAccessService) ListAccess(ctx context.Context, resource authz.ResourceRef) ([]authz.AccessAssignment, []authz.AccessIntent, error) {
	if f.list == nil {
		return nil, nil, nil
	}
	return f.list(ctx, resource)
}

func (f fakeDeploymentAccessService) Assign(ctx context.Context, resource authz.ResourceRef, subjectType authz.AssignmentSubjectType, subjectID string, level authz.AccessLevel) (authz.AccessIntent, bool, error) {
	return f.assign(ctx, resource, subjectType, subjectID, level)
}

func (f fakeDeploymentAccessService) Remove(ctx context.Context, resource authz.ResourceRef, subjectType authz.AssignmentSubjectType, subjectID string) (authz.AccessIntent, bool, error) {
	return f.remove(ctx, resource, subjectType, subjectID)
}

func pendingDeploymentAccessIntent(role authz.RoleSlug) authz.AccessIntent {
	return authz.AccessIntent{
		AccountID: "acct_123", OrganizationID: "org_123", Resource: authz.DeploymentResource("dep_123"),
		Subject: authz.MembershipAssignmentSubject("om_123"), SubjectID: "user_123",
		DesiredRole: role, DesiredVersion: 2,
	}
}

func TestListDeploymentAccessReturnsRolesAndAssignmentSources(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := fakeDeploymentAccessService{
		list: func(ctx context.Context, resource authz.ResourceRef) ([]authz.AccessAssignment, []authz.AccessIntent, error) {
			if _, ok := ctx.Deadline(); !ok || resource != authz.DeploymentResource("dep_123") {
				t.Fatalf("context deadline/resource = %v/%+v", ok, resource)
			}
			return []authz.AccessAssignment{{
				ID: "ra_123", UserID: "user_123", Level: authz.AccessLevelMaintainer,
				Role: authz.RoleDeploymentMaintainer, Source: authz.AssignmentSourceGroup, GroupRoleAssignmentID: "gra_123",
			}}, []authz.AccessIntent{pendingDeploymentAccessIntent(authz.RoleDeploymentViewer)}, nil
		},
	}
	router := gin.New()
	router.GET("/api/v1/deployments/:id/access", ListDeploymentAccess(logger.New("error", "json"), service))

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep_123/access", nil))
	if response.Code != http.StatusOK || !containsAll(response.Body.String(),
		`"deployment_id":"dep_123"`, `"role":"viewer"`, `"role":"writer"`, `"role":"maintainer"`, `"role":"admin"`,
		`"subject_id":"user_123"`, `"source":"group"`, `"group_role_assignment_id":"gra_123"`,
		`"desired_role":"viewer"`, `"sync_status":"pending"`, `"desired_version":2`,
	) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestSetDeploymentAccessUsesProductRoleAndSubject(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := fakeDeploymentAccessService{assign: func(_ context.Context, resource authz.ResourceRef, subjectType authz.AssignmentSubjectType, subjectID string, level authz.AccessLevel) (authz.AccessIntent, bool, error) {
		if resource != authz.DeploymentResource("dep_123") || subjectType != authz.AssignmentSubjectMembership || subjectID != "user_123" || level != authz.AccessLevelMaintainer {
			t.Fatalf("Assign(%+v, %q, %q, %q)", resource, subjectType, subjectID, level)
		}
		return pendingDeploymentAccessIntent(authz.RoleDeploymentMaintainer), true, nil
	}}
	queue := &recordingDeploymentAccessQueue{}
	router := gin.New()
	router.PUT("/api/v1/deployments/:id/access", SetDeploymentAccess(logger.New("error", "json"), service, queue, nil))

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/v1/deployments/dep_123/access", strings.NewReader(`{"subject_type":"member","subject_id":"user_123","role":"maintainer"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || !containsAll(response.Body.String(), `"status":"pending"`, `"desired_role":"maintainer"`) || len(queue.keys) != 1 {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestSetDeploymentAccessRejectsUnknownRoleBeforeService(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := fakeDeploymentAccessService{assign: func(context.Context, authz.ResourceRef, authz.AssignmentSubjectType, string, authz.AccessLevel) (authz.AccessIntent, bool, error) {
		t.Fatal("invalid role reached service")
		return authz.AccessIntent{}, false, nil
	}}
	router := gin.New()
	router.PUT("/api/v1/deployments/:id/access", SetDeploymentAccess(logger.New("error", "json"), service, nil, nil))

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/v1/deployments/dep_123/access", strings.NewReader(`{"subject_type":"member","subject_id":"user_123","role":"builder"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestDeploymentAccessAcceptsGroupSubject(t *testing.T) {
	t.Parallel()

	got, ok := deploymentAccessSubjectType("group")
	if !ok || got != authz.AssignmentSubjectGroup {
		t.Fatalf("deploymentAccessSubjectType(group) = %q, %v", got, ok)
	}
}

func TestSetDeploymentAccessReportsUnprovisionedMember(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := fakeDeploymentAccessService{assign: func(context.Context, authz.ResourceRef, authz.AssignmentSubjectType, string, authz.AccessLevel) (authz.AccessIntent, bool, error) {
		return authz.AccessIntent{}, false, authz.ErrAccessSubjectNotProvisioned
	}}
	router := gin.New()
	router.PUT("/api/v1/deployments/:id/access", SetDeploymentAccess(logger.New("error", "json"), service, nil, nil))

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/v1/deployments/dep_123/access", strings.NewReader(`{"subject_type":"member","subject_id":"user_123","role":"maintainer"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "selected member is not yet provisioned") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRemoveDeploymentAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		path       string
		remove     func(context.Context, authz.ResourceRef, authz.AssignmentSubjectType, string) (authz.AccessIntent, bool, error)
		wantStatus int
	}{
		{
			name: "removes member access",
			path: "/api/v1/deployments/dep_123/access/member/user_123",
			remove: func(_ context.Context, resource authz.ResourceRef, subjectType authz.AssignmentSubjectType, subjectID string) (authz.AccessIntent, bool, error) {
				if resource != authz.DeploymentResource("dep_123") || subjectType != authz.AssignmentSubjectMembership || subjectID != "user_123" {
					t.Fatalf("Remove(%+v, %q, %q)", resource, subjectType, subjectID)
				}
				return pendingDeploymentAccessIntent(""), true, nil
			},
			wantStatus: http.StatusAccepted,
		},
		{
			name: "rejects unknown subject type",
			path: "/api/v1/deployments/dep_123/access/bot/bot_123",
			remove: func(context.Context, authz.ResourceRef, authz.AssignmentSubjectType, string) (authz.AccessIntent, bool, error) {
				t.Fatal("invalid subject type reached service")
				return authz.AccessIntent{}, false, nil
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "maps service error",
			path: "/api/v1/deployments/dep_123/access/member/user_123",
			remove: func(context.Context, authz.ResourceRef, authz.AssignmentSubjectType, string) (authz.AccessIntent, bool, error) {
				return authz.AccessIntent{}, false, authz.ErrResourceNotVisible
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "reports unprovisioned member",
			path: "/api/v1/deployments/dep_123/access/member/user_123",
			remove: func(context.Context, authz.ResourceRef, authz.AssignmentSubjectType, string) (authz.AccessIntent, bool, error) {
				return authz.AccessIntent{}, false, authz.ErrAccessSubjectNotProvisioned
			},
			wantStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := fakeDeploymentAccessService{remove: tt.remove}
			router := gin.New()
			router.DELETE("/api/v1/deployments/:id/access/:subject_type/:subject_id", RemoveDeploymentAccess(logger.New("error", "json"), service, nil, nil))

			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, tt.path, nil))
			if response.Code != tt.wantStatus {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestDeploymentAccessMutationAuditsChangesOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)

	operations := []struct {
		name, method, path, body, action string
	}{
		{name: "grant", method: http.MethodPut, path: "/api/v1/deployments/dep_123/access", body: `{"subject_type":"member","subject_id":"user_123","role":"maintainer"}`, action: auditlog.DeploymentGrantAccess},
		{name: "revoke", method: http.MethodDelete, path: "/api/v1/deployments/dep_123/access/member/user_123", action: auditlog.DeploymentRevokeAccess},
	}
	for _, operation := range operations {
		for _, changed := range []bool{true, false} {
			t.Run(operation.name+map[bool]string{true: "_changed", false: "_unchanged"}[changed], func(t *testing.T) {
				auditStore := &recordingDeploymentAccessAuditStore{}
				service := fakeDeploymentAccessService{
					assign: func(context.Context, authz.ResourceRef, authz.AssignmentSubjectType, string, authz.AccessLevel) (authz.AccessIntent, bool, error) {
						return pendingDeploymentAccessIntent(authz.RoleDeploymentMaintainer), changed, nil
					},
					remove: func(context.Context, authz.ResourceRef, authz.AssignmentSubjectType, string) (authz.AccessIntent, bool, error) {
						return pendingDeploymentAccessIntent(""), changed, nil
					},
				}
				router := gin.New()
				if operation.method == http.MethodPut {
					router.PUT("/api/v1/deployments/:id/access", SetDeploymentAccess(logger.New("error", "json"), service, nil, auditStore))
				} else {
					router.DELETE("/api/v1/deployments/:id/access/:subject_type/:subject_id", RemoveDeploymentAccess(logger.New("error", "json"), service, nil, auditStore))
				}

				response := httptest.NewRecorder()
				request := httptest.NewRequest(operation.method, operation.path, strings.NewReader(operation.body))
				request.Header.Set("Content-Type", "application/json")
				router.ServeHTTP(response, request)
				if response.Code != http.StatusAccepted {
					t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
				}

				if changed {
					if len(auditStore.events) != 1 {
						t.Fatalf("audit events = %d, want 1", len(auditStore.events))
					}
					event := auditStore.events[0]
					metadata, ok := event.Metadata.(map[string]any)
					if event.AccountID != "acct_123" || event.Action != operation.action || event.ResourceType != "deployment" || event.ResourceID != "dep_123" || !ok || metadata["subject_type"] != "member" || metadata["subject_id"] != "user_123" || metadata["sync_status"] != authz.AccessSyncPending {
						t.Fatalf("audit event = %+v", event)
					}
					if operation.method == http.MethodPut && metadata["role"] != "maintainer" {
						t.Fatalf("audit metadata = %#v", metadata)
					}
				} else if len(auditStore.events) != 0 {
					t.Fatalf("audit events = %d, want 0", len(auditStore.events))
				}
			})
		}
	}
}

func TestDeploymentAccessErrorsConcealDisabledAndRetryMissingIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, test := range []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "disabled", err: authz.ErrAccessManagementUnavailable, wantStatus: http.StatusNotFound},
		{name: "hidden", err: authz.ErrResourceNotVisible, wantStatus: http.StatusNotFound},
		{name: "membership", err: authz.ErrWorkOSMembershipUnavailable, wantStatus: http.StatusServiceUnavailable},
		{name: "workos", err: errors.New("workos unavailable"), wantStatus: http.StatusServiceUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := fakeDeploymentAccessService{list: func(context.Context, authz.ResourceRef) ([]authz.AccessAssignment, []authz.AccessIntent, error) {
				return nil, nil, test.err
			}}
			router := gin.New()
			router.GET("/api/v1/deployments/:id/access", ListDeploymentAccess(logger.New("error", "json"), service))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep_123/access", nil))
			if response.Code != test.wantStatus {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}
