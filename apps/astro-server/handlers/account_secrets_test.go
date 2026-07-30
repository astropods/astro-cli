package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/accountvars"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	spec "github.com/astropods/astro-spec"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
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

func setupUpdateVarRouter() (*gin.Engine, sqlmock.Sqlmock) {
	gin.SetMode(gin.TestMode)
	db, mock, _ := sqlmock.New()
	store := accountvars.NewStore(db)
	log := logger.New("error", "json")
	cfg := &config.Config{} // no KMS

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.AccountContextKey), &account.Account{ID: "acct-1", Name: "testacct"})
		c.Next()
	})
	router.PUT("/variables/:varName", UpdateAccountVariable(log, store, cfg))
	return router, mock
}

func putVariable(router *gin.Engine, name, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, "/variables/"+name, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// TestSecretStorageNoKMS verifies that both create and update store secret values as
// plaintext when KMS is not configured — no base64 wrapping should occur.
func TestSecretStorageNoKMS(t *testing.T) {
	t.Run("create stores plaintext", func(t *testing.T) {
		router, mock := setupVarRouter()

		mock.ExpectExec("INSERT INTO account_variables").
			WithArgs("acct-1", "MY_SECRET", "s3cr3t", true, []byte(nil), "").
			WillReturnResult(sqlmock.NewResult(0, 1))

		rec := postVariables(router, `{"variables":[{"name":"MY_SECRET","value":"s3cr3t","secret":true}]}`)
		require.Equal(t, http.StatusOK, rec.Code)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("update stores plaintext", func(t *testing.T) {
		router, mock := setupUpdateVarRouter()
		now := time.Now()

		rows := sqlmock.NewRows([]string{"account_id", "name", "value", "secret", "nonce", "description", "created_at", "updated_at"}).
			AddRow("acct-1", "MY_SECRET", "old_value", true, nil, "", now, now)
		mock.ExpectQuery("SELECT.*account_variables").
			WithArgs("acct-1", "MY_SECRET").
			WillReturnRows(rows)

		// Expects plaintext value — currently FAILS because update base64-encodes without KMS.
		mock.ExpectExec("INSERT INTO account_variables").
			WithArgs("acct-1", "MY_SECRET", "s3cr3t", true, []byte(nil), "").
			WillReturnResult(sqlmock.NewResult(0, 1))

		rec := putVariable(router, "MY_SECRET", `{"value":"s3cr3t"}`)
		require.Equal(t, http.StatusOK, rec.Code)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func setupGetVarRouter() (*gin.Engine, sqlmock.Sqlmock) {
	gin.SetMode(gin.TestMode)
	db, mock, _ := sqlmock.New()
	store := accountvars.NewStore(db)
	log := logger.New("error", "json")

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.AccountContextKey), &account.Account{ID: "acct-1", Name: "testacct"})
		c.Next()
	})
	router.GET("/variables/:varName", GetAccountVariable(log, store))
	return router, mock
}

func getVariable(router *gin.Engine, name string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/variables/"+name, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestGetAccountVariable_Plain(t *testing.T) {
	router, mock := setupGetVarRouter()
	now := time.Now()

	rows := sqlmock.NewRows([]string{"account_id", "name", "value", "secret", "nonce", "description", "created_at", "updated_at"}).
		AddRow("acct-1", "DB_URL", "postgres://localhost/db", false, nil, "database url", now, now)
	mock.ExpectQuery("SELECT.*account_variables").
		WithArgs("acct-1", "DB_URL").
		WillReturnRows(rows)

	rec := getVariable(router, "DB_URL")
	require.Equal(t, http.StatusOK, rec.Code)

	var meta accountvars.VariableMetadata
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &meta))
	require.False(t, meta.Secret)
	require.NotNil(t, meta.Value)
	require.Equal(t, "postgres://localhost/db", *meta.Value)
}

func TestGetAccountVariable_Secret(t *testing.T) {
	router, mock := setupGetVarRouter()
	now := time.Now()

	rows := sqlmock.NewRows([]string{"account_id", "name", "value", "secret", "nonce", "description", "created_at", "updated_at"}).
		AddRow("acct-1", "API_KEY", "ciphertext", true, []byte("nonce12bytes"), "", now, now)
	mock.ExpectQuery("SELECT.*account_variables").
		WithArgs("acct-1", "API_KEY").
		WillReturnRows(rows)

	rec := getVariable(router, "API_KEY")
	require.Equal(t, http.StatusOK, rec.Code)

	var meta accountvars.VariableMetadata
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &meta))
	require.True(t, meta.Secret)
	require.Nil(t, meta.Value)
}

func TestGetAccountVariable_NotFound(t *testing.T) {
	router, mock := setupGetVarRouter()

	mock.ExpectQuery("SELECT.*account_variables").
		WithArgs("acct-1", "MISSING").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "name", "value", "secret", "nonce", "description", "created_at", "updated_at"}))

	rec := getVariable(router, "MISSING")
	require.Equal(t, http.StatusNotFound, rec.Code)
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
		if !spec.IsValidVarName(name) {
			t.Errorf("expected %q to be valid", name)
		}
	}
	for _, name := range invalid {
		if spec.IsValidVarName(name) {
			t.Errorf("expected %q to be invalid", name)
		}
	}
}

func TestResolveVarReferences_RejectsSecretTypeMismatch(t *testing.T) {
	tests := []struct {
		name             string
		accountVarSecret bool
		deploymentSecret bool
		wantMessage      string
	}{
		{
			name:             "secret account variable into plain deployment variable",
			accountVarSecret: true,
			deploymentSecret: false,
			wantMessage:      `secret account variable "SHARED_VALUE" cannot resolve a plain deployment variable`,
		},
		{
			name:             "plain account variable into secret deployment variable",
			accountVarSecret: false,
			deploymentSecret: true,
			wantMessage:      `plain account variable "SHARED_VALUE" cannot resolve a secret deployment variable`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)

			now := time.Now()
			rows := sqlmock.NewRows([]string{
				"account_id", "name", "value", "secret", "nonce", "description", "created_at", "updated_at",
			}).AddRow("acct-1", "SHARED_VALUE", "stored-value", tt.accountVarSecret, nil, "", now, now)
			mock.ExpectQuery("SELECT.*account_variables").
				WithArgs("acct-1", sqlmock.AnyArg()).
				WillReturnRows(rows)

			deployment := &spec.AstroDeploymentSpec{
				Variables: map[string]spec.Variable{
					"TARGET_VALUE": {
						Ref:    "SHARED_VALUE",
						Secret: tt.deploymentSecret,
					},
				},
			}
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/", nil)

			refs, resolveErr := resolveVarReferences(
				ctx,
				logger.New("error", "json"),
				deployment,
				"acct-1",
				accountvars.NewStore(db),
				&config.Config{},
			)

			require.Nil(t, refs)
			require.ErrorContains(t, resolveErr, "incompatible variable references")
			require.ErrorContains(t, resolveErr, tt.wantMessage)
			require.Equal(t, "SHARED_VALUE", deployment.Variables["TARGET_VALUE"].Ref)
			require.Empty(t, deployment.Variables["TARGET_VALUE"].Value)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestResolveVarReferences_ResolvesMatchingTypes(t *testing.T) {
	tests := []struct {
		name   string
		secret bool
	}{
		{name: "plain", secret: false},
		{name: "secret", secret: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)

			now := time.Now()
			rows := sqlmock.NewRows([]string{
				"account_id", "name", "value", "secret", "nonce", "description", "created_at", "updated_at",
			}).AddRow("acct-1", "SHARED_VALUE", "stored-value", tt.secret, nil, "", now, now)
			mock.ExpectQuery("SELECT.*account_variables").
				WithArgs("acct-1", sqlmock.AnyArg()).
				WillReturnRows(rows)
			if tt.secret {
				mock.ExpectQuery("SELECT.*account_encryption_keys").
					WithArgs("acct-1").
					WillReturnRows(sqlmock.NewRows([]string{
						"account_id", "encrypted_data_key", "kms_key_arn", "created_at",
					}))
			}

			deployment := &spec.AstroDeploymentSpec{
				Variables: map[string]spec.Variable{
					"TARGET_VALUE": {
						Ref:    "SHARED_VALUE",
						Secret: tt.secret,
					},
				},
			}
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/", nil)

			refs, resolveErr := resolveVarReferences(
				ctx,
				logger.New("error", "json"),
				deployment,
				"acct-1",
				accountvars.NewStore(db),
				&config.Config{},
			)

			require.NoError(t, resolveErr)
			require.Equal(t, map[string]string{"TARGET_VALUE": "SHARED_VALUE"}, refs)
			require.Empty(t, deployment.Variables["TARGET_VALUE"].Ref)
			require.Equal(t, "stored-value", deployment.Variables["TARGET_VALUE"].Value)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
