package admingrpc

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/authorizationadmin"
	"github.com/astropods/astro/apps/astro-server/internal/riverqueue"
	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type authorizationAdminService interface {
	Inventory(context.Context) (*authorizationadmin.Inventory, error)
}

type authorizationAdminStore interface {
	CreateReset(context.Context, string, bool, *int) (*authorizationadmin.Operation, error)
	AttachJob(context.Context, string, int64) error
	List(context.Context, int) ([]authorizationadmin.Operation, error)
	Fail(context.Context, string, int, int, int, int, []authorizationadmin.ReportEntry, error) error
}

// SetAuthorizationAdmin wires Queen's read-only WorkOS resource inventory and
// the separately guarded reset workflow.
func (s *Server) SetAuthorizationAdmin(service *authorizationadmin.Service, store *authorizationadmin.Store, resetEnabled bool) {
	if service == nil {
		s.authorizationAdmin = nil
	} else {
		s.authorizationAdmin = service
	}
	s.authorizationAdminStore = store
	s.authorizationAdminResetEnabled = resetEnabled
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
	return &adminv1.ListAuthorizationResourcesResponse{
		Resources:    resources,
		ResetEnabled: s.authorizationAdminResetEnabled,
	}, nil
}

func (s *Server) ListAuthorizationOperations(ctx context.Context, req *adminv1.ListAuthorizationOperationsRequest) (*adminv1.ListAuthorizationOperationsResponse, error) {
	if s.authorizationAdminStore == nil {
		return nil, status.Error(codes.FailedPrecondition, "authorization operation store is not configured")
	}
	operations, err := s.authorizationAdminStore.List(ctx, int(req.Limit))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list authorization operations: %v", err)
	}
	result := make([]*adminv1.AuthorizationOperation, 0, len(operations))
	for i := range operations {
		result = append(result, authorizationOperationProto(&operations[i]))
	}
	return &adminv1.ListAuthorizationOperationsResponse{Operations: result}, nil
}

func (s *Server) StartAuthorizationResourceReset(ctx context.Context, req *adminv1.StartAuthorizationResourceResetRequest) (*adminv1.StartAuthorizationResourceResetResponse, error) {
	if !s.authorizationAdminResetEnabled {
		return nil, status.Error(codes.FailedPrecondition, "authorization resource reset is disabled")
	}
	if s.authorizationAdmin == nil || s.authorizationAdminStore == nil || s.queue == nil {
		return nil, status.Error(codes.FailedPrecondition, "authorization resource reset is not configured")
	}
	if req.AccountID == "" {
		return nil, status.Error(codes.InvalidArgument, "account_id is required")
	}
	var confirmedCount *int
	if !req.DryRun {
		if req.ConfirmedCount == nil {
			return nil, status.Error(codes.InvalidArgument, "confirmed_count is required")
		}
		count := int(*req.ConfirmedCount)
		confirmedCount = &count
	}
	operation, err := s.authorizationAdminStore.CreateReset(ctx, req.AccountID, req.DryRun, confirmedCount)
	if err != nil {
		return nil, authorizationAdminError(err)
	}
	args, err := json.Marshal(riverqueue.AuthorizationResourceResetArgs{OperationID: operation.ID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "encode authorization reset job: %v", err)
	}
	jobID, err := s.queue.TriggerJob(ctx, (riverqueue.AuthorizationResourceResetArgs{}).Kind(), args)
	if err != nil {
		_ = s.authorizationAdminStore.Fail(ctx, operation.ID, 0, 0, 0, 1, nil, err)
		return nil, status.Errorf(codes.Internal, "enqueue authorization reset: %v", err)
	}
	if err := s.authorizationAdminStore.AttachJob(ctx, operation.ID, jobID); err != nil {
		if s.log != nil {
			s.log.Warn("authorization reset: attach River job id failed",
				"operation_id", operation.ID,
				"job_id", jobID,
				"error", err,
			)
		}
	}
	return &adminv1.StartAuthorizationResourceResetResponse{Operation: authorizationOperationProto(operation)}, nil
}

func authorizationAdminError(err error) error {
	switch {
	case errors.Is(err, authorizationadmin.ErrOperationInProgress):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, authorizationadmin.ErrOperationNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, authorizationadmin.ErrAccountNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, authorizationadmin.ErrAccountNotLinked):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, authorizationadmin.ErrNotConfigured):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

func authorizationOperationProto(operation *authorizationadmin.Operation) *adminv1.AuthorizationOperation {
	if operation == nil {
		return nil
	}
	result := &adminv1.AuthorizationOperation{
		ID:             operation.ID,
		AccountID:      operation.AccountID,
		DryRun:         operation.DryRun,
		Status:         operation.Status,
		TargetCount:    int32(operation.TargetCount),    //nolint:gosec // bounded by the WorkOS resource count
		ProcessedCount: int32(operation.ProcessedCount), //nolint:gosec // bounded by target count
		SucceededCount: int32(operation.SucceededCount), //nolint:gosec // bounded by target count
		FailedCount:    int32(operation.FailedCount),    //nolint:gosec // bounded by target count
		LastError:      operation.LastError,
		CreatedAt:      operation.CreatedAt.Format(time.RFC3339),
	}
	return result
}
