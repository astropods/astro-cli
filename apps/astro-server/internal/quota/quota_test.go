package quota

import (
	"context"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

func newChecker(t *testing.T, defaults map[string]int64, enforce bool) (*DBChecker, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewDBChecker(db, logger.New("error", "json"), defaults, enforce), mock
}

// expectNoOverride sets the account_limits lookup to return no rows (→ sql.ErrNoRows)
// so the config default applies.
func expectNoOverride(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("account_limits").WillReturnRows(sqlmock.NewRows([]string{"limit_value"}))
}

func TestCheck_UnlimitedDefault_NotBlocked(t *testing.T) {
	c, mock := newChecker(t, map[string]int64{ResourceAgents: Unlimited}, true)
	mock.ExpectQuery("account_limits").WillReturnRows(sqlmock.NewRows([]string{"limit_value"})) // no row → ErrNoRows

	res, err := c.Check(context.Background(), "acct-1", ResourceAgents)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Blocked {
		t.Errorf("unlimited should never block, got %+v", res)
	}
}

func TestCheck_DisabledLimitZero_AlwaysBlocks(t *testing.T) {
	// enforce=false must still block a disabled feature (limit 0).
	c, mock := newChecker(t, map[string]int64{ResourceAgents: 0}, false)
	mock.ExpectQuery("account_limits").WillReturnRows(sqlmock.NewRows([]string{"limit_value"}))

	res, err := c.Check(context.Background(), "acct-1", ResourceAgents)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Blocked || res.Resource != ResourceAgents || res.Limit != 0 {
		t.Errorf("expected blocked with limit 0, got %+v", res)
	}
}

func TestCheck_OverLimit_EnforceBlocks(t *testing.T) {
	c, mock := newChecker(t, map[string]int64{ResourceAgents: 5}, true)
	expectNoOverride(mock)
	mock.ExpectQuery("COUNT").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))

	res, err := c.Check(context.Background(), "acct-1", ResourceAgents)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Blocked || res.Used != 5 || res.Limit != 5 {
		t.Errorf("expected blocked at 5/5, got %+v", res)
	}
}

func TestCheck_OverLimit_NoEnforceAllows(t *testing.T) {
	c, mock := newChecker(t, map[string]int64{ResourceAgents: 5}, false)
	expectNoOverride(mock)
	mock.ExpectQuery("COUNT").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(9))

	res, err := c.Check(context.Background(), "acct-1", ResourceAgents)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Blocked {
		t.Errorf("over-limit with enforce=false should log-only, got %+v", res)
	}
}

func TestCheck_UnderLimit_NotBlocked(t *testing.T) {
	c, mock := newChecker(t, map[string]int64{ResourceMembers: 5}, true)
	expectNoOverride(mock)
	mock.ExpectQuery("COUNT").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	res, err := c.Check(context.Background(), "acct-1", ResourceMembers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Blocked {
		t.Errorf("under limit should not block, got %+v", res)
	}
}

func TestCheck_OverrideBeatsDefault(t *testing.T) {
	// Default is unlimited, but a per-account override caps it at 2.
	c, mock := newChecker(t, map[string]int64{ResourceAgents: Unlimited}, true)
	mock.ExpectQuery("account_limits").WithArgs("acct-1", ResourceAgents).
		WillReturnRows(sqlmock.NewRows([]string{"limit_value"}).AddRow(2))
	mock.ExpectQuery("COUNT").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	res, err := c.Check(context.Background(), "acct-1", ResourceAgents)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Blocked || res.Limit != 2 {
		t.Errorf("expected override limit 2 to block, got %+v", res)
	}
}

func TestLimitResponse_Codes(t *testing.T) {
	disabled := LimitResponse(Result{Blocked: true, Resource: ResourceAgents, Limit: 0})
	if disabled["code"] != "FEATURE_NOT_IN_PLAN" {
		t.Errorf("limit 0 → FEATURE_NOT_IN_PLAN, got %v", disabled["code"])
	}

	over := LimitResponse(Result{Blocked: true, Resource: ResourceAgents, Limit: 5, Used: 5})
	if over["code"] != "ENTITLEMENT_LIMIT_REACHED" {
		t.Errorf("over limit → ENTITLEMENT_LIMIT_REACHED, got %v", over["code"])
	}
	if over["usage"] != float64(5) || over["limit"] != float64(5) {
		t.Errorf("expected usage/limit 5/5, got %v/%v", over["usage"], over["limit"])
	}
}
