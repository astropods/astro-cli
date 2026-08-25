package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// thresholdRouter mounts both threshold writers with no provider. Every case
// here is decided before the provider is reached, so a rejected value answers
// 400 and an accepted one falls through to the no-provider 200.
func thresholdRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(string(auth.AccountContextKey), &account.Account{ID: "acct-1", Name: "acme"})
		c.Next()
	})
	log := logger.New("error", "json")
	r.PUT("/billing/spend/thresholds", SetBillingSpendThresholds(log, nil, nil, "metronome", nil, nil))
	r.PUT("/billing/usage/thresholds", SetBillingUsageThresholds(log, nil, nil, "metronome", nil, nil))
	return r
}

func putThreshold(t *testing.T, r *gin.Engine, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

// A limit of 1e308 is finite, so the negative check passes it. The provider
// stores it and the settings page renders it, but no spend ever reaches it, so
// the account is uncapped while showing a cap.
func TestSetThresholds_AbsurdLimitIsRefused(t *testing.T) {
	r := thresholdRouter(t)
	for _, path := range []string{"/billing/spend/thresholds", "/billing/usage/thresholds"} {
		body := `{"limit":1e308}`
		if strings.Contains(path, "usage") {
			body = `{"metric":"gateway","limit":1e308}`
		}
		rec := putThreshold(t, r, path, body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400: %s", path, rec.Code, rec.Body.String())
		}
	}
}

// The warning is written from the same request, so bounding only the limit
// leaves an alert that never fires. The limit is null here because a limit above
// the warning would be refused by its own bound first.
func TestSetThresholds_AbsurdWarningIsRefused(t *testing.T) {
	r := thresholdRouter(t)
	rec := putThreshold(t, r, "/billing/spend/thresholds", `{"warning":1e308,"limit":null}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	// A malformed body is also a 400, so the status alone would prove nothing.
	var got struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.Contains(got.Error, "cannot exceed") {
		t.Errorf("error = %q, want the bound to be what refused it", got.Error)
	}
}

// The bound is a typo guard, so the largest legitimate value has to survive it.
func TestSetThresholds_TheBoundItselfIsAllowed(t *testing.T) {
	r := thresholdRouter(t)
	body := fmt.Sprintf(`{"limit":%.0f}`, float64(maxThresholdAmount))
	rec := putThreshold(t, r, "/billing/spend/thresholds", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

// Clearing a control sends null, which must not be read as a value to bound.
func TestSetThresholds_NullStillClears(t *testing.T) {
	r := thresholdRouter(t)
	rec := putThreshold(t, r, "/billing/spend/thresholds", `{"warning":null,"limit":null}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}
