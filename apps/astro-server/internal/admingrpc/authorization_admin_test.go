package admingrpc

import (
	"context"
	"testing"

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
	server.SetAuthorizationAdmin(nil)

	_, err := server.ListAuthorizationResources(context.Background(), &adminv1.ListAuthorizationResourcesRequest{})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("status = %v, want FailedPrecondition", status.Code(err))
	}
}

func (f *fakeAuthorizationAdminService) Inventory(context.Context) (*authorizationadmin.Inventory, error) {
	return f.inventory, nil
}

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
