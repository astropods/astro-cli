package admingrpc

import (
	"context"

	"github.com/astropods/astro/apps/astro-server/internal/authorizationadmin"
	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type authorizationAdminService interface {
	Inventory(context.Context) (*authorizationadmin.Inventory, error)
}

// SetAuthorizationAdmin wires Queen's read-only WorkOS resource inventory.
func (s *Server) SetAuthorizationAdmin(service *authorizationadmin.Service) {
	if service == nil {
		s.authorizationAdmin = nil
		return
	}
	s.authorizationAdmin = service
}

func (s *Server) ListAuthorizationResources(ctx context.Context, _ *adminv1.ListAuthorizationResourcesRequest) (*adminv1.ListAuthorizationResourcesResponse, error) {
	if s.authorizationAdmin == nil {
		return nil, status.Error(codes.FailedPrecondition, "authorization resource inventory is not configured")
	}
	inventory, err := s.authorizationAdmin.Inventory(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list authorization resources: %v", err)
	}
	resources := make([]*adminv1.AuthorizationResource, 0, len(inventory.Resources))
	for _, resource := range inventory.Resources {
		assignments := make([]*adminv1.AuthorizationAssignment, 0, len(resource.Assignments))
		for _, assignment := range resource.Assignments {
			assignments = append(assignments, &adminv1.AuthorizationAssignment{
				SubjectType: assignment.SubjectType, SubjectID: assignment.SubjectID,
				SubjectLabel: assignment.SubjectLabel, Role: assignment.Role, Source: assignment.Source,
			})
		}
		resources = append(resources, &adminv1.AuthorizationResource{
			Type:             resource.Type,
			Name:             resource.Name,
			ExternalID:       resource.ExternalID,
			WorkOSResourceID: resource.WorkOSResourceID,
			AccountID:        resource.AccountID,
			AccountName:      resource.AccountName,
			DirectAdmins:     resource.DirectAdmins,
			AssignmentCount:  int32(resource.AssignmentCount), //nolint:gosec // bounded by assignments returned for one resource
			CreatedAt:        resource.CreatedAt,
			SyncState:        resource.SyncState,
			LastError:        resource.LastError,
			Assignments:      assignments,
		})
	}
	return &adminv1.ListAuthorizationResourcesResponse{Resources: resources}, nil
}
