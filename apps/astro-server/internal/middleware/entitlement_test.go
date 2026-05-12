package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/openmeter"
	"github.com/gin-gonic/gin"
)

func injectTestAccount(acct *account.Account) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(string(auth.AccountContextKey), acct)
		c.Next()
	}
}

func testAcct() *account.Account {
	return &account.Account{ID: "acct-test-1", Name: "testorg"}
}

func omServer(entitlements map[string]any) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"entitlements": entitlements})
	}))
}

func setupEntitlementRouter(omSrv *httptest.Server, enforce bool, features ...string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	log := logger.New("error", "json")
	ent := NewEntitlements(log, openmeter.NewClient(omSrv.URL), enforce)
	router := gin.New()
	router.Use(injectTestAccount(testAcct()))
	router.GET("/test", ent.Wrap(func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}, features...))
	return router
}

func get(router *gin.Engine) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/test", nil))
	return rec
}

// Feature present with HasAccess=true → allowed.
func TestEntitlement_HasAccess(t *testing.T) {
	srv := omServer(map[string]any{
		"knowledge_stores":  map[string]any{"hasAccess": true, "usage": 1, "totalAvailableGrantAmount": 5},
		"knowledge_storage": map[string]any{"hasAccess": true},
	})
	defer srv.Close()

	rec := get(setupEntitlementRouter(srv, true, "knowledge_stores", "knowledge_storage"))
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// Feature present with HasAccess=false and enforce=true → 402.
func TestEntitlement_QuotaExceeded_Enforced(t *testing.T) {
	srv := omServer(map[string]any{
		"knowledge_stores":  map[string]any{"hasAccess": false, "usage": 5, "totalAvailableGrantAmount": 5},
		"knowledge_storage": map[string]any{"hasAccess": true},
	})
	defer srv.Close()

	rec := get(setupEntitlementRouter(srv, true, "knowledge_stores", "knowledge_storage"))
	if rec.Code != http.StatusPaymentRequired {
		t.Errorf("expected 402, got %d: %s", rec.Code, rec.Body.String())
	}
}

// Feature present with HasAccess=false and enforce=false → allowed (log only).
func TestEntitlement_QuotaExceeded_NotEnforced(t *testing.T) {
	srv := omServer(map[string]any{
		"knowledge_stores":  map[string]any{"hasAccess": false, "usage": 5},
		"knowledge_storage": map[string]any{"hasAccess": true},
	})
	defer srv.Close()

	rec := get(setupEntitlementRouter(srv, false, "knowledge_stores", "knowledge_storage"))
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 (not enforcing overage), got %d: %s", rec.Code, rec.Body.String())
	}
}

// Feature absent from plan → always block, even when enforce=false.
func TestEntitlement_FeatureNotInPlan_AlwaysBlocked(t *testing.T) {
	srv := omServer(map[string]any{
		"agents": map[string]any{"hasAccess": true},
		// knowledge_stores and knowledge_storage not present
	})
	defer srv.Close()

	for _, enforce := range []bool{true, false} {
		rec := get(setupEntitlementRouter(srv, enforce, "knowledge_stores", "knowledge_storage"))
		if rec.Code != http.StatusPaymentRequired {
			t.Errorf("enforce=%v: expected 402 for feature not in plan, got %d: %s", enforce, rec.Code, rec.Body.String())
		}
	}
}

// OpenMeter API error → fail open regardless of enforce setting.
func TestEntitlement_APIError_FailOpen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	rec := get(setupEntitlementRouter(srv, true, "knowledge_stores"))
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 (fail open on API error), got %d: %s", rec.Code, rec.Body.String())
	}
}

// nil client → skip check entirely.
func TestEntitlement_NilClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := logger.New("error", "json")
	ent := NewEntitlements(log, nil, true)
	router := gin.New()
	router.Use(injectTestAccount(testAcct()))
	router.GET("/test", ent.Wrap(func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}, "knowledge_stores"))

	rec := get(router)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 (nil client), got %d: %s", rec.Code, rec.Body.String())
	}
}

// LimitResponse shape for a known feature with entitlement values.
func TestLimitResponse_KnownFeature(t *testing.T) {
	quota := 5.0
	usage := 3.0
	resp := LimitResponse("knowledge_stores", &openmeter.EntitlementValue{
		TotalAvailableGrantAmount: &quota,
		Usage:                     &usage,
	})
	if resp["code"] != "ENTITLEMENT_LIMIT_REACHED" {
		t.Errorf("unexpected code: %v", resp["code"])
	}
	if resp["feature"] != "knowledge_stores" {
		t.Errorf("unexpected feature: %v", resp["feature"])
	}
	if fmt.Sprintf("%v", resp["limit"]) != "5" {
		t.Errorf("unexpected limit: %v", resp["limit"])
	}
}

// LimitResponse with nil EntitlementValue (feature absent from plan) uses a distinct code and message.
func TestLimitResponse_NilEntitlement(t *testing.T) {
	resp := LimitResponse("knowledge_stores", nil)
	if resp["code"] != "FEATURE_NOT_IN_PLAN" {
		t.Errorf("unexpected code: %v", resp["code"])
	}
	if resp["usage"] != float64(0) || resp["limit"] != float64(0) {
		t.Errorf("expected zero usage/limit for nil entitlement, got usage=%v limit=%v", resp["usage"], resp["limit"])
	}
	details, _ := resp["details"].(string)
	if details == "" || !strings.Contains(details, "not included in your current plan") {
		t.Errorf("expected plan upgrade message, got: %q", details)
	}
}
