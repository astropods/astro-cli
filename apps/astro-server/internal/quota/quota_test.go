package quota

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
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
	c, mock := newChecker(t, map[string]int64{ResourceBlueprints: Unlimited}, true)
	mock.ExpectQuery("account_limits").WillReturnRows(sqlmock.NewRows([]string{"limit_value"})) // no row → ErrNoRows

	res, err := c.Check(context.Background(), "acct-1", ResourceBlueprints)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Blocked {
		t.Errorf("unlimited should never block, got %+v", res)
	}
}

func TestCheck_DisabledLimitZero_AlwaysBlocks(t *testing.T) {
	// enforce=false must still block a disabled feature (limit 0).
	c, mock := newChecker(t, map[string]int64{ResourceBlueprints: 0}, false)
	mock.ExpectQuery("account_limits").WillReturnRows(sqlmock.NewRows([]string{"limit_value"}))

	res, err := c.Check(context.Background(), "acct-1", ResourceBlueprints)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Blocked || res.Resource != ResourceBlueprints || res.Limit != 0 {
		t.Errorf("expected blocked with limit 0, got %+v", res)
	}
}

func TestCheck_OverLimit_EnforceBlocks(t *testing.T) {
	c, mock := newChecker(t, map[string]int64{ResourceBlueprints: 5}, true)
	expectNoOverride(mock)
	mock.ExpectQuery("COUNT").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))

	res, err := c.Check(context.Background(), "acct-1", ResourceBlueprints)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Blocked || res.Used != 5 || res.Limit != 5 {
		t.Errorf("expected blocked at 5/5, got %+v", res)
	}
}

func TestCheck_OverLimit_NoEnforceAllows(t *testing.T) {
	c, mock := newChecker(t, map[string]int64{ResourceBlueprints: 5}, false)
	expectNoOverride(mock)
	mock.ExpectQuery("COUNT").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(9))

	res, err := c.Check(context.Background(), "acct-1", ResourceBlueprints)
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
	c, mock := newChecker(t, map[string]int64{ResourceBlueprints: Unlimited}, true)
	mock.ExpectQuery("account_limits").WithArgs("acct-1", ResourceBlueprints).
		WillReturnRows(sqlmock.NewRows([]string{"limit_value"}).AddRow(2))
	mock.ExpectQuery("COUNT").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	res, err := c.Check(context.Background(), "acct-1", ResourceBlueprints)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Blocked || res.Limit != 2 {
		t.Errorf("expected override limit 2 to block, got %+v", res)
	}
}

func TestBlueprintExists(t *testing.T) {
	for _, tc := range []struct {
		name string
		want bool
	}{{"present", true}, {"absent", false}} {
		t.Run(tc.name, func(t *testing.T) {
			c, mock := newChecker(t, nil, true)
			mock.ExpectQuery("SELECT EXISTS").WithArgs("acct-1", "bp").
				WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(tc.want))
			got, err := c.blueprintExists(context.Background(), "acct-1", "bp")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("exists = %v, want %v", got, tc.want)
			}
		})
	}
}

func setupRegisterRouter(c *DBChecker) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(ctx *gin.Context) {
		ctx.Set(string(auth.AccountContextKey), &account.Account{ID: "acct-1", Name: "testorg"})
		ctx.Next()
	})
	router.POST("/:account/agents/:name/register", c.WrapRegister(func(ctx *gin.Context) {
		ctx.JSON(http.StatusCreated, gin.H{"ok": true})
	}))
	return router
}

func doRegister(router *gin.Engine, name string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/testorg/agents/"+name+"/register", nil))
	return rec
}

// Re-pushing an existing (non-archived) blueprint must never be blocked by the
// blueprint-count limit, even when the account is over that cap: only the build
// limit applies.
func TestWrapRegister_ExistingBlueprint_SkipsAgentCap(t *testing.T) {
	c, mock := newChecker(t, map[string]int64{ResourceBlueprints: 5, ResourceAgentBuilds: 50}, true)
	mock.ExpectQuery("SELECT EXISTS").WithArgs("acct-1", "file-transfer-test").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	// Only agent_builds is evaluated (no agents COUNT), and it is under cap.
	expectNoOverride(mock)
	mock.ExpectQuery("COUNT").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	rec := doRegister(setupRegisterRouter(c), "file-transfer-test")
	if rec.Code != http.StatusCreated {
		t.Errorf("re-push of existing blueprint should not hit the blueprint cap, got %d: %s", rec.Code, rec.Body.String())
	}
}

// A push that creates a new blueprint is still gated by the blueprint-count
// limit, and the 402 speaks of blueprints with the usage/limit counts.
func TestWrapRegister_NewBlueprint_BlockedAtAgentCap(t *testing.T) {
	c, mock := newChecker(t, map[string]int64{ResourceBlueprints: 5, ResourceAgentBuilds: 50}, true)
	mock.ExpectQuery("SELECT EXISTS").WithArgs("acct-1", "brand-new").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	expectNoOverride(mock)
	mock.ExpectQuery("COUNT").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(36))

	rec := doRegister(setupRegisterRouter(c), "brand-new")
	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("new blueprint over the blueprint cap should be blocked, got %d: %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, "Blueprints limit reached (36 of 5 used)") {
		t.Errorf("expected blueprint wording with counts, got: %s", body)
	}
}

func TestLimitResponse_AgentsWording(t *testing.T) {
	resp := LimitResponse(Result{Blocked: true, Resource: ResourceBlueprints, Limit: 5, Used: 36})
	details, _ := resp["details"].(string)
	if !strings.Contains(details, "Blueprints limit reached (36 of 5 used)") {
		t.Errorf("expected blueprint wording with counts, got: %q", details)
	}
	if strings.Contains(strings.ToLower(details), "registered agents") {
		t.Errorf("details should say blueprints, not agents: %q", details)
	}
	if !strings.Contains(details, "archive") {
		t.Errorf("expected archiving remedy in details, got: %q", details)
	}
}

func TestLimitResponse_Codes(t *testing.T) {
	disabled := LimitResponse(Result{Blocked: true, Resource: ResourceBlueprints, Limit: 0})
	if disabled["code"] != "FEATURE_NOT_IN_PLAN" {
		t.Errorf("limit 0 → FEATURE_NOT_IN_PLAN, got %v", disabled["code"])
	}

	over := LimitResponse(Result{Blocked: true, Resource: ResourceBlueprints, Limit: 5, Used: 5})
	if over["code"] != "ENTITLEMENT_LIMIT_REACHED" {
		t.Errorf("over limit → ENTITLEMENT_LIMIT_REACHED, got %v", over["code"])
	}
	if over["usage"] != float64(5) || over["limit"] != float64(5) {
		t.Errorf("expected usage/limit 5/5, got %v/%v", over["usage"], over["limit"])
	}
}
