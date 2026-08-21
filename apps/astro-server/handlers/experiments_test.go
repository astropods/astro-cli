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
		c.Set(string(auth.AccountContextKey), &account.Account{
			ID: "acct_123", Type: "organization", WorkOSOrganizationID: "org_1",
		})
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user_123"})
		c.Set(string(auth.SessionContextKey), &auth.Session{
			OrganizationID: "org_1", Permissions: []string{"org:manage", "org:admin"},
		})
		c.Next()
	})
	router.GET("/experiment/:experiment", GetAccountExperiment(logger.New("error", "json"), store, nil))
	router.PUT("/experiment/:experiment", UpdateAccountExperiment(logger.New("error", "json"), store, auditStore, cache, nil))

	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, httptest.NewRequest(http.MethodGet, "/experiment/fine-grained-access", nil))
	if getResponse.Code != http.StatusOK || !containsAll(getResponse.Body.String(), `"experiment":"fine_grained_access"`, `"enabled":true`) {
		t.Fatalf("GET status=%d body=%s", getResponse.Code, getResponse.Body.String())
	}

	putResponse := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/experiment/fine-grained-access", bytes.NewBufferString(`{"enabled":false}`))
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
	router.GET("/experiment/:experiment", GetAccountExperiment(logger.New("error", "json"), &fakeExperimentStore{}, nil))

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/experiment/fine-grained-access", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

// The slug is the wire name; an unknown one must not fall through to some
// default experiment.
func TestGetAccountExperiment_UnknownSlugIs404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.AccountContextKey), &account.Account{ID: "acct-1", Type: "organization"})
		c.Next()
	})
	router.GET("/experiment/:experiment", GetAccountExperiment(logger.New("error", "json"), &fakeExperimentStore{}, nil))

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/experiment/not-a-real-experiment", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.Code)
	}
}

// Every registered slug must name a real key, or the switch writes a row
// nothing reads.
func TestExperimentRegistryKeysAreDeclared(t *testing.T) {
	known := map[experiment.Key]bool{
		experiment.FineGrainedAccess:         true,
		experiment.PromptClassificationStats: true,
	}
	for slug, def := range experimentsBySlug {
		if !known[def.key] {
			t.Errorf("slug %q maps to undeclared key %q", slug, def.key)
		}
		if def.label == "" {
			t.Errorf("slug %q has no label for audit entries", slug)
		}
	}
}

// Classification runs off a personal account's own telemetry, so unlike
// fine-grained access it must be settable there — org-only would lock a solo
// developer out of the page entirely.
func TestPromptClassificationExperimentAllowsPersonalAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.AccountContextKey), &account.Account{ID: "acct_123", Type: "personal"})
		c.Next()
	})
	router.GET("/experiment/:experiment", GetAccountExperiment(logger.New("error", "json"), &fakeExperimentStore{}, nil))

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/experiment/prompt-classification-stats", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
}

// The route group gates on org:manage, which both owners and admins carry.
// Fine-grained access governs deployment privacy and stayed owner-only, so the
// extra permission is enforced per experiment rather than per route.
func TestExperimentPermissionIsPerExperiment(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := map[string]struct {
		slug        string
		permissions []string
		want        int
	}{
		"admin may not toggle fine-grained access": {
			"fine-grained-access", []string{"org:manage"}, http.StatusForbidden,
		},
		"owner may toggle fine-grained access": {
			"fine-grained-access", []string{"org:manage", "org:admin"}, http.StatusOK,
		},
		"admin may toggle classification stats": {
			"prompt-classification-stats", []string{"org:manage"}, http.StatusOK,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(auth.AccountContextKey), &account.Account{
					ID: "acct_123", Type: "organization", WorkOSOrganizationID: "org_1",
				})
				c.Set(string(auth.UserContextKey), &auth.User{ID: "user_123"})
				c.Set(string(auth.SessionContextKey), &auth.Session{
					OrganizationID: "org_1", Permissions: tc.permissions,
				})
				c.Next()
			})
			router.GET("/experiment/:experiment",
				GetAccountExperiment(logger.New("error", "json"), &fakeExperimentStore{}, nil))

			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/experiment/"+tc.slug, nil))
			if response.Code != tc.want {
				t.Errorf("status = %d, want %d (body %s)", response.Code, tc.want, response.Body.String())
			}
		})
	}
}
