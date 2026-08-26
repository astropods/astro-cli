package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/appstore"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/connectapps"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

type fakeConnect struct {
	created         *connectapps.Application
	createErr       error
	deletedApps     []string
	deleteAppErr    error
	secrets         []connectapps.Secret
	createSecretErr error
	deletedSecrets  []string
}

func (f *fakeConnect) CreateApplication(_ context.Context, orgID, name, _ string, scopes []string) (*connectapps.Application, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = &connectapps.Application{ID: "app_workos_1", ClientID: "client_1", Scopes: scopes}
	_ = orgID
	_ = name
	return f.created, nil
}

func (f *fakeConnect) DeleteApplication(_ context.Context, id string) error {
	f.deletedApps = append(f.deletedApps, id)
	return f.deleteAppErr
}

func (f *fakeConnect) CreateSecret(_ context.Context, _ string) (*connectapps.NewSecret, error) {
	if f.createSecretErr != nil {
		return nil, f.createSecretErr
	}
	return &connectapps.NewSecret{
		Secret: connectapps.Secret{ID: "secret_new", Hint: "…wxyz"},
		Value:  "sk_plaintext",
	}, nil
}

func (f *fakeConnect) ListSecrets(_ context.Context, _ string) ([]connectapps.Secret, error) {
	return f.secrets, nil
}

func (f *fakeConnect) DeleteSecret(_ context.Context, id string) error {
	f.deletedSecrets = append(f.deletedSecrets, id)
	return nil
}

type recordingAppAudit struct{ events []auditlog.Event }

func (s *recordingAppAudit) LogAsync(_ *logger.Logger, e auditlog.Event) {
	s.events = append(s.events, e)
}

type appFixture struct {
	handler *AppHandler
	mock    sqlmock.Sqlmock
	connect *fakeConnect
	audit   *recordingAppAudit
	acct    *account.Account
}

func newAppFixture(t *testing.T) *appFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	connect := &fakeConnect{}
	audit := &recordingAppAudit{}
	return &appFixture{
		handler: NewAppHandler(logger.New("error", "json"), appstore.NewStore(db), connect, audit),
		mock:    mock,
		connect: connect,
		audit:   audit,
		acct: &account.Account{
			ID: "acct_123", Type: "organization", WorkOSOrganizationID: "org_123",
		},
	}
}

func (f *appFixture) call(handler gin.HandlerFunc, method, body string) *httptest.ResponseRecorder {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.AccountContextKey), f.acct)
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user_admin"})
		c.Next()
	})
	router.Any("/apps", func(c *gin.Context) {
		c.Params = append(c.Params,
			gin.Param{Key: "app_id", Value: "app-1"},
			gin.Param{Key: "secret_id", Value: "secret_a"},
		)
		handler(c)
	})
	request := httptest.NewRequest(method, "/apps", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func appRow(accountID string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "account_id", "name", "description", "workos_application_id",
		"client_id", "scopes", "created_by", "created_at", "updated_at",
	}).AddRow("app-1", accountID, "ci", "", "app_workos_1", "client_1",
		"{audiences:manage}", "user_admin", time.Now(), time.Now())
}

func TestAppRejectsPersonalAccount(t *testing.T) {
	f := newAppFixture(t)
	f.acct = &account.Account{ID: "acct_p", Type: "personal"}

	response := f.call(f.handler.List, http.MethodGet, "")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s, want 400", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "organizations") {
		t.Fatalf("error should name the constraint: %s", response.Body.String())
	}
}

func TestAppCreateReturnsSecretOnce(t *testing.T) {
	f := newAppFixture(t)
	f.mock.ExpectQuery("INSERT INTO account_apps").WillReturnRows(appRow("acct_123"))

	response := f.call(f.handler.Create, http.MethodPost,
		`{"name":"ci","scopes":["audiences:manage"]}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "sk_plaintext") {
		t.Fatalf("creation must return the plaintext secret: %s", response.Body.String())
	}
	if f.connect.created == nil || f.connect.created.Scopes[0] != "audiences:manage" {
		t.Fatalf("the validated scopes should reach the WorkOS layer: %+v", f.connect.created)
	}
	if len(f.audit.events) != 1 || f.audit.events[0].Action != auditlog.AppCreate {
		t.Fatalf("audit events = %+v", f.audit.events)
	}
}

func TestAppCreateRejectsUnknownScope(t *testing.T) {
	f := newAppFixture(t)

	response := f.call(f.handler.Create, http.MethodPost, `{"name":"ci","scopes":["billing:write"]}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", response.Code)
	}
	if f.connect.created != nil {
		t.Fatal("an unknown scope must not reach WorkOS")
	}
}

func TestAppCreateWithoutScopesIsAllowed(t *testing.T) {
	f := newAppFixture(t)
	f.mock.ExpectQuery("INSERT INTO account_apps").WillReturnRows(appRow("acct_123"))

	response := f.call(f.handler.Create, http.MethodPost, `{"name":"ci"}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s: scopes are not selectable yet", response.Code, response.Body.String())
	}
}

func TestAppCreateRollsBackWorkOSOnRowFailure(t *testing.T) {
	f := newAppFixture(t)
	f.mock.ExpectQuery("INSERT INTO account_apps").WillReturnError(errors.New("boom"))

	response := f.call(f.handler.Create, http.MethodPost, `{"name":"ci","scopes":["audiences:read"]}`)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d, want 500", response.Code)
	}
	if len(f.connect.deletedApps) != 1 || f.connect.deletedApps[0] != "app_workos_1" {
		t.Fatalf("the orphaned WorkOS application must be deleted: %+v", f.connect.deletedApps)
	}
}

func TestAppDeleteRemovesWorkOSApplication(t *testing.T) {
	f := newAppFixture(t)
	f.mock.ExpectQuery("FROM account_apps").WithArgs("app-1").WillReturnRows(appRow("acct_123"))
	f.mock.ExpectExec("DELETE FROM account_apps").WithArgs("app-1").WillReturnResult(sqlmock.NewResult(0, 1))

	if response := f.call(f.handler.Delete, http.MethodDelete, ""); response.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if len(f.connect.deletedApps) != 1 {
		t.Fatalf("WorkOS application not deleted: %+v", f.connect.deletedApps)
	}
}

func TestAppDeleteRejectsAnotherAccountsApp(t *testing.T) {
	f := newAppFixture(t)
	f.mock.ExpectQuery("FROM account_apps").WithArgs("app-1").WillReturnRows(appRow("acct_other"))

	if response := f.call(f.handler.Delete, http.MethodDelete, ""); response.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", response.Code)
	}
	if len(f.connect.deletedApps) != 0 {
		t.Fatal("a foreign app must never reach WorkOS")
	}
}

func TestAppDeleteMissingIs404(t *testing.T) {
	f := newAppFixture(t)
	f.mock.ExpectQuery("FROM account_apps").WithArgs("app-1").WillReturnError(sql.ErrNoRows)

	if response := f.call(f.handler.Delete, http.MethodDelete, ""); response.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", response.Code)
	}
}

func TestAppSecretLimitIs409(t *testing.T) {
	f := newAppFixture(t)
	f.mock.ExpectQuery("FROM account_apps").WithArgs("app-1").WillReturnRows(appRow("acct_123"))
	f.connect.createSecretErr = connectapps.ErrSecretLimit

	response := f.call(f.handler.CreateSecret, http.MethodPost, "")
	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s, want 409", response.Code, response.Body.String())
	}
}

func TestAppRevokeLastSecretIsBlocked(t *testing.T) {
	f := newAppFixture(t)
	f.mock.ExpectQuery("FROM account_apps").WithArgs("app-1").WillReturnRows(appRow("acct_123"))
	f.connect.secrets = []connectapps.Secret{{ID: "secret_a"}}

	response := f.call(f.handler.DeleteSecret, http.MethodDelete, "")
	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s, want 409", response.Code, response.Body.String())
	}
	if len(f.connect.deletedSecrets) != 0 {
		t.Fatal("the only secret must not be revoked")
	}
}

func TestAppRevokeSecretWithAReplacement(t *testing.T) {
	f := newAppFixture(t)
	f.mock.ExpectQuery("FROM account_apps").WithArgs("app-1").WillReturnRows(appRow("acct_123"))
	f.connect.secrets = []connectapps.Secret{{ID: "secret_a"}, {ID: "secret_b"}}

	if response := f.call(f.handler.DeleteSecret, http.MethodDelete, ""); response.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if len(f.connect.deletedSecrets) != 1 || f.connect.deletedSecrets[0] != "secret_a" {
		t.Fatalf("wrong secret revoked: %+v", f.connect.deletedSecrets)
	}
}

func TestAppRevokeUnknownSecretIs404(t *testing.T) {
	f := newAppFixture(t)
	f.mock.ExpectQuery("FROM account_apps").WithArgs("app-1").WillReturnRows(appRow("acct_123"))
	f.connect.secrets = []connectapps.Secret{{ID: "secret_other"}, {ID: "secret_more"}}

	if response := f.call(f.handler.DeleteSecret, http.MethodDelete, ""); response.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", response.Code)
	}
}

func TestAppListSurvivesWorkOSSecretFailure(t *testing.T) {
	f := newAppFixture(t)
	f.mock.ExpectQuery("FROM account_apps").WillReturnRows(appRow("acct_123"))

	response := f.call(f.handler.List, http.MethodGet, "")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "available_scopes") {
		t.Fatalf("the list should advertise the scope vocabulary: %s", response.Body.String())
	}
}
