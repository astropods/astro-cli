package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/postman/astro/apps/astro-server/internal/agentindex"
	"github.com/postman/astro/apps/astro-server/internal/logger"
)

func setupAgentTestRouter() (*gin.Engine, *agentindex.Index, sqlmock.Sqlmock) {
	gin.SetMode(gin.TestMode)
	db, mock, _ := sqlmock.New()
	index := agentindex.NewIndexWithDB(db)
	router := gin.New()
	return router, index, mock
}

func TestRegisterAgent_Success(t *testing.T) {
	router, index, mock := setupAgentTestRouter()
	log := logger.New("error", "json")

	router.POST("/api/v1/agents/register", RegisterAgent(log, index))

	// Expect transaction: BEGIN, INSERT agent, INSERT version, COMMIT
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO agents").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO agent_versions").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	body := `{
		"name": "test-agent",
		"version": "1.0.0",
		"registry": "registry.example.com",
		"spec_content": "name: test-agent\nversion: 1.0.0\n"
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["message"] != "Agent registered successfully" {
		t.Errorf("expected success message, got %v", resp["message"])
	}
	if resp["name"] != "test-agent" {
		t.Errorf("expected name 'test-agent', got %v", resp["name"])
	}
	if resp["version"] != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %v", resp["version"])
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
			name: "missing name",
			body: `{"version": "1.0.0", "registry": "reg.io", "spec_content": "name: x\n"}`,
		},
		{
			name: "missing version",
			body: `{"name": "agent", "registry": "reg.io", "spec_content": "name: x\n"}`,
		},
		{
			name: "missing registry",
			body: `{"name": "agent", "version": "1.0.0", "spec_content": "name: x\n"}`,
		},
		{
			name: "missing spec_content",
			body: `{"name": "agent", "version": "1.0.0", "registry": "reg.io"}`,
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

			router.POST("/api/v1/agents/register", RegisterAgent(log, index))

			req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/register", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
			}

			var resp map[string]interface{}
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

	router.POST("/api/v1/agents/register", RegisterAgent(log, index))

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

	router.POST("/api/v1/agents/register", RegisterAgent(log, index))

	body := `{
		"name": "test-agent",
		"version": "1.0.0",
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

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["error"] != "Invalid spec YAML" {
		t.Errorf("expected 'Invalid spec YAML' error, got %v", resp["error"])
	}
}

// TestRegisterAgent_RouteConflict reproduces the real routing from main.go
// where GET /agents/:name and POST /agents/register coexist.
// This catches the 404 bug: gin matches "register" as :name param.
func TestRegisterAgent_RouteConflict(t *testing.T) {
	router, index, mock := setupAgentTestRouter()
	log := logger.New("error", "json")

	v1 := router.Group("/api/v1")

	// Register routes exactly as main.go does
	v1.GET("/agents", ListAgents(log, index))
	v1.GET("/agents/:name", GetAgent(log, index))
	v1.GET("/agents/:name/:version", GetAgentVersion(log, index))

	protected := v1.Group("")
	protected.POST("/agents/register", RegisterAgent(log, index))

	// Set up mock expectations for a successful registration
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO agents").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO agent_versions").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	body := `{
		"name": "test-agent",
		"version": "1.0.0",
		"registry": "registry.example.com",
		"spec_content": "name: test-agent\nversion: 1.0.0\n"
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d; body: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["message"] != "Agent registered successfully" {
		t.Errorf("expected success message, got %v", resp["message"])
	}
}

// TestRegisterAgent_WithFullRouting verifies POST /agents/register works
// when all routes are registered as they are in main.go (GET /agents/:name etc.)
func TestRegisterAgent_WithFullRouting(t *testing.T) {
	router, index, mock := setupAgentTestRouter()
	log := logger.New("error", "json")

	v1 := router.Group("/api/v1")
	v1.GET("/agents", ListAgents(log, index))
	v1.GET("/agents/:name", GetAgent(log, index))
	v1.GET("/agents/:name/:version", GetAgentVersion(log, index))

	protected := v1.Group("")
	protected.POST("/agents/register", RegisterAgent(log, index))

	// Set up mock for successful registration
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO agents").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO agent_versions").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	body := `{
		"name": "test-agent",
		"version": "1.0.0",
		"registry": "registry.example.com",
		"spec_content": "name: test-agent\nversion: 1.0.0\n"
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d; body: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["message"] != "Agent registered successfully" {
		t.Errorf("expected success message, got %v", resp["message"])
	}
}

// TestRegisterAgent_GETDoesNotMatchRegister verifies that GET /agents/register
// hits the :name wildcard route (not the POST register route). This is the bug
// scenario: if a client follows a redirect and downgrades POST→GET, it gets 404.
func TestRegisterAgent_GETDoesNotMatchRegister(t *testing.T) {
	router, index, mock := setupAgentTestRouter()
	log := logger.New("error", "json")

	v1 := router.Group("/api/v1")
	v1.GET("/agents/:name", GetAgent(log, index))

	protected := v1.Group("")
	protected.POST("/agents/register", RegisterAgent(log, index))

	// Mock: GetAgent will query for agent named "register" and find nothing
	mock.ExpectQuery("SELECT .* FROM agents").
		WithArgs("register").
		WillReturnRows(sqlmock.NewRows([]string{"name", "registry", "created_at", "updated_at"}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/register", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	// GET /agents/register matches GET /agents/:name → tries to find agent "register" → 404
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d; body: %s", http.StatusNotFound, rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["error"] != "Agent not found" {
		t.Errorf("expected 'Agent not found' error, got %v", resp["error"])
	}
	if details, ok := resp["details"].(string); ok {
		if !strings.Contains(details, "register") {
			t.Errorf("expected details to mention 'register', got %v", details)
		}
	}
}

func TestRegisterAgent_DBError(t *testing.T) {
	router, index, mock := setupAgentTestRouter()
	log := logger.New("error", "json")

	router.POST("/api/v1/agents/register", RegisterAgent(log, index))

	// Simulate DB failure on BEGIN
	mock.ExpectBegin().WillReturnError(sqlmock.ErrCancelled)

	body := `{
		"name": "test-agent",
		"version": "1.0.0",
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

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["error"] != "Failed to register agent" {
		t.Errorf("expected 'Failed to register agent' error, got %v", resp["error"])
	}
}
