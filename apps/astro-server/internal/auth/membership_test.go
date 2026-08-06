package auth

import (
	"errors"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/account"
)

type fakeMembershipIDResolver struct {
	orgID       string
	account     *account.Account
	member      *account.AccountMember
	orgErr      error
	memberErr   error
	orgCalls    int
	memberCalls int
}

func (f *fakeMembershipIDResolver) GetByWorkOSOrganizationID(orgID string) (*account.Account, error) {
	f.orgCalls++
	if f.orgErr != nil {
		return nil, f.orgErr
	}
	if orgID != f.orgID {
		return nil, errors.New("org not found")
	}
	return f.account, nil
}

func (f *fakeMembershipIDResolver) GetMember(accountID, userID string) (*account.AccountMember, error) {
	f.memberCalls++
	if f.memberErr != nil {
		return nil, f.memberErr
	}
	if f.member == nil || f.member.AccountID != accountID || f.member.UserID != userID {
		return nil, errors.New("member not found")
	}
	return f.member, nil
}

func TestResolveWorkOSMembershipID_JWTClaimWins(t *testing.T) {
	t.Parallel()

	resolver := &fakeMembershipIDResolver{
		orgID:   "org_A",
		account: &account.Account{ID: "acct-1"},
		member: &account.AccountMember{
			AccountID:          "acct-1",
			UserID:             "user-1",
			WorkOSMembershipID: "om_from_db",
		},
	}

	got, err := ResolveWorkOSMembershipID(resolver, "user-1", "org_A", "om_from_jwt")
	if err != nil {
		t.Fatalf("ResolveWorkOSMembershipID() error = %v", err)
	}
	if got != "om_from_jwt" {
		t.Fatalf("ResolveWorkOSMembershipID() = %q, want om_from_jwt", got)
	}
	if resolver.orgCalls != 0 {
		t.Fatalf("GetByWorkOSOrganizationID calls = %d, want 0 when JWT claim present", resolver.orgCalls)
	}
}

func TestResolveWorkOSMembershipID_DBFallback(t *testing.T) {
	t.Parallel()

	resolver := &fakeMembershipIDResolver{
		orgID:   "org_A",
		account: &account.Account{ID: "acct-1"},
		member: &account.AccountMember{
			AccountID:          "acct-1",
			UserID:             "user-1",
			WorkOSMembershipID: "om_from_db",
		},
	}

	got, err := ResolveWorkOSMembershipID(resolver, "user-1", "org_A", "")
	if err != nil {
		t.Fatalf("ResolveWorkOSMembershipID() error = %v", err)
	}
	if got != "om_from_db" {
		t.Fatalf("ResolveWorkOSMembershipID() = %q, want om_from_db", got)
	}
	if resolver.orgCalls != 1 || resolver.memberCalls != 1 {
		t.Fatalf("orgCalls=%d memberCalls=%d, want 1 each", resolver.orgCalls, resolver.memberCalls)
	}
}

func TestResolveWorkOSMembershipID_EmptyForNoOrgScope(t *testing.T) {
	t.Parallel()

	resolver := &fakeMembershipIDResolver{
		orgID:   "org_A",
		account: &account.Account{ID: "acct-1"},
		member: &account.AccountMember{
			AccountID:          "acct-1",
			UserID:             "user-1",
			WorkOSMembershipID: "om_from_db",
		},
	}

	got, err := ResolveWorkOSMembershipID(resolver, "user-1", "", "")
	if err != nil {
		t.Fatalf("ResolveWorkOSMembershipID() error = %v", err)
	}
	if got != "" {
		t.Fatalf("ResolveWorkOSMembershipID() = %q, want empty", got)
	}
	if resolver.orgCalls != 0 {
		t.Fatalf("GetByWorkOSOrganizationID calls = %d, want 0 for empty org scope", resolver.orgCalls)
	}
}

func TestResolveWorkOSMembershipID_NilResolver(t *testing.T) {
	t.Parallel()

	got, err := ResolveWorkOSMembershipID(nil, "user-1", "org_A", "")
	if got != "" {
		t.Fatalf("ResolveWorkOSMembershipID() = %q, want empty", got)
	}
	if err == nil {
		t.Fatal("ResolveWorkOSMembershipID() error = nil, want resolver error")
	}
}

func TestResolveWorkOSMembershipID_DBFallbackEmptyWhenMemberHasNoWorkOSLink(t *testing.T) {
	t.Parallel()

	resolver := &fakeMembershipIDResolver{
		orgID:   "org_A",
		account: &account.Account{ID: "acct-1"},
		member: &account.AccountMember{
			AccountID: "acct-1",
			UserID:    "user-1",
		},
	}

	got, err := ResolveWorkOSMembershipID(resolver, "user-1", "org_A", "")
	if got != "" {
		t.Fatalf("ResolveWorkOSMembershipID() = %q, want empty when member has no workos link", got)
	}
	if !errors.Is(err, ErrWorkOSMembershipIDNotFound) {
		t.Fatalf("ResolveWorkOSMembershipID() error = %v, want ErrWorkOSMembershipIDNotFound", err)
	}
}

func TestResolveWorkOSMembershipID_DBFallbackEmptyWhenOrgLookupFails(t *testing.T) {
	t.Parallel()

	resolver := &fakeMembershipIDResolver{orgErr: errors.New("org not linked")}

	got, err := ResolveWorkOSMembershipID(resolver, "user-1", "org_A", "")
	if got != "" {
		t.Fatalf("ResolveWorkOSMembershipID() = %q, want empty when org lookup fails", got)
	}
	if err == nil {
		t.Fatal("ResolveWorkOSMembershipID() error = nil, want org lookup error")
	}
	if resolver.orgCalls != 1 {
		t.Fatalf("orgCalls = %d, want 1", resolver.orgCalls)
	}
	if resolver.memberCalls != 0 {
		t.Fatalf("memberCalls = %d, want 0 when org lookup fails", resolver.memberCalls)
	}
}
