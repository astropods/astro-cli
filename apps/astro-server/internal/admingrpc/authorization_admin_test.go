package admingrpc

import (
	"context"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/authorizationadmin"
	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeAuthorizationAdminService struct {
	inventory *authorizationadmin.Inventory
}

func TestAuthorizationInventoryIsNotConfiguredForNilService(t *testing.T) {
	server := &Server{}
	server.SetAuthorizationAdmin(nil, nil, false)

	_, err := server.ListAuthorizationResources(context.Background(), &adminv1.ListAuthorizationResourcesRequest{})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("status = %v, want FailedPrecondition", status.Code(err))
	}
}

func (f *fakeAuthorizationAdminService) Inventory(context.Context) (*authorizationadmin.Inventory, error) {
	return f.inventory, nil
}

type fakeAuthorizationAdminStore struct {
	operation *authorizationadmin.Operation
	jobID     int64
}

func (f *fakeAuthorizationAdminStore) CreateReset(_ context.Context, accountID string, dryRun bool, confirmedCount *int) (*authorizationadmin.Operation, error) {
	f.operation = &authorizationadmin.Operation{ID: "op_123", AccountID: accountID, DryRun: dryRun, ConfirmedCount: confirmedCount, Status: "queued", CreatedAt: time.Now()}
	return f.operation, nil
}
func (f *fakeAuthorizationAdminStore) AttachJob(_ context.Context, _ string, jobID int64) error {
	f.jobID = jobID
	return nil
}
func (f *fakeAuthorizationAdminStore) Get(context.Context, string) (*authorizationadmin.Operation, error) {
	return f.operation, nil
}
func (f *fakeAuthorizationAdminStore) List(context.Context, int) ([]authorizationadmin.Operation, error) {
	return nil, nil
}
func (*fakeAuthorizationAdminStore) Fail(context.Context, string, int, int, int, int, []authorizationadmin.ReportEntry, error) error {
	return nil
}
func (*fakeAuthorizationAdminStore) ReleaseMaintenance(context.Context, string) error { return nil }

func TestAuthorizationInventoryReusesDeploymentInspectorLinkData(t *testing.T) {
	server := &Server{
		authorizationAdmin: &fakeAuthorizationAdminService{inventory: &authorizationadmin.Inventory{
			Resources: []authorizationadmin.Resource{{
				Type: "deployment", ExternalID: "dep_123", WorkOSResourceID: "resource_123",
				Assignments: []authorizationadmin.Assignment{{
					SubjectType: "group", SubjectID: "group_123", SubjectLabel: "Platform Engineering",
					Role: "deployment-viewer", Source: "direct",
				}},
			}},
		}},
	}
	response, err := server.ListAuthorizationResources(context.Background(), &adminv1.ListAuthorizationResourcesRequest{})
	if err != nil || len(response.Resources) != 1 || response.Resources[0].ExternalID != "dep_123" ||
		len(response.Resources[0].Assignments) != 1 || response.Resources[0].Assignments[0].SubjectLabel != "Platform Engineering" {
		t.Fatalf("ListAuthorizationResources() = (%+v, %v)", response, err)
	}
}

func TestAuthorizationResetIsServerDisabledByDefault(t *testing.T) {
	server := &Server{}
	_, err := server.StartAuthorizationResourceReset(context.Background(), &adminv1.StartAuthorizationResourceResetRequest{DryRun: true})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("status = %v, want FailedPrecondition", status.Code(err))
	}
}

func TestAuthorizationResetRequiresTypedCount(t *testing.T) {
	server := &Server{
		authorizationAdminResetEnabled: true,
		authorizationAdmin:             &fakeAuthorizationAdminService{},
		authorizationAdminStore:        &fakeAuthorizationAdminStore{},
		queue:                          &mockAdminJobQueue{},
	}
	_, err := server.StartAuthorizationResourceReset(context.Background(), &adminv1.StartAuthorizationResourceResetRequest{AccountID: "acct_123"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("status = %v, want InvalidArgument", status.Code(err))
	}
}

func TestAuthorizationResetRequiresAccount(t *testing.T) {
	server := &Server{
		authorizationAdminResetEnabled: true,
		authorizationAdmin:             &fakeAuthorizationAdminService{},
		authorizationAdminStore:        &fakeAuthorizationAdminStore{},
		queue:                          &mockAdminJobQueue{},
	}
	_, err := server.StartAuthorizationResourceReset(context.Background(), &adminv1.StartAuthorizationResourceResetRequest{DryRun: true})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("status = %v, want InvalidArgument", status.Code(err))
	}
}
