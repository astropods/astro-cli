package authz

import (
	"context"
	"errors"
	"fmt"
)

type accessReconciliationStore interface {
	PendingForResource(context.Context, string, ResourceRef) ([]AccessIntent, error)
	ResourceDeleted(context.Context, ResourceRef) (bool, error)
	MarkSynced(context.Context, AccessIntent) (bool, error)
	Discard(context.Context, AccessIntent) (bool, error)
	RecordFailure(context.Context, AccessIntent, error) error
}

// AccessReconciler moves one direct built-in role toward its durable desired state.
type AccessReconciler struct {
	assignments AccessAssignments
	store       accessReconciliationStore
}

func NewAccessReconciler(assignments AccessAssignments, store accessReconciliationStore) *AccessReconciler {
	return &AccessReconciler{assignments: assignments, store: store}
}

// ReconcileResource converges all pending direct built-in roles on one resource.
// A false, nil result means intents changed repeatedly while reconciliation ran.
func (r *AccessReconciler) ReconcileResource(ctx context.Context, organizationID string, resource ResourceRef) (bool, error) {
	if r == nil || r.assignments == nil || r.store == nil {
		return false, errors.New("resource access reconciler is not configured")
	}
	for range 3 {
		intents, err := r.store.PendingForResource(ctx, organizationID, resource)
		if err != nil {
			return false, err
		}
		if len(intents) == 0 {
			return true, nil
		}
		stable, err := r.reconcileIntents(ctx, intents)
		if err != nil || stable {
			return stable, err
		}
	}
	return false, nil
}

func (r *AccessReconciler) reconcileIntents(ctx context.Context, intents []AccessIntent) (bool, error) {
	var membershipAssignments []RoleAssignment
	var membershipErr error
	for _, intent := range intents {
		if intent.Subject.Type != AssignmentSubjectGroup {
			membershipAssignments, membershipErr = r.assignments.ListRoleAssignments(ctx, intent.OrganizationID, intent.Resource)
			break
		}
	}

	stable := true
	var result error
	for _, intent := range intents {
		assignments, err := membershipAssignments, membershipErr
		if intent.Subject.Type == AssignmentSubjectGroup {
			assignments, err = r.assignmentsForGroup(ctx, intent)
		}
		synced, reconcileErr := r.reconcileIntent(ctx, intent, assignments, err)
		stable = stable && synced
		result = errors.Join(result, reconcileErr)
	}
	return stable, result
}

func (r *AccessReconciler) reconcileIntent(ctx context.Context, intent AccessIntent, assignments []RoleAssignment, err error) (bool, error) {
	if err != nil {
		if errors.Is(err, ErrResourceNotFound) {
			if intent.Subject.Type == AssignmentSubjectGroup {
				return r.store.Discard(ctx, intent)
			}
			deleted, deletionErr := r.store.ResourceDeleted(ctx, intent.Resource)
			if deletionErr != nil {
				return r.fail(ctx, intent, fmt.Errorf("confirm resource deletion: %w", deletionErr))
			}
			if deleted {
				return r.store.Discard(ctx, intent)
			}
		}
		return r.fail(ctx, intent, fmt.Errorf("list current resource access: %w", err))
	}

	existing := directBuiltInAssignments(assignments, intent.Subject, intent.Resource)
	hasDesired := false
	for _, assignment := range existing {
		if intent.DesiredRole != "" && assignment.Role == intent.DesiredRole {
			hasDesired = true
			continue
		}
		if err := r.assignments.RemoveRole(ctx, intent.Subject, assignment.Role, intent.Resource); err != nil && !errors.Is(err, ErrRoleAssignmentNotFound) {
			return r.fail(ctx, intent, fmt.Errorf("remove stale resource access: %w", err))
		}
	}
	if intent.DesiredRole != "" && !hasDesired {
		if err := r.assignments.AssignRole(ctx, intent.Subject, intent.DesiredRole, intent.Resource); err != nil && !errors.Is(err, ErrRoleAssignmentExists) {
			return r.fail(ctx, intent, fmt.Errorf("assign desired resource access: %w", err))
		}
	}

	synced, err := r.store.MarkSynced(ctx, intent)
	if err != nil {
		return false, err
	}
	return synced, nil
}

func (r *AccessReconciler) assignmentsForGroup(ctx context.Context, intent AccessIntent) ([]RoleAssignment, error) {
	assignments, err := r.assignments.ListGroupRoleAssignments(ctx, intent.Subject.ID)
	if err != nil {
		return nil, err
	}
	result := make([]RoleAssignment, 0, len(assignments))
	for _, assignment := range assignments {
		if assignment.Resource == intent.Resource {
			result = append(result, assignment)
		}
	}
	return result, nil
}

func (r *AccessReconciler) fail(ctx context.Context, intent AccessIntent, cause error) (bool, error) {
	if err := r.store.RecordFailure(ctx, intent, cause); err != nil {
		return false, errors.Join(cause, err)
	}
	return false, cause
}
