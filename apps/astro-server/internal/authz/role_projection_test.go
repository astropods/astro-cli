package authz_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/authz"
)

type fakeReconcileQueue struct {
	keys []authz.AccessIntentKey
	err  error
}

func (q *fakeReconcileQueue) InsertResourceAccessFGAReconcileJob(_ context.Context, key authz.AccessIntentKey) error {
	q.keys = append(q.keys, key)
	return q.err
}

func provisionedMembers(membershipID string) fakeAccessMembers {
	return fakeAccessMembers{byUser: func(accountID, userID string) (*account.AccountMember, error) {
		return &account.AccountMember{AccountID: accountID, UserID: userID, WorkOSMembershipID: membershipID}, nil
	}}
}

func recordingIntents(changed bool) (*fakeAccessIntents, *[]authz.AccessIntent) {
	var recorded []authz.AccessIntent
	intents := &fakeAccessIntents{record: func(intent authz.AccessIntent) (authz.AccessIntent, bool, error) {
		recorded = append(recorded, intent)
		intent.DesiredVersion = 1
		return intent, changed, nil
	}}
	return intents, &recorded
}

func TestRoleProjectorGrantsCreatorAdmin(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		resource authz.ResourceRef
		want     authz.RoleSlug
	}{
		{"deployment", authz.DeploymentResource("dep_123"), authz.RoleDeploymentAdmin},
		{"blueprint", authz.BlueprintResource("bp_123"), authz.RoleBlueprintAdmin},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			intents, recorded := recordingIntents(true)
			queue := &fakeReconcileQueue{}
			projector := authz.NewRoleProjector(intents, provisionedMembers("om_123"), queue)

			if err := projector.GrantCreatorAdmin(context.Background(), "acct_123", "org_123", "user_1", tc.resource); err != nil {
				t.Fatalf("GrantCreatorAdmin() error = %v", err)
			}
			if len(*recorded) != 1 {
				t.Fatalf("recorded intents = %d", len(*recorded))
			}
			intent := (*recorded)[0]
			if intent.Resource != tc.resource || intent.DesiredRole != tc.want {
				t.Fatalf("intent = %+v, want %s on %+v", intent, tc.want, tc.resource)
			}
			if intent.AccountID != "acct_123" || intent.OrganizationID != "org_123" {
				t.Fatalf("intent scope = %q/%q", intent.AccountID, intent.OrganizationID)
			}
			if intent.Subject != authz.MembershipAssignmentSubject("om_123") || intent.SubjectID != "user_1" {
				t.Fatalf("intent subject = %+v/%q", intent.Subject, intent.SubjectID)
			}
			if len(queue.keys) != 1 || queue.keys[0].Resource != tc.resource {
				t.Fatalf("enqueued = %+v", queue.keys)
			}
		})
	}
}

func TestRoleProjectorGrantRejectsUnresolvedCreator(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		members fakeAccessMembers
	}{
		{"no membership mirror", provisionedMembers("")},
		{"no member row", fakeAccessMembers{byUser: func(string, string) (*account.AccountMember, error) {
			return nil, sql.ErrNoRows
		}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			intents, recorded := recordingIntents(true)
			projector := authz.NewRoleProjector(intents, tc.members, &fakeReconcileQueue{})

			err := projector.GrantCreatorAdmin(context.Background(), "acct_123", "org_123", "user_1", authz.DeploymentResource("dep_123"))
			if !errors.Is(err, authz.ErrAccessSubjectNotProvisioned) {
				t.Fatalf("GrantCreatorAdmin() error = %v", err)
			}
			if len(*recorded) != 0 {
				t.Fatalf("recorded intents = %+v", *recorded)
			}
		})
	}
}

func TestRoleProjectorGrantRejectsUnregisteredResourceType(t *testing.T) {
	t.Parallel()

	intents, recorded := recordingIntents(true)
	projector := authz.NewRoleProjector(intents, provisionedMembers("om_123"), &fakeReconcileQueue{})

	err := projector.GrantCreatorAdmin(context.Background(), "acct_123", "org_123", "user_1", authz.KnowledgeStoreResource("ks_123"))
	if err == nil {
		t.Fatal("GrantCreatorAdmin() error = nil")
	}
	if len(*recorded) != 0 {
		t.Fatalf("recorded intents = %+v", *recorded)
	}
}

func TestAccountRoleForOrganizationRole(t *testing.T) {
	t.Parallel()

	for organizationRole, want := range map[string]authz.RoleSlug{
		"owner":   authz.RoleAccountAdmin,
		"admin":   authz.RoleAccountAdmin,
		"member":  authz.RoleAccountMember,
		"billing": authz.RoleAccountMember,
		"":        authz.RoleAccountMember,
	} {
		if got := authz.AccountRoleForOrganizationRole(organizationRole); got != want {
			t.Fatalf("AccountRoleForOrganizationRole(%q) = %q, want %q", organizationRole, got, want)
		}
	}
}

func TestRoleProjectorProjectsAccountRole(t *testing.T) {
	t.Parallel()

	intents, recorded := recordingIntents(true)
	queue := &fakeReconcileQueue{}
	projector := authz.NewRoleProjector(intents, provisionedMembers("om_123"), queue)

	if err := projector.ProjectAccountRole(context.Background(), "acct_123", "org_123", "user_1", "om_123", "admin"); err != nil {
		t.Fatalf("ProjectAccountRole() error = %v", err)
	}
	intent := (*recorded)[0]
	if intent.Resource != authz.AccountResource("acct_123") || intent.DesiredRole != authz.RoleAccountAdmin {
		t.Fatalf("intent = %+v", intent)
	}
	if len(queue.keys) != 1 {
		t.Fatalf("enqueued = %+v", queue.keys)
	}
}

func TestRoleProjectorRevokesAccountRole(t *testing.T) {
	t.Parallel()

	intents, recorded := recordingIntents(true)
	projector := authz.NewRoleProjector(intents, provisionedMembers("om_123"), &fakeReconcileQueue{})

	if err := projector.RevokeAccountRole(context.Background(), "acct_123", "org_123", "user_1", "om_123"); err != nil {
		t.Fatalf("RevokeAccountRole() error = %v", err)
	}
	if intent := (*recorded)[0]; intent.DesiredRole != "" || intent.Resource != authz.AccountResource("acct_123") {
		t.Fatalf("intent = %+v", intent)
	}
}

func TestRoleProjectorRequiresMembershipIdentity(t *testing.T) {
	t.Parallel()

	intents, recorded := recordingIntents(true)
	projector := authz.NewRoleProjector(intents, provisionedMembers("om_123"), &fakeReconcileQueue{})

	err := projector.ProjectAccountRole(context.Background(), "acct_123", "org_123", "user_1", "", "member")
	if !errors.Is(err, authz.ErrAccessSubjectNotProvisioned) {
		t.Fatalf("ProjectAccountRole() error = %v", err)
	}
	if len(*recorded) != 0 {
		t.Fatalf("recorded intents = %+v", *recorded)
	}
}

func TestRoleProjectorSkipsReconciliationForUnchangedSyncedIntent(t *testing.T) {
	t.Parallel()

	intents := &fakeAccessIntents{record: func(intent authz.AccessIntent) (authz.AccessIntent, bool, error) {
		intent.DesiredVersion = 4
		intent.SyncedVersion = 4
		return intent, false, nil
	}}
	queue := &fakeReconcileQueue{}
	projector := authz.NewRoleProjector(intents, provisionedMembers("om_123"), queue)

	if err := projector.ProjectAccountRole(context.Background(), "acct_123", "org_123", "user_1", "om_123", "member"); err != nil {
		t.Fatalf("ProjectAccountRole() error = %v", err)
	}
	if len(queue.keys) != 0 {
		t.Fatalf("enqueued = %+v", queue.keys)
	}
}

func TestRoleProjectorEnqueueFailureKeepsIntentRecorded(t *testing.T) {
	t.Parallel()

	intents, recorded := recordingIntents(true)
	projector := authz.NewRoleProjector(intents, provisionedMembers("om_123"), &fakeReconcileQueue{err: errors.New("river down")})

	if err := projector.ProjectAccountRole(context.Background(), "acct_123", "org_123", "user_1", "om_123", "member"); err != nil {
		t.Fatalf("ProjectAccountRole() error = %v", err)
	}
	if len(*recorded) != 1 {
		t.Fatalf("recorded intents = %+v", *recorded)
	}
}

func TestNilRoleProjectorIsNoOp(t *testing.T) {
	t.Parallel()

	var projector *authz.RoleProjector
	if err := projector.GrantCreatorAdmin(context.Background(), "acct_123", "org_123", "user_1", authz.DeploymentResource("dep_123")); err != nil {
		t.Fatalf("GrantCreatorAdmin() error = %v", err)
	}
	if err := projector.ProjectAccountRole(context.Background(), "acct_123", "org_123", "user_1", "om_123", "admin"); err != nil {
		t.Fatalf("ProjectAccountRole() error = %v", err)
	}
	if err := projector.RevokeAccountRole(context.Background(), "acct_123", "org_123", "user_1", "om_123"); err != nil {
		t.Fatalf("RevokeAccountRole() error = %v", err)
	}
}
