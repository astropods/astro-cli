package authz_test

import (
	"context"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/authz"
)

func TestSessionOrgMatchesAccount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		sub                authz.Subject
		accountType        string
		accountWorkOSOrgID string
		want               bool
	}{
		{
			name:               "personal account skips org scope",
			sub:                authz.Subject{UserID: "u1", OrgID: "org_other"},
			accountType:        "personal",
			accountWorkOSOrgID: "",
			want:               true,
		},
		{
			name:               "org account matching session org",
			sub:                authz.Subject{UserID: "u1", OrgID: "org_A"},
			accountType:        "organization",
			accountWorkOSOrgID: "org_A",
			want:               true,
		},
		{
			name:               "org account mismatched session org",
			sub:                authz.Subject{UserID: "u1", OrgID: "org_A"},
			accountType:        "organization",
			accountWorkOSOrgID: "org_B",
			want:               false,
		},
		{
			name:               "org account empty session org",
			sub:                authz.Subject{UserID: "u1"},
			accountType:        "organization",
			accountWorkOSOrgID: "org_A",
			want:               false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := authz.SessionOrgMatchesAccount(tt.sub, tt.accountType, tt.accountWorkOSOrgID)
			if got != tt.want {
				t.Fatalf("SessionOrgMatchesAccount() = %v, want %v", got, tt.want)
			}
		})
	}
}

// MembershipChecker does not apply org-scope rules today (PR1 scaffold). This test
// documents the gap: a member with JWT scoped to a different org is still allowed.
// It should fail once deployment middleware chains SessionOrgMatchesAccount before Authorize.
func TestMembershipChecker_allowsMemberDespiteOrgScopeMismatch(t *testing.T) {
	t.Parallel()

	const (
		resourceAccountID   = "acct-resource"
		resourceWorkOSOrgID = "org_resource"
	)

	checker := authz.NewMembershipChecker(
		fakeMemberChecker{
			isMember: func(accountID, userID string) (bool, error) {
				return accountID == resourceAccountID && userID == "user-1", nil
			},
		},
		fakeAccountResolver{
			accountForResource: func(authz.ResourceRef) (string, bool, error) {
				return resourceAccountID, false, nil
			},
		},
	)

	sub := authz.Subject{
		UserID: "user-1",
		OrgID:  "org_different_from_resource",
	}
	if authz.SessionOrgMatchesAccount(sub, "organization", resourceWorkOSOrgID) {
		t.Fatal("precondition: session org must not match resource account org for this test")
	}

	allowed, err := checker.Authorize(
		context.Background(),
		sub,
		authz.ActionDeploymentManage,
		authz.DeploymentResource("dep-1"),
	)
	if err != nil {
		t.Fatalf("Authorize() error: %v", err)
	}
	if !allowed {
		t.Fatal("Authorize() = false; PR1 MembershipChecker ignores org scope (expected until middleware wires SessionOrgMatchesAccount)")
	}
}

func TestMembershipChecker_personalFlagDoesNotChangeAuthorizeOutcome(t *testing.T) {
	t.Parallel()

	sub := authz.Subject{UserID: "user-1"}
	res := authz.DeploymentResource("dep-1")

	for _, personal := range []bool{true, false} {
		checker := authz.NewMembershipChecker(
			fakeMemberChecker{
				isMember: func(accountID, userID string) (bool, error) {
					return accountID == "acct-1" && userID == sub.UserID, nil
				},
			},
			fakeAccountResolver{
				accountForResource: func(authz.ResourceRef) (string, bool, error) {
					return "acct-1", personal, nil
				},
			},
		)

		allowed, err := checker.Authorize(context.Background(), sub, authz.ActionDeploymentManage, res)
		if err != nil {
			t.Fatalf("personal=%v: Authorize() error: %v", personal, err)
		}
		if !allowed {
			t.Fatalf("personal=%v: Authorize() = false, want true for member regardless of personal flag", personal)
		}
	}
}
