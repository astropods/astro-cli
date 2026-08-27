package authz_test

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"sync"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/authz"
)

const (
	contractAccountID      = "acct_contract"
	contractOrganizationID = "org_contract"
	contractDeploymentID   = "dep_contract"
)

func TestFGACContractAssignmentDiscoveryEnforcementAndRevocation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	resource := authz.DeploymentResource(contractDeploymentID)
	workos := newContractWorkOS()
	fga := workos.fga()
	groups := workos.groupsAPI()
	intents := newContractIntentStore()
	members := contractMembers()
	service := authz.NewAccessService(
		fga,
		enabledResourceGate,
		accountResolverFunc(func(context.Context, authz.ResourceRef) (string, bool, error) {
			return contractAccountID, false, nil
		}),
		organizationResolverFunc(func(context.Context, authz.ResourceRef) (string, bool, error) {
			return contractOrganizationID, false, nil
		}),
		members,
		groups,
		intents,
	)
	reconciler := authz.NewAccessReconciler(fga, intents)
	checker := authz.NewFGAChecker(
		fga,
		enabledResourceGate,
		organizationResolverFunc(func(context.Context, authz.ResourceRef) (string, bool, error) {
			return contractOrganizationID, false, nil
		}),
	)

	sohum := authz.Subject{UserID: "user_sohum", OrgID: contractOrganizationID, MembershipID: "om_sohum"}
	matt := authz.Subject{UserID: "user_matt", OrgID: contractOrganizationID, MembershipID: "om_matt"}
	if err := fga.AssignRole(ctx, authz.MembershipAssignmentSubject(sohum.MembershipID), authz.RoleDeploymentAdmin, resource); err != nil {
		t.Fatalf("seed creator access: %v", err)
	}
	assertContractDecision(t, checker, sohum, authz.ActionDeploymentManageAccess, resource, true)
	assertContractDecision(t, checker, matt, authz.ActionDeploymentRead, resource, false)
	assertContractVisibility(t, workos, members, matt, nil)

	group, err := groups.CreateGroup(ctx, contractOrganizationID, "Platform Engineering", "Deployment maintainers")
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	for _, membershipID := range []string{"om_matt", "om_saswat"} {
		if err := groups.AddGroupMember(ctx, contractOrganizationID, group.ID, membershipID); err != nil {
			t.Fatalf("add %s to group: %v", membershipID, err)
		}
	}
	groupIntent, changed, err := service.Assign(ctx, resource, authz.AssignmentSubjectGroup, group.ID, authz.AccessLevelMaintainer)
	if err != nil || !changed {
		t.Fatalf("record group maintainer intent: changed=%t error=%v", changed, err)
	}
	if synced, err := reconciler.ReconcileResource(ctx, contractOrganizationID, resource); err != nil || !synced {
		t.Fatalf("reconcile group maintainer: synced=%t error=%v", synced, err)
	}
	if current, ok := intents.get(groupIntent.Key()); !ok || current.Status() != authz.AccessSyncSynced {
		t.Fatalf("group intent did not converge: %+v, found=%t", current, ok)
	}

	assertContractVisibility(t, workos, members, matt, []string{contractDeploymentID})
	for _, action := range []authz.Action{
		authz.ActionDeploymentRead,
		authz.ActionDeploymentEdit,
		authz.ActionDeploymentOperate,
	} {
		assertContractDecision(t, checker, matt, action, resource, true)
	}
	assertContractDecision(t, checker, matt, authz.ActionDeploymentDelete, resource, false)
	assertContractDecision(t, checker, matt, authz.ActionDeploymentManageAccess, resource, false)
	assignments, recorded, err := service.ListAccess(ctx, resource)
	if err != nil {
		t.Fatalf("list effective group access: %v", err)
	}
	assertContractDerivedMaintainer(t, assignments, "user_matt")
	assertContractDerivedMaintainer(t, assignments, "user_saswat")
	if len(recorded) != 1 || recorded[0].Status() != authz.AccessSyncSynced {
		t.Fatalf("access intents = %+v, want one synced group intent", recorded)
	}

	if err := groups.RemoveGroupMember(ctx, contractOrganizationID, group.ID, matt.MembershipID); err != nil {
		t.Fatalf("remove Matt from group: %v", err)
	}
	assertContractDecision(t, checker, matt, authz.ActionDeploymentRead, resource, false)
	assertContractVisibility(t, workos, members, matt, nil)

	directIntent, changed, err := service.Assign(ctx, resource, authz.AssignmentSubjectMembership, matt.UserID, authz.AccessLevelViewer)
	if err != nil || !changed {
		t.Fatalf("record direct viewer intent: changed=%t error=%v", changed, err)
	}
	if synced, err := reconciler.ReconcileResource(ctx, contractOrganizationID, resource); err != nil || !synced {
		t.Fatalf("reconcile direct viewer: synced=%t error=%v", synced, err)
	}
	assertContractDecision(t, checker, matt, authz.ActionDeploymentRead, resource, true)
	assertContractDecision(t, checker, matt, authz.ActionDeploymentEdit, resource, false)
	assertContractVisibility(t, workos, members, matt, []string{contractDeploymentID})

	removedIntent, changed, err := service.Remove(ctx, resource, authz.AssignmentSubjectMembership, matt.UserID)
	if err != nil || !changed || removedIntent.DesiredVersion <= directIntent.DesiredVersion {
		t.Fatalf("record direct revocation: intent=%+v changed=%t error=%v", removedIntent, changed, err)
	}
	if synced, err := reconciler.ReconcileResource(ctx, contractOrganizationID, resource); err != nil || !synced {
		t.Fatalf("reconcile direct revocation: synced=%t error=%v", synced, err)
	}
	assertContractDecision(t, checker, matt, authz.ActionDeploymentRead, resource, false)
	assertContractVisibility(t, workos, members, matt, nil)
}

func assertContractDecision(
	t *testing.T,
	checker authz.Checker,
	subject authz.Subject,
	action authz.Action,
	resource authz.ResourceRef,
	want bool,
) {
	t.Helper()
	allowed, err := checker.Authorize(context.Background(), subject, action, resource)
	if err != nil || allowed != want {
		t.Fatalf("Authorize(%s, %s) = (%t, %v), want %t", subject.UserID, action, allowed, err, want)
	}
}

func assertContractVisibility(
	t *testing.T,
	workos *contractWorkOS,
	members fakeAccessMembers,
	subject authz.Subject,
	want []string,
) {
	t.Helper()
	discovery := authz.NewDeploymentDiscovery(
		contractDecisionLogger{},
		true,
		workos.fga(),
		experimentGateFunc(func(context.Context, string) (bool, error) { return true, nil }),
		discoveryMemberStoreFunc(func(ctx context.Context, accountID, userID string) (*account.AccountMember, error) {
			return members.GetMemberContext(ctx, accountID, userID)
		}),
	)
	visible, err := discovery.Visible(context.Background(), subject.UserID, []account.AccountWithRole{{
		ID: contractAccountID, Type: "organization", WorkOSOrganizationID: contractOrganizationID,
	}})
	if err != nil {
		t.Fatalf("discover visibility for %s: %v", subject.UserID, err)
	}
	if !visible.EnforcesAccount(contractAccountID) || !reflect.DeepEqual(visible.ReadableDeploymentIDs, want) {
		t.Fatalf("visibility for %s = %+v, want readable=%v", subject.UserID, visible, want)
	}
}

func assertContractDerivedMaintainer(t *testing.T, assignments []authz.AccessAssignment, userID string) {
	t.Helper()
	for _, assignment := range assignments {
		if assignment.UserID == userID && assignment.Level == authz.AccessLevelMaintainer && assignment.Source == authz.AssignmentSourceGroup {
			return
		}
	}
	t.Fatalf("missing group-derived maintainer assignment for %s: %+v", userID, assignments)
}

type contractDecisionLogger struct{}

func (contractDecisionLogger) Debug(string, ...any) {}
func (contractDecisionLogger) Info(string, ...any)  {}
func (contractDecisionLogger) Warn(string, ...any)  {}

type contractWorkOS struct {
	mu           sync.Mutex
	nextID       int
	assignments  []authz.RoleAssignment
	groups       map[string]authz.Group
	groupMembers map[string]map[string]struct{}
}

func newContractWorkOS() *contractWorkOS {
	return &contractWorkOS{
		groups:       make(map[string]authz.Group),
		groupMembers: make(map[string]map[string]struct{}),
	}
}

func (s *contractWorkOS) fga() *authz.FakeFGA {
	return &authz.FakeFGA{
		AssignRoleFunc:               s.assignRole,
		RemoveRoleFunc:               s.removeRole,
		ListRoleAssignmentsFunc:      s.listRoleAssignments,
		ListGroupRoleAssignmentsFunc: s.listGroupRoleAssignments,
		ListResourcesFunc:            s.listResources,
		CheckFunc:                    s.check,
		ListPermissionsFunc:          s.listPermissions,
	}
}

func (s *contractWorkOS) groupsAPI() *authz.FakeGroups {
	return &authz.FakeGroups{
		CreateGroupFunc: func(_ context.Context, organizationID, name, description string) (authz.Group, error) {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.nextID++
			group := authz.Group{ID: fmt.Sprintf("group_%d", s.nextID), OrganizationID: organizationID, Name: name, Description: description}
			s.groups[group.ID] = group
			s.groupMembers[group.ID] = make(map[string]struct{})
			return group, nil
		},
		GetGroupFunc: func(_ context.Context, organizationID, groupID string) (authz.Group, error) {
			s.mu.Lock()
			defer s.mu.Unlock()
			group, ok := s.groups[groupID]
			if !ok || group.OrganizationID != organizationID {
				return authz.Group{}, authz.ErrGroupNotFound
			}
			return group, nil
		},
		AddGroupMemberFunc: func(_ context.Context, organizationID, groupID, membershipID string) error {
			s.mu.Lock()
			defer s.mu.Unlock()
			group, ok := s.groups[groupID]
			if !ok || group.OrganizationID != organizationID {
				return authz.ErrGroupNotFound
			}
			s.groupMembers[groupID][membershipID] = struct{}{}
			return nil
		},
		RemoveGroupMemberFunc: func(_ context.Context, organizationID, groupID, membershipID string) error {
			s.mu.Lock()
			defer s.mu.Unlock()
			group, ok := s.groups[groupID]
			if !ok || group.OrganizationID != organizationID {
				return authz.ErrGroupNotFound
			}
			delete(s.groupMembers[groupID], membershipID)
			return nil
		},
	}
}

func (s *contractWorkOS) assignRole(_ context.Context, subject authz.AssignmentSubject, role authz.RoleSlug, resource authz.ResourceRef) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, assignment := range s.assignments {
		if assignment.Subject == subject && assignment.Role == role && assignment.Resource == resource {
			return authz.ErrRoleAssignmentExists
		}
	}
	s.nextID++
	s.assignments = append(s.assignments, authz.RoleAssignment{
		ID: fmt.Sprintf("assignment_%d", s.nextID), Subject: subject, Role: role,
		Source: authz.AssignmentSourceDirect, Resource: resource,
	})
	return nil
}

func (s *contractWorkOS) removeRole(_ context.Context, subject authz.AssignmentSubject, role authz.RoleSlug, resource authz.ResourceRef) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, assignment := range s.assignments {
		if assignment.Subject == subject && assignment.Role == role && assignment.Resource == resource {
			s.assignments = append(s.assignments[:i], s.assignments[i+1:]...)
			return nil
		}
	}
	return authz.ErrRoleAssignmentNotFound
}

func (s *contractWorkOS) listRoleAssignments(_ context.Context, organizationID string, resource authz.ResourceRef) ([]authz.RoleAssignment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if organizationID != contractOrganizationID {
		return nil, authz.ErrResourceNotFound
	}
	result := make([]authz.RoleAssignment, 0)
	for _, assignment := range s.assignments {
		if assignment.Resource != resource {
			continue
		}
		if assignment.Subject.Type == authz.AssignmentSubjectMembership {
			result = append(result, assignment)
			continue
		}
		for membershipID := range s.groupMembers[assignment.Subject.ID] {
			derived := assignment
			derived.ID += ":" + membershipID
			derived.Subject = authz.MembershipAssignmentSubject(membershipID)
			derived.Source = authz.AssignmentSourceGroup
			derived.GroupRoleAssignmentID = assignment.ID
			result = append(result, derived)
		}
	}
	return result, nil
}

func (s *contractWorkOS) listGroupRoleAssignments(_ context.Context, groupID string) ([]authz.RoleAssignment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.groups[groupID]; !ok {
		return nil, authz.ErrResourceNotFound
	}
	result := make([]authz.RoleAssignment, 0)
	for _, assignment := range s.assignments {
		if assignment.Subject == authz.GroupAssignmentSubject(groupID) {
			result = append(result, assignment)
		}
	}
	return result, nil
}

func (s *contractWorkOS) listResources(_ context.Context, membershipID string, action authz.Action, parent authz.ResourceRef) ([]authz.ResourceRef, error) {
	if parent != (authz.ResourceRef{Type: authz.ResourceOrganization, ExternalID: contractOrganizationID}) {
		return nil, authz.ErrResourceNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	resources := make(map[authz.ResourceRef]struct{})
	for _, assignment := range s.assignments {
		if s.assignmentApplies(assignment, membershipID) && contractRoleAllows(assignment.Role, action) {
			resources[assignment.Resource] = struct{}{}
		}
	}
	result := make([]authz.ResourceRef, 0, len(resources))
	for resource := range resources {
		result = append(result, resource)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ExternalID < result[j].ExternalID })
	return result, nil
}

func (s *contractWorkOS) check(_ context.Context, membershipID string, action authz.Action, resource authz.ResourceRef) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, assignment := range s.assignments {
		if assignment.Resource == resource && s.assignmentApplies(assignment, membershipID) && contractRoleAllows(assignment.Role, action) {
			return true, nil
		}
	}
	return false, nil
}

func (s *contractWorkOS) listPermissions(_ context.Context, membershipID string, resource authz.ResourceRef) ([]authz.Action, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	set := make(map[authz.Action]struct{})
	for _, assignment := range s.assignments {
		if assignment.Resource != resource || !s.assignmentApplies(assignment, membershipID) {
			continue
		}
		for _, role := range authz.ResourceRoles(resource.Type) {
			if role.Slug == assignment.Role {
				for _, action := range role.Actions {
					set[action] = struct{}{}
				}
			}
		}
	}
	result := make([]authz.Action, 0, len(set))
	for action := range set {
		result = append(result, action)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result, nil
}

func (s *contractWorkOS) assignmentApplies(assignment authz.RoleAssignment, membershipID string) bool {
	if assignment.Subject == authz.MembershipAssignmentSubject(membershipID) {
		return true
	}
	_, ok := s.groupMembers[assignment.Subject.ID][membershipID]
	return assignment.Subject.Type == authz.AssignmentSubjectGroup && ok
}

func contractRoleAllows(role authz.RoleSlug, action authz.Action) bool {
	for _, candidate := range authz.ResourceRoles(authz.ResourceDeployment) {
		if candidate.Slug != role {
			continue
		}
		for _, allowed := range candidate.Actions {
			if allowed == action {
				return true
			}
		}
	}
	return false
}

type contractIntentStore struct {
	mu      sync.Mutex
	intents map[authz.AccessIntentKey]authz.AccessIntent
}

func newContractIntentStore() *contractIntentStore {
	return &contractIntentStore{intents: make(map[authz.AccessIntentKey]authz.AccessIntent)}
}

func (s *contractIntentStore) Record(_ context.Context, intent authz.AccessIntent) (authz.AccessIntent, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, exists := s.intents[intent.Key()]
	if exists && current.DesiredRole == intent.DesiredRole {
		return current, false, nil
	}
	intent.DesiredVersion = current.DesiredVersion + 1
	intent.SyncedRole = current.SyncedRole
	intent.SyncedVersion = current.SyncedVersion
	s.intents[intent.Key()] = intent
	return intent, true, nil
}

func (s *contractIntentStore) ListForResource(_ context.Context, accountID string, resource authz.ResourceRef) ([]authz.AccessIntent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]authz.AccessIntent, 0)
	for _, intent := range s.intents {
		if intent.AccountID == accountID && intent.Resource == resource {
			result = append(result, intent)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Subject.ID < result[j].Subject.ID })
	return result, nil
}

func (s *contractIntentStore) PendingForResource(_ context.Context, organizationID string, resource authz.ResourceRef) ([]authz.AccessIntent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]authz.AccessIntent, 0)
	for _, intent := range s.intents {
		if intent.OrganizationID == organizationID && intent.Resource == resource && intent.DesiredVersion != intent.SyncedVersion {
			result = append(result, intent)
		}
	}
	return result, nil
}

func (s *contractIntentStore) MarkSynced(_ context.Context, intent authz.AccessIntent) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.intents[intent.Key()]
	if !ok || current.DesiredVersion != intent.DesiredVersion {
		return false, nil
	}
	current.SyncedRole = current.DesiredRole
	current.SyncedVersion = current.DesiredVersion
	s.intents[intent.Key()] = current
	return true, nil
}

func (s *contractIntentStore) Discard(_ context.Context, intent authz.AccessIntent) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.intents[intent.Key()]
	if !ok || current.DesiredVersion != intent.DesiredVersion {
		return false, nil
	}
	delete(s.intents, intent.Key())
	return true, nil
}

func (s *contractIntentStore) ResourceDeleted(context.Context, authz.ResourceRef) (bool, error) {
	return false, nil
}

func (s *contractIntentStore) RecordFailure(_ context.Context, intent authz.AccessIntent, cause error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.intents[intent.Key()]
	current.LastError = cause.Error()
	current.AttemptCount++
	s.intents[intent.Key()] = current
	return nil
}

func (s *contractIntentStore) get(key authz.AccessIntentKey) (authz.AccessIntent, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	intent, ok := s.intents[key]
	return intent, ok
}

func contractMembers() fakeAccessMembers {
	byUser := map[string]*account.AccountMember{
		"user_sohum":  {AccountID: contractAccountID, UserID: "user_sohum", WorkOSMembershipID: "om_sohum"},
		"user_matt":   {AccountID: contractAccountID, UserID: "user_matt", WorkOSMembershipID: "om_matt"},
		"user_saswat": {AccountID: contractAccountID, UserID: "user_saswat", WorkOSMembershipID: "om_saswat"},
	}
	return fakeAccessMembers{
		byUser: func(accountID, userID string) (*account.AccountMember, error) {
			member := byUser[userID]
			if member == nil || member.AccountID != accountID {
				return nil, fmt.Errorf("member %s not found", userID)
			}
			copy := *member
			return &copy, nil
		},
		byMemberships: func(ids []string) (map[string]*account.AccountMember, error) {
			result := make(map[string]*account.AccountMember)
			for _, member := range byUser {
				for _, id := range ids {
					if member.WorkOSMembershipID == id {
						copy := *member
						result[id] = &copy
					}
				}
			}
			return result, nil
		},
	}
}
