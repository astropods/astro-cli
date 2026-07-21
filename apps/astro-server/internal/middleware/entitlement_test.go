package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// With no status store wired (OSS/noop), the gate is a pass-through even in
// enforce mode. Balance-driven blocking is exercised by the state-machine tests
// in internal/billing (computeStatus) — the gate here only reads the cached row.

func TestEntitlement_WrapPassesThroughWhenNoStore(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ent := NewEntitlements(nil, true, nil)
	router := gin.New()
	router.GET("/test", ent.Wrap(func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/test", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestEntitlement_AllowsWhenNoStore(t *testing.T) {
	ent := NewEntitlements(nil, true, nil)
	if ent.Blocked(context.Background(), "acct-1") {
		t.Error("expected not blocked with no status store")
	}
}

func TestPaymentRequiredResponse_Shape(t *testing.T) {
	resp := PaymentRequiredResponse()
	if resp["code"] != "BILLING_SUSPENDED" {
		t.Errorf("unexpected code: %v", resp["code"])
	}
}
