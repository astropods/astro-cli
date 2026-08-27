package admingrpc

import (
	"context"
	"reflect"
	"sync"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/org"
	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
)

type fakeOrganizationMembershipLister struct {
	mu    sync.Mutex
	pages map[string]org.MembershipPage
	after []string
}

func (f *fakeOrganizationMembershipLister) ListMembershipsPage(_ context.Context, _ string, opts org.ListOpts) (org.MembershipPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.after = append(f.after, opts.After)
	return f.pages[opts.After], nil
}

func TestGetDeploymentAccessExplainsEffectiveAccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(`SELECT`).WillReturnRows(deploymentRow("dep-1", "acct-1", "astro-abc-0"))
	mock.ExpectQuery(`SELECT a.type, COALESCE\(ao.workos_org_id`).
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"type", "workos_org_id"}).AddRow("organization", "org-1"))
	mock.ExpectQuery(`SELECT am.user_id, COALESCE`).
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "email"}).
			AddRow("user-owner", "owner@example.com").
			AddRow("user-maintainer", "maintainer@example.com").
			AddRow("user-viewer", "viewer@example.com").
			AddRow("user-none", "none@example.com"))

	memberships := &fakeOrganizationMembershipLister{pages: map[string]org.MembershipPage{
		"": {
			Memberships: []org.Membership{
				{ID: "om-owner", UserID: "user-owner", RoleSlugs: []string{"member", "owner"}, Status: "active"},
				{ID: "om-maintainer", UserID: "user-maintainer", RoleSlugs: []string{"member"}, Status: "active"},
			},
			NextCursor: "om-maintainer",
		},
		"om-maintainer": {
			Memberships: []org.Membership{
				{ID: "om-viewer", UserID: "user-viewer", RoleSlugs: []string{"member"}, Status: "active"},
				{ID: "om-none", UserID: "user-none", RoleSlugs: []string{"member"}, Status: "active"},
			},
		},
	}}
	resource := authz.DeploymentResource("dep-1")
	fga := &authz.FakeFGA{
		ListRoleAssignmentsFunc: func(_ context.Context, organizationID string, got authz.ResourceRef) ([]authz.RoleAssignment, error) {
			if organizationID != "org-1" || got != resource {
				t.Fatalf("ListRoleAssignments(%q, %+v)", organizationID, got)
			}
			return []authz.RoleAssignment{
				{Subject: authz.MembershipAssignmentSubject("om-maintainer"), Role: authz.RoleDeploymentMaintainer, Source: "group", Resource: resource},
				{Subject: authz.MembershipAssignmentSubject("om-viewer"), Role: authz.RoleDeploymentViewer, Source: "direct", Resource: resource},
			}, nil
		},
		ListMembershipsFunc: func(_ context.Context, organizationID string, got authz.ResourceRef, action authz.Action) ([]string, error) {
			if organizationID != "org-1" || got != resource {
				t.Fatalf("ListMemberships(%q, %+v, %q)", organizationID, got, action)
			}
			switch action {
			case authz.ActionDeploymentRead:
				return []string{"om-owner", "om-maintainer", "om-viewer"}, nil
			case authz.ActionDeploymentEdit, authz.ActionDeploymentOperate:
				return []string{"om-owner", "om-maintainer"}, nil
			case authz.ActionDeploymentDelete, authz.ActionDeploymentManageAccess:
				return []string{"om-owner"}, nil
			default:
				t.Fatalf("unexpected action %q", action)
				return nil, nil
			}
		},
	}

	server := &Server{deployStore: deploymentstore.NewStore(db), db: db}
	server.SetDeploymentAccessInspector(fga, memberships)
	response, err := server.GetDeploymentAccess(context.Background(), &adminv1.GetDeploymentAccessRequest{DeploymentId: "dep-1"})
	if err != nil {
		t.Fatalf("GetDeploymentAccess() error = %v", err)
	}
	if response.Status != "available" || len(response.Members) != 4 {
		t.Fatalf("GetDeploymentAccess() = %+v", response)
	}
	if got := []string{response.Members[0].UserID, response.Members[1].UserID, response.Members[2].UserID, response.Members[3].UserID}; !reflect.DeepEqual(got, []string{"user-owner", "user-maintainer", "user-viewer", "user-none"}) {
		t.Fatalf("member order = %v", got)
	}
	if got := response.Members[0].Sources; !reflect.DeepEqual(got, []string{"organization"}) {
		t.Fatalf("owner sources = %v", got)
	}
	if got := response.Members[1].Sources; !reflect.DeepEqual(got, []string{"group"}) {
		t.Fatalf("maintainer sources = %v", got)
	}
	if got := response.Members[2].Sources; !reflect.DeepEqual(got, []string{"direct"}) {
		t.Fatalf("viewer sources = %v", got)
	}
	if got := response.Members[3].Permissions; len(got) != 0 {
		t.Fatalf("unassigned permissions = %v", got)
	}
	if got := memberships.after; !reflect.DeepEqual(got, []string{"", "om-maintainer"}) {
		t.Fatalf("membership cursors = %v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetDeploymentAccessPersonalDoesNotCallWorkOS(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery(`SELECT`).WillReturnRows(deploymentRow("dep-1", "acct-1", "astro-abc-0"))
	mock.ExpectQuery(`SELECT a.type, COALESCE\(ao.workos_org_id`).
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"type", "workos_org_id"}).AddRow("personal", ""))

	server := &Server{deployStore: deploymentstore.NewStore(db), db: db}
	response, err := server.GetDeploymentAccess(context.Background(), &adminv1.GetDeploymentAccessRequest{DeploymentId: "dep-1"})
	if err != nil {
		t.Fatalf("GetDeploymentAccess() error = %v", err)
	}
	if response.Status != "personal" || len(response.Members) != 0 {
		t.Fatalf("GetDeploymentAccess() = %+v", response)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetDeploymentAccessReportsUnregisteredResource(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery(`SELECT`).WillReturnRows(deploymentRow("dep-1", "acct-1", "astro-abc-0"))
	mock.ExpectQuery(`SELECT a.type, COALESCE\(ao.workos_org_id`).
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"type", "workos_org_id"}).AddRow("organization", "org-1"))

	memberships := &fakeOrganizationMembershipLister{pages: map[string]org.MembershipPage{"": {}}}
	fga := &authz.FakeFGA{
		ListRoleAssignmentsFunc: func(context.Context, string, authz.ResourceRef) ([]authz.RoleAssignment, error) {
			return nil, authz.ErrResourceNotFound
		},
		ListMembershipsFunc: func(context.Context, string, authz.ResourceRef, authz.Action) ([]string, error) {
			return nil, authz.ErrResourceNotFound
		},
	}
	server := &Server{deployStore: deploymentstore.NewStore(db), db: db}
	server.SetDeploymentAccessInspector(fga, memberships)
	response, err := server.GetDeploymentAccess(context.Background(), &adminv1.GetDeploymentAccessRequest{DeploymentId: "dep-1"})
	if err != nil || response.Status != "not_registered" || len(response.Members) != 0 {
		t.Fatalf("GetDeploymentAccess() = (%+v, %v)", response, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
