package authz_test

import (
	"context"
	"errors"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/authz"
)

type fakeAccessReconciliationStore struct {
	intent          *authz.AccessIntent
	failures        []error
	resourceDeleted bool
	discarded       bool
}

func (s *fakeAccessReconciliationStore) ResourceDeleted(context.Context, authz.ResourceRef) (bool, error) {
	return s.resourceDeleted, nil
}

func (s *fakeAccessReconciliationStore) Discard(_ context.Context, intent authz.AccessIntent) (bool, error) {
	if s.intent == nil || s.intent.DesiredVersion != intent.DesiredVersion {
		return false, nil
	}
	s.discarded = true
	s.intent = nil
	return true, nil
}

func (s *fakeAccessReconciliationStore) PendingForResource(context.Context, string, authz.ResourceRef) ([]authz.AccessIntent, error) {
	if s.intent == nil || s.intent.SyncedVersion == s.intent.DesiredVersion {
		return nil, nil
	}
	return []authz.AccessIntent{*s.intent}, nil
}

func (s *fakeAccessReconciliationStore) MarkSynced(_ context.Context, intent authz.AccessIntent) (bool, error) {
	if s.intent.DesiredVersion != intent.DesiredVersion {
		return false, nil
	}
	s.intent.SyncedRole = intent.DesiredRole
	s.intent.SyncedVersion = intent.DesiredVersion
	return true, nil
}

func (s *fakeAccessReconciliationStore) RecordFailure(_ context.Context, _ authz.AccessIntent, cause error) error {
	s.failures = append(s.failures, cause)
	return nil
}

func TestAccessReconcilerReplacesRoleFromDesiredState(t *testing.T) {
	t.Parallel()

	resource := authz.DeploymentResource("dep_123")
	subject := authz.MembershipAssignmentSubject("om_123")
	store := &fakeAccessReconciliationStore{intent: &authz.AccessIntent{
		OrganizationID: "org_123", Resource: resource, Subject: subject,
		DesiredRole: authz.RoleDeploymentViewer, DesiredVersion: 2,
	}}
	var calls []string
	fga := &authz.FakeFGA{
		ListRoleAssignmentsFunc: func(context.Context, string, authz.ResourceRef) ([]authz.RoleAssignment, error) {
			return []authz.RoleAssignment{{Subject: subject, Role: authz.RoleDeploymentAdmin, Source: authz.AssignmentSourceDirect, Resource: resource}}, nil
		},
		RemoveRoleFunc: func(_ context.Context, _ authz.AssignmentSubject, role authz.RoleSlug, _ authz.ResourceRef) error {
			calls = append(calls, "remove:"+string(role))
			return nil
		},
		AssignRoleFunc: func(_ context.Context, _ authz.AssignmentSubject, role authz.RoleSlug, _ authz.ResourceRef) error {
			calls = append(calls, "assign:"+string(role))
			return nil
		},
	}

	synced, err := authz.NewAccessReconciler(fga, store).ReconcileResource(context.Background(), store.intent.OrganizationID, store.intent.Resource)
	if err != nil || !synced || len(calls) != 2 || calls[0] != "remove:deployment-admin" || calls[1] != "assign:deployment-viewer" {
		t.Fatalf("Reconcile() synced=%t error=%v calls=%v", synced, err, calls)
	}
	if store.intent.Status() != authz.AccessSyncSynced {
		t.Fatalf("intent status = %q", store.intent.Status())
	}
}

func TestAccessReconcilerRecoversFromPartialWorkOSFailure(t *testing.T) {
	t.Parallel()

	resource := authz.DeploymentResource("dep_123")
	subject := authz.MembershipAssignmentSubject("om_123")
	store := &fakeAccessReconciliationStore{intent: &authz.AccessIntent{
		OrganizationID: "org_123", Resource: resource, Subject: subject,
		DesiredRole: authz.RoleDeploymentViewer, DesiredVersion: 1,
	}}
	current := authz.RoleDeploymentBuilder
	assignAttempts := 0
	fga := &authz.FakeFGA{
		ListRoleAssignmentsFunc: func(context.Context, string, authz.ResourceRef) ([]authz.RoleAssignment, error) {
			if current == "" {
				return nil, nil
			}
			return []authz.RoleAssignment{{Subject: subject, Role: current, Source: authz.AssignmentSourceDirect, Resource: resource}}, nil
		},
		RemoveRoleFunc: func(context.Context, authz.AssignmentSubject, authz.RoleSlug, authz.ResourceRef) error {
			current = ""
			return nil
		},
		AssignRoleFunc: func(_ context.Context, _ authz.AssignmentSubject, role authz.RoleSlug, _ authz.ResourceRef) error {
			assignAttempts++
			if assignAttempts == 1 {
				return errors.New("WorkOS unavailable")
			}
			current = role
			return nil
		},
	}
	reconciler := authz.NewAccessReconciler(fga, store)

	if synced, err := reconciler.ReconcileResource(context.Background(), store.intent.OrganizationID, store.intent.Resource); err == nil || synced || len(store.failures) != 1 {
		t.Fatalf("first Reconcile() synced=%t error=%v failures=%d", synced, err, len(store.failures))
	}
	if current != "" || store.intent.SyncedVersion != 0 {
		t.Fatalf("partial state current=%q synced_version=%d", current, store.intent.SyncedVersion)
	}
	if synced, err := reconciler.ReconcileResource(context.Background(), store.intent.OrganizationID, store.intent.Resource); err != nil || !synced || current != authz.RoleDeploymentViewer {
		t.Fatalf("retry Reconcile() synced=%t error=%v current=%q", synced, err, current)
	}
}

func TestAccessReconcilerRemovalKeepsDerivedAndCustomRoles(t *testing.T) {
	t.Parallel()

	resource := authz.DeploymentResource("dep_123")
	subject := authz.MembershipAssignmentSubject("om_123")
	store := &fakeAccessReconciliationStore{intent: &authz.AccessIntent{
		OrganizationID: "org_123", Resource: resource, Subject: subject, DesiredVersion: 1,
	}}
	var removed []authz.RoleSlug
	fga := &authz.FakeFGA{
		ListRoleAssignmentsFunc: func(context.Context, string, authz.ResourceRef) ([]authz.RoleAssignment, error) {
			return []authz.RoleAssignment{
				{Subject: subject, Role: authz.RoleDeploymentBuilder, Source: authz.AssignmentSourceDirect, Resource: resource},
				{Subject: subject, Role: authz.RoleDeploymentViewer, Source: authz.AssignmentSourceGroup, Resource: resource},
				{Subject: subject, Role: authz.RoleSlug("custom-support"), Source: authz.AssignmentSourceDirect, Resource: resource},
			}, nil
		},
		RemoveRoleFunc: func(_ context.Context, _ authz.AssignmentSubject, role authz.RoleSlug, _ authz.ResourceRef) error {
			removed = append(removed, role)
			return nil
		},
	}

	synced, err := authz.NewAccessReconciler(fga, store).ReconcileResource(context.Background(), store.intent.OrganizationID, store.intent.Resource)
	if err != nil || !synced || len(removed) != 1 || removed[0] != authz.RoleDeploymentBuilder {
		t.Fatalf("Reconcile() synced=%t error=%v removed=%v", synced, err, removed)
	}
}

func TestAccessReconcilerDiscardsIntentWhenResourceIsGone(t *testing.T) {
	t.Parallel()

	store := &fakeAccessReconciliationStore{resourceDeleted: true, intent: &authz.AccessIntent{
		OrganizationID: "org_123", Resource: authz.DeploymentResource("dep_123"),
		Subject: authz.MembershipAssignmentSubject("om_123"), DesiredRole: authz.RoleDeploymentViewer, DesiredVersion: 1,
	}}
	fga := &authz.FakeFGA{ListRoleAssignmentsFunc: func(context.Context, string, authz.ResourceRef) ([]authz.RoleAssignment, error) {
		return nil, authz.ErrResourceNotFound
	}}

	synced, err := authz.NewAccessReconciler(fga, store).ReconcileResource(context.Background(), store.intent.OrganizationID, store.intent.Resource)
	if err != nil || !synced || !store.discarded || len(store.failures) != 0 {
		t.Fatalf("Reconcile() synced=%t error=%v discarded=%t failures=%v", synced, err, store.discarded, store.failures)
	}
}

func TestAccessReconcilerRetriesWhenWorkOSResourceIsNotYetProvisioned(t *testing.T) {
	t.Parallel()

	store := &fakeAccessReconciliationStore{intent: &authz.AccessIntent{
		OrganizationID: "org_123", Resource: authz.DeploymentResource("dep_123"),
		Subject: authz.MembershipAssignmentSubject("om_123"), DesiredRole: authz.RoleDeploymentViewer, DesiredVersion: 1,
	}}
	fga := &authz.FakeFGA{ListRoleAssignmentsFunc: func(context.Context, string, authz.ResourceRef) ([]authz.RoleAssignment, error) {
		return nil, authz.ErrResourceNotFound
	}}

	synced, err := authz.NewAccessReconciler(fga, store).ReconcileResource(context.Background(), store.intent.OrganizationID, store.intent.Resource)
	if !errors.Is(err, authz.ErrResourceNotFound) || synced || store.discarded || len(store.failures) != 1 {
		t.Fatalf("Reconcile() synced=%t error=%v discarded=%t failures=%v", synced, err, store.discarded, store.failures)
	}
}

func TestAccessReconcilerDiscardsIntentWhenGroupIsGone(t *testing.T) {
	t.Parallel()

	store := &fakeAccessReconciliationStore{intent: &authz.AccessIntent{
		OrganizationID: "org_123", Resource: authz.DeploymentResource("dep_123"),
		Subject: authz.GroupAssignmentSubject("group_123"), DesiredRole: authz.RoleDeploymentViewer, DesiredVersion: 1,
	}}
	fga := &authz.FakeFGA{ListGroupRoleAssignmentsFunc: func(context.Context, string) ([]authz.RoleAssignment, error) {
		return nil, authz.ErrResourceNotFound
	}}

	synced, err := authz.NewAccessReconciler(fga, store).ReconcileResource(context.Background(), store.intent.OrganizationID, store.intent.Resource)
	if err != nil || !synced || !store.discarded || len(store.failures) != 0 {
		t.Fatalf("ReconcileResource() synced=%t error=%v discarded=%t failures=%v", synced, err, store.discarded, store.failures)
	}
}
