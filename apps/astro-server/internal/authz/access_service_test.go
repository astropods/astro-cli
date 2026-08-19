package authz_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/authz"
)

type fakeAccessMembers struct {
	byUser        func(string, string) (*account.AccountMember, error)
	byMemberships func([]string) (map[string]*account.AccountMember, error)
}

func (f fakeAccessMembers) GetMemberContext(_ context.Context, accountID, userID string) (*account.AccountMember, error) {
	return f.byUser(accountID, userID)
}

func (f fakeAccessMembers) GetMembersByWorkosMembershipIDsContext(_ context.Context, membershipIDs []string) (map[string]*account.AccountMember, error) {
	return f.byMemberships(membershipIDs)
}

type fakeAccessIntents struct {
	record func(authz.AccessIntent) (authz.AccessIntent, bool, error)
	list   func(string, authz.ResourceRef) ([]authz.AccessIntent, error)
}

func (f *fakeAccessIntents) Record(_ context.Context, intent authz.AccessIntent) (authz.AccessIntent, bool, error) {
	return f.record(intent)
}

func (f *fakeAccessIntents) ListForResource(_ context.Context, accountID string, resource authz.ResourceRef) ([]authz.AccessIntent, error) {
	if f.list == nil {
		return nil, nil
	}
	return f.list(accountID, resource)
}

func TestAccessServiceListsBuiltInDirectAndGroupDerivedAccessOnly(t *testing.T) {
	t.Parallel()

	resource := authz.DeploymentResource("dep_123")
	fga := &authz.FakeFGA{ListRoleAssignmentsFunc: func(_ context.Context, organizationID string, got authz.ResourceRef) ([]authz.RoleAssignment, error) {
		if organizationID != "org_123" || got != resource {
			t.Fatalf("list organization=%q resource=%+v", organizationID, got)
		}
		return []authz.RoleAssignment{
			{ID: "ra_2", Subject: authz.MembershipAssignmentSubject("om_2"), Role: authz.RoleDeploymentBuilder, Source: authz.AssignmentSourceGroup, GroupRoleAssignmentID: "gra_1", Resource: resource},
			{ID: "ra_1", Subject: authz.MembershipAssignmentSubject("om_1"), Role: authz.RoleDeploymentViewer, Source: authz.AssignmentSourceDirect, Resource: resource},
			{ID: "ra_custom", Subject: authz.MembershipAssignmentSubject("om_1"), Role: authz.RoleSlug("custom-support"), Source: authz.AssignmentSourceDirect, Resource: resource},
		}, nil
	}}
	service := newAccessService(t, fga, fakeAccessMembers{byMemberships: resolvedAccessMembers})

	got, err := service.List(context.Background(), resource)
	want := []authz.AccessAssignment{
		{ID: "ra_1", UserID: "user_om_1", Level: authz.AccessLevelViewer, Role: authz.RoleDeploymentViewer, Source: authz.AssignmentSourceDirect},
		{ID: "ra_2", UserID: "user_om_2", Level: authz.AccessLevelBuilder, Role: authz.RoleDeploymentBuilder, Source: authz.AssignmentSourceGroup, GroupRoleAssignmentID: "gra_1"},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("List() = (%#v, %v), want %#v", got, err, want)
	}
}

func TestAccessServiceListSkipsUnresolvableSubjects(t *testing.T) {
	t.Parallel()

	resource := authz.DeploymentResource("dep_123")
	fga := &authz.FakeFGA{ListRoleAssignmentsFunc: func(context.Context, string, authz.ResourceRef) ([]authz.RoleAssignment, error) {
		return []authz.RoleAssignment{
			{ID: "ra_group", Subject: authz.GroupAssignmentSubject("group_123"), Role: authz.RoleDeploymentViewer, Source: authz.AssignmentSourceDirect, Resource: resource},
			{ID: "ra_missing", Subject: authz.MembershipAssignmentSubject("om_missing"), Role: authz.RoleDeploymentViewer, Source: authz.AssignmentSourceDirect, Resource: resource},
			{ID: "ra_foreign", Subject: authz.MembershipAssignmentSubject("om_foreign"), Role: authz.RoleDeploymentViewer, Source: authz.AssignmentSourceDirect, Resource: resource},
			{ID: "ra_visible", Subject: authz.MembershipAssignmentSubject("om_visible"), Role: authz.RoleDeploymentBuilder, Source: authz.AssignmentSourceDirect, Resource: resource},
		}, nil
	}}
	service := newAccessService(t, fga, fakeAccessMembers{byMemberships: func([]string) (map[string]*account.AccountMember, error) {
		return map[string]*account.AccountMember{
			"om_foreign": {AccountID: "acct_other", UserID: "user_foreign", WorkOSMembershipID: "om_foreign"},
			"om_visible": {AccountID: "acct_123", UserID: "user_visible", WorkOSMembershipID: "om_visible"},
		}, nil
	}})

	got, err := service.List(context.Background(), resource)
	if err != nil || len(got) != 1 || got[0].UserID != "user_visible" {
		t.Fatalf("List() = (%#v, %v), want only resolved assignment", got, err)
	}
}

func TestAccessServiceListBatchesMembershipResolution(t *testing.T) {
	t.Parallel()

	resource := authz.DeploymentResource("dep_123")
	fga := &authz.FakeFGA{ListRoleAssignmentsFunc: func(context.Context, string, authz.ResourceRef) ([]authz.RoleAssignment, error) {
		return []authz.RoleAssignment{
			{ID: "ra_1", Subject: authz.MembershipAssignmentSubject("om_123"), Role: authz.RoleDeploymentViewer, Source: authz.AssignmentSourceDirect, Resource: resource},
			{ID: "ra_2", Subject: authz.MembershipAssignmentSubject("om_456"), Role: authz.RoleDeploymentBuilder, Source: authz.AssignmentSourceGroup, Resource: resource},
		}, nil
	}}
	lookups := 0
	service := newAccessService(t, fga, fakeAccessMembers{byMemberships: func(ids []string) (map[string]*account.AccountMember, error) {
		lookups++
		if !reflect.DeepEqual(ids, []string{"om_123", "om_456"}) {
			t.Fatalf("membership IDs = %v", ids)
		}
		return resolvedAccessMembers(ids)
	}})

	got, err := service.List(context.Background(), resource)
	if err != nil || len(got) != 2 || lookups != 1 {
		t.Fatalf("List() = (%#v, %v), lookups=%d", got, err, lookups)
	}
}

func TestAccessServiceListReturnsMemberLookupFailure(t *testing.T) {
	t.Parallel()

	lookupErr := errors.New("database unavailable")
	resource := authz.DeploymentResource("dep_123")
	fga := &authz.FakeFGA{ListRoleAssignmentsFunc: func(context.Context, string, authz.ResourceRef) ([]authz.RoleAssignment, error) {
		return []authz.RoleAssignment{{
			ID: "ra_1", Subject: authz.MembershipAssignmentSubject("om_1"), Role: authz.RoleDeploymentViewer,
			Source: authz.AssignmentSourceDirect, Resource: resource,
		}}, nil
	}}
	service := newAccessService(t, fga, fakeAccessMembers{byMemberships: func([]string) (map[string]*account.AccountMember, error) {
		return nil, lookupErr
	}})

	if _, err := service.List(context.Background(), resource); !errors.Is(err, lookupErr) {
		t.Fatalf("List() error = %v, want lookup failure", err)
	}
}

func TestAccessServiceRecordsDesiredStateWithoutCallingWorkOS(t *testing.T) {
	t.Parallel()

	resource := authz.DeploymentResource("dep_123")
	members := fakeAccessMembers{byUser: func(accountID, userID string) (*account.AccountMember, error) {
		return &account.AccountMember{AccountID: accountID, UserID: userID, WorkOSMembershipID: "om_123"}, nil
	}}
	for _, test := range []struct {
		name string
		role authz.RoleSlug
		call func(*authz.AccessService) (authz.AccessIntent, bool, error)
	}{
		{name: "assign", role: authz.RoleDeploymentBuilder, call: func(service *authz.AccessService) (authz.AccessIntent, bool, error) {
			return service.Assign(context.Background(), resource, authz.AssignmentSubjectMembership, "user_123", authz.AccessLevelBuilder)
		}},
		{name: "remove", call: func(service *authz.AccessService) (authz.AccessIntent, bool, error) {
			return service.Remove(context.Background(), resource, authz.AssignmentSubjectMembership, "user_123")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			intents := &fakeAccessIntents{record: func(intent authz.AccessIntent) (authz.AccessIntent, bool, error) {
				if intent.AccountID != "acct_123" || intent.OrganizationID != "org_123" || intent.Resource != resource ||
					intent.Subject != authz.MembershipAssignmentSubject("om_123") || intent.SubjectID != "user_123" || intent.DesiredRole != test.role {
					t.Fatalf("recorded intent = %+v", intent)
				}
				intent.DesiredVersion = 1
				return intent, true, nil
			}}
			service := newAccessServiceWithIntents(t, &authz.FakeFGA{}, members, intents)
			intent, changed, err := test.call(service)
			if err != nil || !changed || intent.Status() != authz.AccessSyncPending {
				t.Fatalf("mutation intent=%+v changed=%t error=%v", intent, changed, err)
			}
		})
	}
}

func TestAccessServiceMutationPreservesMemberLookupFailure(t *testing.T) {
	t.Parallel()

	lookupErr := errors.New("database unavailable")
	service := newAccessServiceWithIntents(t, &authz.FakeFGA{}, fakeAccessMembers{
		byUser: func(string, string) (*account.AccountMember, error) { return nil, lookupErr },
	}, &fakeAccessIntents{})
	_, _, err := service.Assign(context.Background(), authz.DeploymentResource("dep_123"), authz.AssignmentSubjectMembership, "user_123", authz.AccessLevelViewer)
	if errors.Is(err, authz.ErrResourceNotVisible) || !errors.Is(err, lookupErr) {
		t.Fatalf("Assign() error = %v, want lookup error", err)
	}
}

func TestAccessServiceDisabledDoesNotReachWorkOS(t *testing.T) {
	t.Parallel()

	service := authz.NewAccessService(
		&authz.FakeFGA{},
		resourceGateFunc(func(context.Context, authz.ResourceRef) (bool, error) { return false, nil }),
		accountResolverFunc(func(context.Context, authz.ResourceRef) (string, bool, error) { return "acct_123", false, nil }),
		organizationResolverFunc(func(context.Context, authz.ResourceRef) (string, bool, error) { return "org_123", false, nil }),
		fakeAccessMembers{},
		&fakeAccessIntents{},
	)
	_, err := service.List(context.Background(), authz.DeploymentResource("dep_123"))
	if !errors.Is(err, authz.ErrAccessManagementUnavailable) {
		t.Fatalf("List() error = %v, want ErrAccessManagementUnavailable", err)
	}
}

func newAccessService(t *testing.T, fga *authz.FakeFGA, members fakeAccessMembers) *authz.AccessService {
	t.Helper()
	return newAccessServiceWithIntents(t, fga, members, &fakeAccessIntents{})
}

func newAccessServiceWithIntents(t *testing.T, fga *authz.FakeFGA, members fakeAccessMembers, intents *fakeAccessIntents) *authz.AccessService {
	t.Helper()
	return authz.NewAccessService(
		fga,
		enabledResourceGate,
		accountResolverFunc(func(_ context.Context, resource authz.ResourceRef) (string, bool, error) {
			if resource != authz.DeploymentResource("dep_123") {
				t.Fatalf("account resource = %+v", resource)
			}
			return "acct_123", false, nil
		}),
		organizationResolverFunc(func(_ context.Context, resource authz.ResourceRef) (string, bool, error) {
			if resource != authz.DeploymentResource("dep_123") {
				t.Fatalf("organization resource = %+v", resource)
			}
			return "org_123", false, nil
		}),
		members,
		intents,
	)
}

func resolvedAccessMembers(ids []string) (map[string]*account.AccountMember, error) {
	members := make(map[string]*account.AccountMember, len(ids))
	for _, id := range ids {
		members[id] = &account.AccountMember{AccountID: "acct_123", UserID: "user_" + id, WorkOSMembershipID: id}
	}
	return members, nil
}
