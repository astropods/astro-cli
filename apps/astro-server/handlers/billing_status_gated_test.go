package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

var billingRecordColumns = []string{
	"status", "reason", "dunning_since", "alert_active",
	"force_suspended", "credits_exhausted", "has_payment_method",
}

// gated is the server's verdict on whether a status is worth surfacing, and it
// is the only input the web client and the CLI branch on. Recomputing it in a
// client is how the banner and the 402 drift apart.
func TestBillingStatus_GatedFollowsEnforcementAndSuspendedWorkloads(t *testing.T) {
	cases := []struct {
		name       string
		status     billing.Status
		reason     string
		enforced   bool
		stopped    bool
		wantGated  bool
		wantAction string
	}{
		{
			name:   "enforced suspension gates and names the fix",
			status: billing.StatusSuspended, reason: billing.ReasonCreditsExhausted,
			enforced: true, wantGated: true, wantAction: "add_card",
		},
		{
			name:   "observe mode reports the status without gating",
			status: billing.StatusSuspended, reason: billing.ReasonCreditsExhausted,
			enforced: false, stopped: false, wantGated: false,
		},
		{
			name:   "workloads already stopped outlive enforcement being turned off",
			status: billing.StatusSuspended, reason: billing.ReasonCreditsExhausted,
			enforced: false, stopped: true, wantGated: true, wantAction: "add_card",
		},
		{
			name:   "an active account never gates, even enforced",
			status: billing.StatusActive, enforced: true, wantGated: false,
		},
		{
			name:   "past_due gates so the grace period is visible",
			status: billing.StatusPastDue, reason: billing.ReasonDunning,
			enforced: true, wantGated: true, wantAction: "update_card",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			statusDB, statusMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
			deployDB, deployMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
			acctID := uuid.New().String()

			var reason any
			if tc.reason != "" {
				reason = tc.reason
			}
			statusMock.ExpectQuery(`SELECT status, reason`).
				WillReturnRows(sqlmock.NewRows(billingRecordColumns).
					AddRow(string(tc.status), reason, nil, false, false, tc.reason == billing.ReasonCreditsExhausted, false))
			// Only a non-active status reads the workload state.
			if tc.status != billing.StatusActive {
				deployMock.ExpectQuery(`SELECT EXISTS`).
					WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(tc.stopped))
			}

			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(auth.AccountContextKey), &account.Account{ID: acctID, Name: "acme"})
				c.Next()
			})
			router.GET("/billing/status", GetBillingStatus(
				logger.New("error", "json"),
				billing.NewStatusStore(statusDB, 7),
				deploymentstore.NewStore(deployDB),
				tc.enforced,
			))

			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/billing/status", nil))
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
			}

			var resp BillingStatusResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if resp.Gated != tc.wantGated {
				t.Errorf("gated = %v, want %v", resp.Gated, tc.wantGated)
			}
			if resp.Action != tc.wantAction {
				t.Errorf("action = %q, want %q", resp.Action, tc.wantAction)
			}
		})
	}
}
