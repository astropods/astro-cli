package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
)

// blockingChecker always reports the account suspended, with a reason.
type blockingChecker struct{ reason string }

func (b blockingChecker) Check(context.Context, string) middleware.Decision {
	return middleware.Decision{Blocked: true, Reason: b.reason}
}

// A user who is not a member of the deployment's account must not learn that
// account's billing state. A 402 naming the gating reason tells any
// authenticated caller holding a deployment id whether some other tenant is
// suspended and why, which is a cross-tenant disclosure.
func TestWakeUpDeployment_DecidesMembershipBeforeBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	accountDB, accountMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	deployDB, deployMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "outsider"})
		c.Next()
	})
	router.POST("/api/v1/deployments/:id/wakeup", WakeUpDeployment(
		logger.New("error", "json"),
		account.NewAccountStore(accountDB),
		deploymentstore.NewStore(deployDB),
		&mockQueue{}, nil,
		blockingChecker{reason: "credits_exhausted"},
	))

	depID := deployid.New()
	rev := 1
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRowWithStatus(depID, uuid.New().String(), "victim-agent",
			"build-1", "astro-abc123", "Victim", `{}`, "stopped", &rev, time.Now()))
	// Not a member.
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/deployments/"+depID+"/wakeup", nil))

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: authorization must be decided before billing", w.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["reason"] != nil || body["code"] == "BILLING_SUSPENDED" {
		t.Errorf("response disclosed billing state to a non-member: %v", body)
	}
}
