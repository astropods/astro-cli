package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

func setupAgentTestRouter() (*gin.Engine, *agentindex.Index, sqlmock.Sqlmock) {
	gin.SetMode(gin.TestMode)
	db, mock, _ := sqlmock.New()
	index := agentindex.NewIndexWithDB(db)
	router := gin.New()
	return router, index, mock
}

// injectTestAccount is a test middleware that sets a fake account in context,
// simulating what ResolveAccount middleware does in production.
func injectTestAccount() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(string(auth.AccountContextKey), &account.Account{
			ID:   "test-account-id",
			Name: "testaccount",
			Type: "personal",
		})
		c.Next()
	}
}

func TestRegisterAgent_Success(t *testing.T) {
	router, index, mock := setupAgentTestRouter()
	log := logger.New("error", "json")

	router.POST("/api/v1/agents/:account/:name/register", injectTestAccount(), RegisterAgent(log, index, nil, "", nil, nil))

	// Expect transaction: BEGIN, INSERT agent, INSERT version, COMMIT
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO agents").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO agent_versions").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	body := `{
		"build_id": "a3f2b1c9",
		"registry": "registry.example.com",
		"spec_content": "name: test-agent\nversion: 1.0.0\n"
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/testaccount/test-agent/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["message"] != "Agent registered successfully" {
		t.Errorf("expected success message, got %v", resp["message"])
	}
	if resp["name"] != "test-agent" {
		t.Errorf("expected name 'test-agent', got %v", resp["name"])
	}
	if resp["build_id"] != "a3f2b1c9" {
		t.Errorf("expected build_id 'a3f2b1c9', got %v", resp["build_id"])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestRegisterAgent_NoReadme_ReturnsHint(t *testing.T) {
	router, index, mock := setupAgentTestRouter()
	log := logger.New("error", "json")

	router.POST("/api/v1/agents/:account/:name/register", injectTestAccount(), RegisterAgent(log, index, nil, "", nil, nil))

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO agents").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO agent_versions").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	body := `{
		"build_id": "a3f2b1c9",
		"registry": "registry.example.com",
		"spec_content": "name: test-agent\nversion: 1.0.0\n"
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/testaccount/test-agent/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	hints, ok := resp["hints"].([]any)
	if !ok || len(hints) == 0 {
		t.Fatal("expected hints array in response when readme is empty")
	}

	hint, _ := hints[0].(string)
	if !strings.Contains(hint, "AGENT.md") {
		t.Errorf("expected hint about AGENT.md, got %q", hint)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestRegisterAgent_WithReadme_NoHint(t *testing.T) {
	router, index, mock := setupAgentTestRouter()
	log := logger.New("error", "json")

	router.POST("/api/v1/agents/:account/:name/register", injectTestAccount(), RegisterAgent(log, index, nil, "", nil, nil))

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO agents").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO agent_versions").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	body := `{
		"build_id": "a3f2b1c9",
		"registry": "registry.example.com",
		"spec_content": "name: test-agent\nversion: 1.0.0\n",
		"readme": "# My Agent\nA great agent."
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/testaccount/test-agent/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if _, ok := resp["hints"]; ok {
		t.Error("expected no hints when readme is provided")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestRegisterAgent_MissingFields(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "missing build_id",
			body: `{"name": "agent", "registry": "reg.io", "spec_content": "name: x\n"}`,
		},
		{
			name: "missing registry",
			body: `{"name": "agent", "build_id": "a3f2b1c9", "spec_content": "name: x\n"}`,
		},
		{
			name: "missing spec_content",
			body: `{"name": "agent", "build_id": "a3f2b1c9", "registry": "reg.io"}`,
		},
		{
			name: "empty body",
			body: `{}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, index, _ := setupAgentTestRouter()
			log := logger.New("error", "json")

			router.POST("/api/v1/agents/register", injectTestAccount(), RegisterAgent(log, index, nil, "", nil, nil))

			req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/register", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
			}

			var resp map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}

			if resp["error"] != "Invalid request body" {
				t.Errorf("expected 'Invalid request body' error, got %v", resp["error"])
			}
		})
	}
}

func TestRegisterAgent_InvalidJSON(t *testing.T) {
	router, index, _ := setupAgentTestRouter()
	log := logger.New("error", "json")

	router.POST("/api/v1/agents/register", injectTestAccount(), RegisterAgent(log, index, nil, "", nil, nil))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/register", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestRegisterAgent_InvalidYAMLSpec(t *testing.T) {
	router, index, _ := setupAgentTestRouter()
	log := logger.New("error", "json")

	router.POST("/api/v1/agents/register", injectTestAccount(), RegisterAgent(log, index, nil, "", nil, nil))

	body := `{
		"name": "test-agent",
		"build_id": "a3f2b1c9",
		"registry": "registry.example.com",
		"spec_content": "invalid: yaml: [: content"
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["error"] != "Invalid spec YAML" {
		t.Errorf("expected 'Invalid spec YAML' error, got %v", resp["error"])
	}
}

func TestRegisterAgent_RejectsSecretDefaults(t *testing.T) {
	router, index, _ := setupAgentTestRouter()
	log := logger.New("error", "json")

	router.POST("/api/v1/agents/:account/:name/register", injectTestAccount(), RegisterAgent(log, index, nil, "", nil, nil))

	// Spec with a secret input that still has a default value
	specYAML := `name: test-agent
spec: package/v1
agent:
  image: test:latest
inputs:
  api_key:
    name: API_KEY
    secret: true
    default: sk-leaked-secret
    datatype: string
`
	body := fmt.Sprintf(`{
		"build_id": "a3f2b1c9",
		"registry": "registry.example.com",
		"spec_content": %q
	}`, specYAML)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/testaccount/test-agent/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if !strings.Contains(resp["error"].(string), "Secret inputs must not have default values") {
		t.Errorf("expected secret default rejection error, got %v", resp["error"])
	}
}

func TestRegisterAgent_DBError(t *testing.T) {
	router, index, mock := setupAgentTestRouter()
	log := logger.New("error", "json")

	router.POST("/api/v1/agents/register", injectTestAccount(), RegisterAgent(log, index, nil, "", nil, nil))

	// Simulate DB failure on BEGIN
	mock.ExpectBegin().WillReturnError(sqlmock.ErrCancelled)

	body := `{
		"name": "test-agent",
		"build_id": "a3f2b1c9",
		"registry": "registry.example.com",
		"spec_content": "name: test-agent\n"
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["error"] != "Failed to register agent" {
		t.Errorf("expected 'Failed to register agent' error, got %v", resp["error"])
	}
}

func TestRegisterAgent_RejectsOrgScopedName(t *testing.T) {
	tests := []struct {
		name      string
		agentName string
		wantCode  int
	}{
		// Names with slashes get a 404 from gin's router before reaching the handler,
		// so we only test cases that reach our validation code.
		{"bare @ rejected", "@my-agent", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, _, _ := setupAgentTestRouter()
			log := logger.New("error", "json")
			router.POST("/api/v1/agents/:account/:name/register", injectTestAccount(), RegisterAgent(log, nil, nil, "", nil, nil))

			body := `{
				"build_id": "a3f2b1c9",
				"registry": "registry.example.com",
				"spec_content": "name: test-agent\nversion: 1.0.0\n"
			}`

			req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/testaccount/"+url.PathEscape(tt.agentName)+"/register", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Errorf("expected status %d, got %d: %s", tt.wantCode, rec.Code, rec.Body.String())
			}
			var resp map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}
			errMsg, _ := resp["error"].(string)
			if !strings.Contains(errMsg, "@org/ prefix") {
				t.Errorf("expected org prefix error message, got %q", errMsg)
			}
		})
	}
}

func TestRegisterAgent_VersionGate(t *testing.T) {
	tests := []struct {
		name           string
		minVersion     string
		cliVersion     string
		expectRejected bool
	}{
		{"no gate configured", "", "0.1.0", false},
		{"cli meets minimum", "0.3.0", "0.3.7", false},
		{"cli equals minimum", "0.3.7", "0.3.7", false},
		{"cli below minimum", "0.4.0", "0.3.7", true},
		{"no header sent", "0.3.0", "", true},
		{"dev build rejected", "0.3.0", "dev", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, index, mock := setupAgentTestRouter()
			log := logger.New("error", "json")

			router.POST("/api/v1/agents/:account/:name/register", injectTestAccount(), RegisterAgent(log, index, nil, tt.minVersion, nil, nil))

			if !tt.expectRejected {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO agents").
					WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec("INSERT INTO agent_versions").
					WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectCommit()
			}

			body := `{
				"build_id": "a3f2b1c9",
				"registry": "registry.example.com",
				"spec_content": "name: test-agent\nversion: 1.0.0\n"
			}`

			req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/testaccount/test-agent/register", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			if tt.cliVersion != "" {
				req.Header.Set("X-Cli-Version", tt.cliVersion)
			}
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if tt.expectRejected {
				if rec.Code != http.StatusUpgradeRequired {
					t.Errorf("expected status %d, got %d: %s", http.StatusUpgradeRequired, rec.Code, rec.Body.String())
				}
				var resp map[string]any
				if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}
				errMsg, _ := resp["error"].(string)
				if !strings.Contains(errMsg, "minimum") {
					t.Errorf("expected version gate error message, got %q", errMsg)
				}
			} else {
				if rec.Code != http.StatusCreated {
					t.Errorf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
				}
			}
		})
	}
}
