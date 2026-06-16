package authz_test

import (
	"context"
	"errors"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/authz"
)

var _ authz.Checker = (*authz.MembershipChecker)(nil)

type fakeMemberChecker struct {
	isMember func(accountID, userID string) (bool, error)
}

func (f fakeMemberChecker) IsMember(accountID, userID string) (bool, error) {
	return f.isMember(accountID, userID)
}

type fakeAccountResolver struct {
	accountForResource func(res authz.ResourceRef) (accountID string, personal bool, err error)
}

func (f fakeAccountResolver) AccountForResource(res authz.ResourceRef) (string, bool, error) {
	return f.accountForResource(res)
}

func TestMembershipChecker_Authorize(t *testing.T) {
	t.Parallel()

	res := authz.DeploymentResource("dep-123")
	sub := authz.Subject{UserID: "user-1"}
	resolverErr := errors.New("lookup failed")

	tests := []struct {
		name      string
		personal  bool
		isMember  bool
		memberErr error
		resErr    error
		want      bool
		wantErr   error
	}{
		{
			name:     "personal account member allows",
			personal: true,
			isMember: true,
			want:     true,
		},
		{
			name:     "personal account non-member denies",
			personal: true,
			isMember: false,
			want:     false,
		},
		{
			name:     "org account member allows",
			personal: false,
			isMember: true,
			want:     true,
		},
		{
			name:     "org account non-member denies",
			personal: false,
			isMember: false,
			want:     false,
		},
		{
			name:    "resolver error propagates",
			resErr:  resolverErr,
			wantErr: resolverErr,
		},
		{
			name:      "membership lookup error propagates",
			personal:  true,
			memberErr: errors.New("db down"),
			wantErr:   errors.New("db down"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			checker := authz.NewMembershipChecker(
				fakeMemberChecker{
					isMember: func(accountID, userID string) (bool, error) {
						if tt.memberErr != nil {
							return false, tt.memberErr
						}
						if accountID != "acct-1" || userID != sub.UserID {
							t.Fatalf("IsMember(%q, %q) unexpected args", accountID, userID)
						}
						return tt.isMember, nil
					},
				},
				fakeAccountResolver{
					accountForResource: func(got authz.ResourceRef) (string, bool, error) {
						if got != res {
							t.Fatalf("AccountForResource(%+v) unexpected resource", got)
						}
						if tt.resErr != nil {
							return "", false, tt.resErr
						}
						return "acct-1", tt.personal, nil
					},
				},
			)

			got, err := checker.Authorize(context.Background(), sub, authz.ActionDeploymentManage, res)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) && err.Error() != tt.wantErr.Error() {
					t.Fatalf("Authorize() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Authorize() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("Authorize() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMembershipChecker_resolverErrorSkipsIsMember(t *testing.T) {
	t.Parallel()

	resolverErr := errors.New("deployment not found")
	checker := authz.NewMembershipChecker(
		fakeMemberChecker{
			isMember: func(accountID, userID string) (bool, error) {
				t.Fatal("IsMember must not be called when resolver fails")
				return false, nil
			},
		},
		fakeAccountResolver{
			accountForResource: func(authz.ResourceRef) (string, bool, error) {
				return "", false, resolverErr
			},
		},
	)

	got, err := checker.Authorize(
		context.Background(),
		authz.Subject{UserID: "user-1"},
		authz.ActionDeploymentManage,
		authz.DeploymentResource("missing"),
	)
	if !errors.Is(err, resolverErr) {
		t.Fatalf("Authorize() error = %v, want %v", err, resolverErr)
	}
	if got {
		t.Fatalf("Authorize() = true, want false")
	}
}

func TestMembershipChecker_crossAccountMembershipDenied(t *testing.T) {
	t.Parallel()

	memberships := map[string]map[string]bool{
		"acct-a": {"user-1": true},
		"acct-b": {},
	}

	checker := authz.NewMembershipChecker(
		fakeMemberChecker{
			isMember: func(accountID, userID string) (bool, error) {
				byUser, ok := memberships[accountID]
				if !ok {
					return false, nil
				}
				return byUser[userID], nil
			},
		},
		fakeAccountResolver{
			accountForResource: func(res authz.ResourceRef) (string, bool, error) {
				if res.ExternalID == "dep-on-a" {
					return "acct-a", false, nil
				}
				return "acct-b", false, nil
			},
		},
	)

	sub := authz.Subject{UserID: "user-1"}

	allowedOnA, err := checker.Authorize(
		context.Background(), sub, authz.ActionDeploymentManage, authz.DeploymentResource("dep-on-a"),
	)
	if err != nil {
		t.Fatalf("Authorize(dep-on-a) error: %v", err)
	}
	if !allowedOnA {
		t.Fatal("Authorize(dep-on-a) = false, want true (member of owning account)")
	}

	allowedOnB, err := checker.Authorize(
		context.Background(), sub, authz.ActionDeploymentManage, authz.DeploymentResource("dep-on-b"),
	)
	if err != nil {
		t.Fatalf("Authorize(dep-on-b) error: %v", err)
	}
	if allowedOnB {
		t.Fatal("Authorize(dep-on-b) = true, want false (member of different account only)")
	}
}

func TestMembershipChecker_emptyUserIDDenied(t *testing.T) {
	t.Parallel()

	checker := authz.NewMembershipChecker(
		fakeMemberChecker{
			isMember: func(accountID, userID string) (bool, error) {
				if userID != "" {
					t.Fatalf("IsMember userID = %q, want empty string", userID)
				}
				return false, nil
			},
		},
		fakeAccountResolver{
			accountForResource: func(authz.ResourceRef) (string, bool, error) {
				return "acct-1", true, nil
			},
		},
	)

	got, err := checker.Authorize(
		context.Background(),
		authz.Subject{},
		authz.ActionDeploymentManage,
		authz.DeploymentResource("dep-1"),
	)
	if err != nil {
		t.Fatalf("Authorize() error: %v", err)
	}
	if got {
		t.Fatal("Authorize() = true, want false for empty UserID")
	}
}
