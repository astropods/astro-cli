package authz_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/authz"
)

type managedAccountStoreFunc func(context.Context, []string) ([]string, error)

func (f managedAccountStoreFunc) AccountsWithManagedDeployments(ctx context.Context, accountIDs []string) ([]string, error) {
	return f(ctx, accountIDs)
}

type discoveryMemberStore struct {
	members map[string]*account.AccountMember
}

func (s discoveryMemberStore) GetMember(accountID, userID string) (*account.AccountMember, error) {
	member, ok := s.members[accountID+"/"+userID]
	if !ok {
		return nil, errors.New("member not found")
	}
	return member, nil
}

func TestDeploymentVisibilityEnforcesUnsortedAccountIDs(t *testing.T) {
	t.Parallel()

	visibility := authz.DeploymentVisibility{FGAAccountIDs: []string{"acct_z", "acct_a", "acct_m"}}
	if !visibility.EnforcesAccount("acct_a") {
		t.Fatal("expected unsorted account ID to remain enforced")
	}
	if visibility.EnforcesAccount("acct_missing") {
		t.Fatal("unexpected enforcement for absent account ID")
	}
}

func TestDeploymentDiscoverySkipsManagedLookupWithoutWorkOSOrganizations(t *testing.T) {
	t.Parallel()

	managedCalls := 0
	discovery := authz.NewDeploymentDiscovery(
		true,
		&authz.FakeFGA{},
		experimentGateFunc(func(context.Context, string) (bool, error) {
			t.Fatal("experiment gate should not be called")
			return false, nil
		}),
		managedAccountStoreFunc(func(context.Context, []string) ([]string, error) {
			managedCalls++
			return nil, errors.New("managed lookup should not be called")
		}),
		discoveryMemberStore{},
	)

	visible, err := discovery.Visible(context.Background(), "user_123", []account.AccountWithRole{
		{ID: "acct_personal", Type: "personal"},
		{ID: "acct_without_org", Type: "organization"},
	})
	if err != nil || managedCalls != 0 || len(visible.FGAAccountIDs) != 0 || len(visible.ReadableDeploymentIDs) != 0 {
		t.Fatalf("Visible() = (%#v, %v), managed calls = %d", visible, err, managedCalls)
	}
}

func TestDeploymentDiscoveryListsOncePerManagedOptedInOrganization(t *testing.T) {
	t.Parallel()

	calls := 0
	fga := &authz.FakeFGA{ListResourcesFunc: func(_ context.Context, membershipID string, action authz.Action, parent authz.ResourceRef) ([]authz.ResourceRef, error) {
		calls++
		if membershipID != "om_123" || action != authz.ActionDeploymentRead || parent != (authz.ResourceRef{Type: authz.ResourceOrganization, ExternalID: "org_123"}) {
			t.Fatalf("discovery request = %q %q %#v", membershipID, action, parent)
		}
		return []authz.ResourceRef{
			authz.DeploymentResource("dep_2"),
			authz.DeploymentResource("dep_1"),
			authz.DeploymentResource("dep_1"),
			{Type: authz.ResourceOrganization, ExternalID: "org_123"},
		}, nil
	}}
	discovery := authz.NewDeploymentDiscovery(
		true,
		fga,
		experimentGateFunc(func(_ context.Context, accountID string) (bool, error) {
			return accountID == "acct_123", nil
		}),
		managedAccountStoreFunc(func(_ context.Context, accountIDs []string) ([]string, error) {
			if !reflect.DeepEqual(accountIDs, []string{"acct_123", "acct_legacy"}) {
				t.Fatalf("managed account request = %#v", accountIDs)
			}
			return []string{"acct_123"}, nil
		}),
		discoveryMemberStore{members: map[string]*account.AccountMember{
			"acct_123/user_123": {AccountID: "acct_123", UserID: "user_123", WorkOSMembershipID: "om_123"},
		}},
	)

	visible, err := discovery.Visible(context.Background(), "user_123", []account.AccountWithRole{
		{ID: "acct_123", Type: "organization", WorkOSOrganizationID: "org_123"},
		{ID: "acct_legacy", Type: "organization", WorkOSOrganizationID: "org_legacy"},
		{ID: "acct_personal", Type: "personal"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !reflect.DeepEqual(visible.FGAAccountIDs, []string{"acct_123"}) ||
		!reflect.DeepEqual(visible.ReadableDeploymentIDs, []string{"dep_1", "dep_2"}) {
		t.Fatalf("calls=%d visible=%#v", calls, visible)
	}
	if !visible.EnforcesAccount("acct_123") {
		t.Fatalf("visibility enforcement = %#v", visible)
	}
}

func TestDeploymentDiscoveryCachesPollingBurstsAndInvalidatesExperimentChanges(t *testing.T) {
	t.Parallel()

	enabled := true
	experimentCalls := 0
	discoveryCalls := 0
	fga := &authz.FakeFGA{ListResourcesFunc: func(context.Context, string, authz.Action, authz.ResourceRef) ([]authz.ResourceRef, error) {
		discoveryCalls++
		return []authz.ResourceRef{authz.DeploymentResource("dep_123")}, nil
	}}
	discovery := authz.NewDeploymentDiscovery(
		true,
		fga,
		experimentGateFunc(func(context.Context, string) (bool, error) {
			experimentCalls++
			return enabled, nil
		}),
		managedAccountStoreFunc(func(context.Context, []string) ([]string, error) {
			return []string{"acct_123"}, nil
		}),
		discoveryMemberStore{members: map[string]*account.AccountMember{
			"acct_123/user_123": {AccountID: "acct_123", UserID: "user_123", WorkOSMembershipID: "om_123"},
		}},
	)
	accounts := []account.AccountWithRole{{
		ID: "acct_123", Type: "organization", WorkOSOrganizationID: "org_123",
	}}

	for range 2 {
		visible, err := discovery.Visible(context.Background(), "user_123", accounts)
		if err != nil || !reflect.DeepEqual(visible.ReadableDeploymentIDs, []string{"dep_123"}) {
			t.Fatalf("Visible() = (%#v, %v)", visible, err)
		}
	}
	if discoveryCalls != 1 || experimentCalls != 2 {
		t.Fatalf("discovery calls = %d, experiment calls = %d", discoveryCalls, experimentCalls)
	}

	enabled = false
	discovery.InvalidateAccount("acct_123")
	visible, err := discovery.Visible(context.Background(), "user_123", accounts)
	if err != nil || visible.EnforcesAccount("acct_123") {
		t.Fatalf("disabled Visible() = (%#v, %v)", visible, err)
	}
	enabled = true
	discovery.InvalidateAccount("acct_123")
	_, err = discovery.Visible(context.Background(), "user_123", accounts)
	if err != nil {
		t.Fatal(err)
	}
	if discoveryCalls != 2 {
		t.Fatalf("discovery calls after experiment toggle = %d, want 2", discoveryCalls)
	}
}

func TestDeploymentDiscoveryFailsClosedOnlyForFailedOrganization(t *testing.T) {
	t.Parallel()

	fga := &authz.FakeFGA{ListResourcesFunc: func(_ context.Context, membershipID string, _ authz.Action, _ authz.ResourceRef) ([]authz.ResourceRef, error) {
		if membershipID == "om_failed" {
			return nil, errors.New("WorkOS unavailable")
		}
		return []authz.ResourceRef{authz.DeploymentResource("dep_healthy")}, nil
	}}
	discovery := authz.NewDeploymentDiscovery(
		true,
		fga,
		experimentGateFunc(func(context.Context, string) (bool, error) { return true, nil }),
		managedAccountStoreFunc(func(context.Context, []string) ([]string, error) {
			return []string{"acct_failed", "acct_healthy", "acct_missing"}, nil
		}),
		discoveryMemberStore{members: map[string]*account.AccountMember{
			"acct_failed/user_123":  {AccountID: "acct_failed", UserID: "user_123", WorkOSMembershipID: "om_failed"},
			"acct_healthy/user_123": {AccountID: "acct_healthy", UserID: "user_123", WorkOSMembershipID: "om_healthy"},
			"acct_missing/user_123": {AccountID: "acct_missing", UserID: "user_123"},
		}},
	)
	visible, err := discovery.Visible(context.Background(), "user_123", []account.AccountWithRole{
		{ID: "acct_failed", Type: "organization", WorkOSOrganizationID: "org_failed"},
		{ID: "acct_healthy", Type: "organization", WorkOSOrganizationID: "org_healthy"},
		{ID: "acct_missing", Type: "organization", WorkOSOrganizationID: "org_missing"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(visible.FGAAccountIDs, []string{"acct_failed", "acct_healthy", "acct_missing"}) ||
		!reflect.DeepEqual(visible.ReadableDeploymentIDs, []string{"dep_healthy"}) {
		t.Fatalf("Visible() = %#v", visible)
	}
}

func TestDeploymentDiscoveryInactiveDoesNoWork(t *testing.T) {
	t.Parallel()

	discovery := authz.NewDeploymentDiscovery(false, nil, nil, nil, nil)
	visible, err := discovery.Visible(context.Background(), "user_123", []account.AccountWithRole{{ID: "acct_123"}})
	if err != nil || len(visible.FGAAccountIDs) != 0 {
		t.Fatalf("Visible() = %#v, %v", visible, err)
	}
}
