package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/accountvars"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

func setupVarRouter() (*gin.Engine, sqlmock.Sqlmock) {
	gin.SetMode(gin.TestMode)
	db, mock, _ := sqlmock.New()
	store := accountvars.NewStore(db)
	log := logger.New("error", "json")
	cfg := &config.Config{} // no KMS — secrets stored as plaintext in tests

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.AccountContextKey), &account.Account{ID: "acct-1", Name: "testacct"})
		c.Next()
	})
	router.POST("/variables", CreateAccountVariable(log, store, cfg))
	return router, mock
}

func postVariables(router *gin.Engine, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/variables", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestCreateAccountVariable_SingleEntry(t *testing.T) {
	router, mock := setupVarRouter()

	mock.ExpectExec("INSERT INTO account_variables").
		WithArgs("acct-1", "API_KEY", "sk-123", false, sqlmock.AnyArg(), "My key").
		WillReturnResult(sqlmock.NewResult(0, 1))

	rec := postVariables(router, `{"variables":[{"name":"API_KEY","value":"sk-123","secret":false,"description":"My key"}]}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp CreateAccountVariablesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %s", err)
	}

	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if resp.Results[0].Name != "API_KEY" || resp.Results[0].Status != "created" {
		t.Errorf("unexpected result: %+v", resp.Results[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet mock expectations: %s", err)
	}
}

func TestCreateAccountVariable_BulkEntries(t *testing.T) {
	router, mock := setupVarRouter()

	mock.ExpectExec("INSERT INTO account_variables").
		WithArgs("acct-1", "FOO", "bar", false, sqlmock.AnyArg(), "").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO account_variables").
		WithArgs("acct-1", "SECRET_KEY", "s3cr3t", true, sqlmock.AnyArg(), "").
		WillReturnResult(sqlmock.NewResult(0, 1))

	body := `{"variables":[
		{"name":"FOO","value":"bar","secret":false},
		{"name":"SECRET_KEY","value":"s3cr3t","secret":true}
	]}`
	rec := postVariables(router, body)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp CreateAccountVariablesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %s", err)
	}

	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Results))
	}
	for _, r := range resp.Results {
		if r.Status != "created" {
			t.Errorf("expected status 'created' for %s, got %s (error: %s)", r.Name, r.Status, r.Error)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet mock expectations: %s", err)
	}
}

func TestCreateAccountVariable_InvalidName(t *testing.T) {
	router, mock := setupVarRouter()

	// The valid entry should still be saved
	mock.ExpectExec("INSERT INTO account_variables").
		WithArgs("acct-1", "GOOD_KEY", "val", false, sqlmock.AnyArg(), "").
		WillReturnResult(sqlmock.NewResult(0, 1))

	body := `{"variables":[
		{"name":"1BAD","value":"x","secret":false},
		{"name":"GOOD_KEY","value":"val","secret":false}
	]}`
	rec := postVariables(router, body)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp CreateAccountVariablesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %s", err)
	}

	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Results))
	}
	if resp.Results[0].Status != "error" || resp.Results[0].Error != "invalid variable name" {
		t.Errorf("expected error for 1BAD, got: %+v", resp.Results[0])
	}
	if resp.Results[1].Status != "created" {
		t.Errorf("expected created for GOOD_KEY, got: %+v", resp.Results[1])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet mock expectations: %s", err)
	}
}

func TestCreateAccountVariable_MixedCaseNames(t *testing.T) {
	router, mock := setupVarRouter()

	names := []string{"myApp_key", "database_url", "_INTERNAL", "NodeEnv"}
	for _, name := range names {
		mock.ExpectExec("INSERT INTO account_variables").
			WithArgs("acct-1", name, "val", false, sqlmock.AnyArg(), "").
			WillReturnResult(sqlmock.NewResult(0, 1))
	}

	entries := make([]string, len(names))
	for i, name := range names {
		entries[i] = `{"name":"` + name + `","value":"val","secret":false}`
	}
	body := `{"variables":[` + strings.Join(entries, ",") + `]}`
	rec := postVariables(router, body)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp CreateAccountVariablesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %s", err)
	}

	for i, r := range resp.Results {
		if r.Status != "created" {
			t.Errorf("expected created for %s, got %s (error: %s)", names[i], r.Status, r.Error)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet mock expectations: %s", err)
	}
}

func TestCreateAccountVariable_EmptyVariables(t *testing.T) {
	router, _ := setupVarRouter()

	rec := postVariables(router, `{"variables":[]}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateAccountVariable_InvalidJSON(t *testing.T) {
	router, _ := setupVarRouter()

	rec := postVariables(router, `not json`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateAccountVariable_DBError(t *testing.T) {
	router, mock := setupVarRouter()

	mock.ExpectExec("INSERT INTO account_variables").
		WithArgs("acct-1", "FAIL_KEY", "val", false, sqlmock.AnyArg(), "").
		WillReturnError(sqlmock.ErrCancelled)

	rec := postVariables(router, `{"variables":[{"name":"FAIL_KEY","value":"val","secret":false}]}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (per-entry errors), got %d: %s", rec.Code, rec.Body.String())
	}

	var resp CreateAccountVariablesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %s", err)
	}

	if resp.Results[0].Status != "error" || resp.Results[0].Error != "failed to save" {
		t.Errorf("expected save error, got: %+v", resp.Results[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet mock expectations: %s", err)
	}
}

func TestValidVarName(t *testing.T) {
	valid := []string{
		"FOO", "API_KEY", "a", "myVar", "database_url",
		"NodeEnv", "_INTERNAL", "_", "X", "A1_B2",
	}
	invalid := []string{
		"", "1BAD", "HAS SPACE", "HAS-DASH", "has.dot",
	}

	for _, name := range valid {
		if !validVarName.MatchString(name) {
			t.Errorf("expected %q to be valid", name)
		}
	}
	for _, name := range invalid {
		if validVarName.MatchString(name) {
			t.Errorf("expected %q to be invalid", name)
		}
	}
}
