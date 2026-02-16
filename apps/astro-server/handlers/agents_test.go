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

	router.POST("/api/v1/agents/:account/:name/register", RegisterAgent(log, index, nil))

	// Expect transaction: BEGIN, INSERT agent, INSERT version, COMMIT
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO agents").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO agent_versions").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
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
	if resp["build_id"] != "a3f2b1c9" {
		t.Errorf("expected build_id 'a3f2b1c9', got %v", resp["build_id"])
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

			router.POST("/api/v1/agents/register", RegisterAgent(log, index, nil))

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

	router.POST("/api/v1/agents/register", RegisterAgent(log, index, nil))

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

	router.POST("/api/v1/agents/register", RegisterAgent(log, index, nil))

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

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["error"] != "Invalid spec YAML" {
		t.Errorf("expected 'Invalid spec YAML' error, got %v", resp["error"])
	}
}


func TestRegisterAgent_DBError(t *testing.T) {
	router, index, mock := setupAgentTestRouter()
	log := logger.New("error", "json")

	router.POST("/api/v1/agents/register", RegisterAgent(log, index, nil))

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

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["error"] != "Failed to register agent" {
		t.Errorf("expected 'Failed to register agent' error, got %v", resp["error"])
	}
}
