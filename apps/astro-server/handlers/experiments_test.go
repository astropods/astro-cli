package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/experiment"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

type fakeExperimentStore struct {
	enabled bool
	set     *bool
}

func (s *fakeExperimentStore) Enabled(context.Context, string, experiment.Key) (bool, error) {
	return s.enabled, nil
}

func (s *fakeExperimentStore) SetEnabled(_ context.Context, _ string, _ experiment.Key, enabled bool) error {
	s.set = &enabled
	return nil
}

type auditEventObserver chan auditlog.Event

func (o auditEventObserver) OnAudit(_ context.Context, event auditlog.Event) {
	o <- event
}

type recordingExperimentCache struct {
	accountID string
}

func (c *recordingExperimentCache) InvalidateAccount(accountID string) {
	c.accountID = accountID
}

func TestFineGrainedAccessExperimentHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)

	auditDB, auditMock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = auditDB.Close() })
	auditMock.ExpectExec("INSERT INTO audit_logs").
		WithArgs(
			"acct_123", "user_123", "user", auditlog.AccountUpdateExperiment,
			"account_experiment", string(experiment.FineGrainedAccess), nil,
			"Updated fine-grained access experiment", sqlmock.AnyArg(), sqlmock.AnyArg(), nil,
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	auditEvents := make(auditEventObserver, 1)
	auditStore := auditlog.NewStore(auditDB).Observe(auditEvents)

	store := &fakeExperimentStore{enabled: true}
	cache := &recordingExperimentCache{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.AccountContextKey), &account.Account{ID: "acct_123", Type: "organization"})
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user_123"})
		c.Next()
	})
	router.GET("/experiment", GetFineGrainedAccessExperiment(logger.New("error", "json"), store))
	router.PUT("/experiment", UpdateFineGrainedAccessExperiment(logger.New("error", "json"), store, auditStore, cache))

	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, httptest.NewRequest(http.MethodGet, "/experiment", nil))
	if getResponse.Code != http.StatusOK || !containsAll(getResponse.Body.String(), `"experiment":"fine_grained_access"`, `"enabled":true`) {
		t.Fatalf("GET status=%d body=%s", getResponse.Code, getResponse.Body.String())
	}

	putResponse := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/experiment", bytes.NewBufferString(`{"enabled":false}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(putResponse, request)
	if putResponse.Code != http.StatusOK || store.set == nil || *store.set {
		t.Fatalf("PUT status=%d set=%v body=%s", putResponse.Code, store.set, putResponse.Body.String())
	}
	if cache.accountID != "acct_123" {
		t.Fatalf("invalidated account = %q, want acct_123", cache.accountID)
	}

	select {
	case event := <-auditEvents:
		metadata, ok := event.Metadata.(map[string]any)
		if event.AccountID != "acct_123" || event.ActorID != "user_123" ||
			event.Action != auditlog.AccountUpdateExperiment ||
			event.ResourceType != "account_experiment" ||
			event.ResourceID != string(experiment.FineGrainedAccess) ||
			!ok || metadata["enabled"] != false {
			t.Fatalf("audit event = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for experiment audit event")
	}
	if err := auditMock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFineGrainedAccessExperimentRejectsPersonalAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.AccountContextKey), &account.Account{ID: "acct_123", Type: "personal"})
		c.Next()
	})
	router.GET("/experiment", GetFineGrainedAccessExperiment(logger.New("error", "json"), &fakeExperimentStore{}))

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/experiment", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}
