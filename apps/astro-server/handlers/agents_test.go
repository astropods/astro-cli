package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/heartstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/metricsstore"
	"github.com/astropods/astro/apps/astro-server/internal/openmeter"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

// --- stubs for resolvePublishers ---

type stubUserGetter struct {
	users map[string]*auth.User
}

func (s *stubUserGetter) GetUser(_ context.Context, userID string) (*auth.User, error) {
	u, ok := s.users[userID]
	if !ok {
		return nil, fmt.Errorf("user not found: %s", userID)
	}
	return u, nil
}

type stubAccountLister struct {
	accounts map[string][]account.AccountWithRole
}

func (s *stubAccountLister) GetAccountsForUser(userID string) ([]account.AccountWithRole, error) {
	return s.accounts[userID], nil
}

func TestResolvePublishers_EmptyActors(t *testing.T) {
	users := &stubUserGetter{users: map[string]*auth.User{}}
	accts := &stubAccountLister{accounts: map[string][]account.AccountWithRole{}}

	result := resolvePublishers(context.Background(), nil, users, accts, nil)
	if len(result) != 0 {
		t.Errorf("expected empty slice, got %v", result)
	}
}

func TestResolvePublishers_FullNameAndHandle(t *testing.T) {
	users := &stubUserGetter{users: map[string]*auth.User{
		"u1": {FirstName: "Jane", LastName: "Smith"},
	}}
	accts := &stubAccountLister{accounts: map[string][]account.AccountWithRole{
		"u1": {{Name: "janesmith", Type: "personal"}},
	}}

	result := resolvePublishers(context.Background(), []string{"u1"}, users, accts, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 publisher, got %d", len(result))
	}
	if result[0].Name != "Jane Smith" {
		t.Errorf("expected name 'Jane Smith', got %q", result[0].Name)
	}
	if result[0].Account != "janesmith" {
		t.Errorf("expected account 'janesmith', got %q", result[0].Account)
	}
}

func TestResolvePublishers_NoWorkOSName_FallsBackToHandle(t *testing.T) {
	users := &stubUserGetter{users: map[string]*auth.User{
		"u1": {FirstName: "", LastName: ""},
	}}
	accts := &stubAccountLister{accounts: map[string][]account.AccountWithRole{
		"u1": {{Name: "ghostuser", Type: "personal"}},
	}}

	result := resolvePublishers(context.Background(), []string{"u1"}, users, accts, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 publisher, got %d", len(result))
	}
	if result[0].Name != "ghostuser" {
		t.Errorf("expected name 'ghostuser', got %q", result[0].Name)
	}
}

func TestResolvePublishers_NoNameNoHandle_Skipped(t *testing.T) {
	users := &stubUserGetter{users: map[string]*auth.User{
		"u1": {FirstName: "", LastName: ""},
	}}
	accts := &stubAccountLister{accounts: map[string][]account.AccountWithRole{}}

	result := resolvePublishers(context.Background(), []string{"u1"}, users, accts, nil)
	if len(result) != 0 {
		t.Errorf("expected actor with no name/handle to be skipped, got %v", result)
	}
}

func TestResolvePublishers_WorkOSError_Skipped(t *testing.T) {
	users := &stubUserGetter{users: map[string]*auth.User{}}
	accts := &stubAccountLister{accounts: map[string][]account.AccountWithRole{}}

	result := resolvePublishers(context.Background(), []string{"missing-user"}, users, accts, nil)
	if len(result) != 0 {
		t.Errorf("expected unresolvable actor to be skipped, got %v", result)
	}
}

func TestResolvePublishers_MultipleActors_OrderPreserved(t *testing.T) {
	users := &stubUserGetter{users: map[string]*auth.User{
		"u1": {FirstName: "Alice", LastName: ""},
		"u2": {FirstName: "Bob", LastName: ""},
		"u3": {FirstName: "Carol", LastName: ""},
	}}
	accts := &stubAccountLister{accounts: map[string][]account.AccountWithRole{
		"u1": {{Name: "alice", Type: "personal"}},
		"u2": {{Name: "bob", Type: "personal"}},
		"u3": {{Name: "carol", Type: "personal"}},
	}}

	result := resolvePublishers(context.Background(), []string{"u1", "u2", "u3"}, users, accts, nil)
	if len(result) != 3 {
		t.Fatalf("expected 3 publishers, got %d", len(result))
	}
	for i, want := range []string{"Alice", "Bob", "Carol"} {
		if result[i].Name != want {
			t.Errorf("position %d: expected %q, got %q", i, want, result[i].Name)
		}
	}
}

func TestResolvePublishers_NonPersonalAccountIgnored(t *testing.T) {
	users := &stubUserGetter{users: map[string]*auth.User{
		"u1": {FirstName: "Dev", LastName: "Team"},
	}}
	accts := &stubAccountLister{accounts: map[string][]account.AccountWithRole{
		"u1": {
			{Name: "acme-org", Type: "organization"},
			{Name: "devteam", Type: "personal"},
		},
	}}

	result := resolvePublishers(context.Background(), []string{"u1"}, users, accts, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 publisher, got %d", len(result))
	}
	if result[0].Account != "devteam" {
		t.Errorf("expected personal account 'devteam', got %q", result[0].Account)
	}
}

func setupAgentTestRouter() (*gin.Engine, *agentindex.Index, sqlmock.Sqlmock) {
	gin.SetMode(gin.TestMode)
	db, mock, _ := sqlmock.New()
	index := agentindex.NewIndexWithDB(db)
	router := gin.New()
	return router, index, mock
}

func blueprintAccountListColumns(paginated bool) []string {
	cols := []string{
		"account_id", "name", "registry", "visibility", "avatar_colors", "created_at", "updated_at",
		"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at",
		"version_count",
	}
	if paginated {
		cols = append(cols, "list_total")
	}
	return cols
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

	router.POST("/api/v1/agents/:account/:name/register", injectTestAccount(), RegisterAgent(log, index, nil, "", nil, nil, nil))

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
		"spec_content": "name: test-agent\nagent:\n  image: agent:latest\n"
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

	router.POST("/api/v1/agents/:account/:name/register", injectTestAccount(), RegisterAgent(log, index, nil, "", nil, nil, nil))

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
		"spec_content": "name: test-agent\nagent:\n  image: agent:latest\n"
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

	router.POST("/api/v1/agents/:account/:name/register", injectTestAccount(), RegisterAgent(log, index, nil, "", nil, nil, nil))

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
		"spec_content": "name: test-agent\nagent:\n  image: agent:latest\n",
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

			router.POST("/api/v1/agents/:account/:name/register", injectTestAccount(), RegisterAgent(log, index, nil, "", nil, nil, nil))

			req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/testaccount/my-agent/register", strings.NewReader(tt.body))
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

	router.POST("/api/v1/agents/:account/:name/register", injectTestAccount(), RegisterAgent(log, index, nil, "", nil, nil, nil))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/testaccount/my-agent/register", strings.NewReader("not json"))
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

	router.POST("/api/v1/agents/:account/:name/register", injectTestAccount(), RegisterAgent(log, index, nil, "", nil, nil, nil))

	body := `{
		"name": "test-agent",
		"build_id": "a3f2b1c9",
		"registry": "registry.example.com",
		"spec_content": "invalid: yaml: [: content"
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/testaccount/my-agent/register", strings.NewReader(body))
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

	router.POST("/api/v1/agents/:account/:name/register", injectTestAccount(), RegisterAgent(log, index, nil, "", nil, nil, nil))

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

	router.POST("/api/v1/agents/:account/:name/register", injectTestAccount(), RegisterAgent(log, index, nil, "", nil, nil, nil))

	// Simulate DB failure on BEGIN
	mock.ExpectBegin().WillReturnError(sqlmock.ErrCancelled)

	body := `{
		"name": "test-agent",
		"build_id": "a3f2b1c9",
		"registry": "registry.example.com",
		"spec_content": "name: test-agent\nagent:\n  image: agent:latest\n"
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/testaccount/my-agent/register", strings.NewReader(body))
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
			router.POST("/api/v1/agents/:account/:name/register", injectTestAccount(), RegisterAgent(log, nil, nil, "", nil, nil, nil))

			body := `{
				"build_id": "a3f2b1c9",
				"registry": "registry.example.com",
				"spec_content": "name: test-agent\nagent:\n  image: agent:latest\n"
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

func TestRegisterAgent_RejectsInvalidName(t *testing.T) {
	invalid := []string{"Weather-poet", "My_Agent", "UPPER_CASE", "has space", "-leading", "trailing-"}
	for _, name := range invalid {
		t.Run(name, func(t *testing.T) {
			router, index, _ := setupAgentTestRouter()
			log := logger.New("error", "json")
			router.POST("/api/v1/agents/:account/:name/register", injectTestAccount(), RegisterAgent(log, index, nil, "", nil, nil, nil))

			body := `{"build_id":"b1","registry":"r.example.com","spec_content":"name: test\nversion: 1.0\n"}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/testaccount/"+url.PathEscape(name)+"/register", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected 400 for name %q, got %d: %s", name, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestCreateBlueprint_Success(t *testing.T) {
	router, index, mock := setupAgentTestRouter()
	log := logger.New("error", "json")

	router.POST("/api/v1/agents/:account", injectTestAccount(), CreateBlueprint(log, index, nil, nil, nil, nil, nil))

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO agents").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("DELETE FROM agent_versions").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	body := `{"name": "my-agent"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/testaccount", strings.NewReader(body))
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
	if resp["name"] != "my-agent" {
		t.Errorf("expected name 'my-agent', got %v", resp["name"])
	}
	if resp["account"] != "testaccount" {
		t.Errorf("expected account 'testaccount', got %v", resp["account"])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestCreateBlueprint_ConflictReturns409(t *testing.T) {
	router, index, mock := setupAgentTestRouter()
	log := logger.New("error", "json")

	router.POST("/api/v1/agents/:account", injectTestAccount(), CreateBlueprint(log, index, nil, nil, nil, nil, nil))

	// Active agent: INSERT returns 0 rows affected → ErrAlreadyExists
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO agents").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	body := `{"name": "existing-agent"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/testaccount", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d: %s", http.StatusConflict, rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	errMsg, _ := resp["error"].(string)
	if !strings.Contains(errMsg, "already exists") {
		t.Errorf("expected 'already exists' in error, got %q", errMsg)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestCreateBlueprint_InvalidNameReturns400(t *testing.T) {
	router, index, _ := setupAgentTestRouter()
	log := logger.New("error", "json")

	router.POST("/api/v1/agents/:account", injectTestAccount(), CreateBlueprint(log, index, nil, nil, nil, nil, nil))

	body := `{"name": "INVALID NAME!!!"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/testaccount", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
}

func TestCreateBlueprint_DBErrorReturns500(t *testing.T) {
	router, index, mock := setupAgentTestRouter()
	log := logger.New("error", "json")

	router.POST("/api/v1/agents/:account", injectTestAccount(), CreateBlueprint(log, index, nil, nil, nil, nil, nil))

	mock.ExpectBegin().WillReturnError(sqlmock.ErrCancelled)

	body := `{"name": "my-agent"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/testaccount", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d: %s", http.StatusInternalServerError, rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
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

			router.POST("/api/v1/agents/:account/:name/register", injectTestAccount(), RegisterAgent(log, index, nil, tt.minVersion, nil, nil, nil))

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
				"spec_content": "name: test-agent\nagent:\n  image: agent:latest\n"
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

func TestListAccountAgents_QueryFilterPassedToIndex(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := logger.New("error", "json")
	now := time.Now()

	indexDB, indexMock, _ := sqlmock.New()
	defer indexDB.Close()
	index := agentindex.NewIndexWithDB(indexDB)
	indexMock.ExpectQuery("SELECT a.account_id, a.name, a.registry").
		WithArgs("test-account-id", "public", "%analytics%", defaultBlueprintListLimit, 0).
		WillReturnRows(sqlmock.NewRows(blueprintAccountListColumns(true)))

	accountDB, accountMock, _ := sqlmock.New()
	defer accountDB.Close()
	accountStore := account.NewAccountStore(accountDB)
	accountMock.ExpectQuery("SELECT a.id, a.name, a.type").
		WithArgs("testaccount").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "cluster_id", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}).
			AddRow("test-account-id", "testaccount", "personal", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, pq.StringArray(nil), pq.StringArray(nil)))

	heartsDB, _, _ := sqlmock.New()
	defer heartsDB.Close()
	metricsDB, _, _ := sqlmock.New()
	defer metricsDB.Close()
	deploysDB, _, _ := sqlmock.New()
	defer deploysDB.Close()

	router := gin.New()
	router.GET("/api/v1/agents/:account", ListAccountAgents(log, index, accountStore,
		heartstore.New(heartsDB), metricsstore.New(metricsDB), deploymentstore.NewStore(deploysDB),
		nil, nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/testaccount?q=analytics", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := indexMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("index expectations: %v", err)
	}
}

func TestListAccountAgents_Member_PrivateVisibilityFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := logger.New("error", "json")
	now := time.Now()

	indexDB, indexMock, _ := sqlmock.New()
	defer indexDB.Close()
	index := agentindex.NewIndexWithDB(indexDB)
	indexMock.ExpectQuery("SELECT a.account_id, a.name, a.registry").
		WithArgs("test-account-id", "private", defaultBlueprintListLimit, 0).
		WillReturnRows(sqlmock.NewRows(blueprintAccountListColumns(true)).
			AddRow("test-account-id", "secret-agent", "registry.example.com", "private", nil, now, now,
				"build-1", "ns", `{"name":"test"}`, "", "", "[]", now, now, 1, 1))

	accountDB, accountMock, _ := sqlmock.New()
	defer accountDB.Close()
	accountStore := account.NewAccountStore(accountDB)
	accountMock.ExpectQuery("SELECT a.id, a.name, a.type").
		WithArgs("testaccount").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "cluster_id", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}).
			AddRow("test-account-id", "testaccount", "personal", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, pq.StringArray(nil), pq.StringArray(nil)))
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("test-account-id", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	heartsDB, _, _ := sqlmock.New()
	defer heartsDB.Close()
	metricsDB, _, _ := sqlmock.New()
	defer metricsDB.Close()
	deploysDB, _, _ := sqlmock.New()
	defer deploysDB.Close()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET("/api/v1/agents/:account", ListAccountAgents(log, index, accountStore,
		heartstore.New(heartsDB), metricsstore.New(metricsDB), deploymentstore.NewStore(deploysDB),
		nil, nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/testaccount?visibility=private", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := indexMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("index expectations: %v", err)
	}
}

func TestListAccountAgents_Member_NoVisibilityFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := logger.New("error", "json")
	now := time.Now()

	indexDB, indexMock, _ := sqlmock.New()
	defer indexDB.Close()
	index := agentindex.NewIndexWithDB(indexDB)
	indexMock.ExpectQuery("SELECT a.account_id, a.name, a.registry").
		WithArgs("test-account-id", defaultBlueprintListLimit, 0).
		WillReturnRows(sqlmock.NewRows(blueprintAccountListColumns(true)).
			AddRow("test-account-id", "public-agent", "registry.example.com", "public", nil, now, now,
				"build-1", "ns", `{"name":"test"}`, "", "", "[]", now, now, 1, 2).
			AddRow("test-account-id", "private-agent", "registry.example.com", "private", nil, now, now,
				"build-2", "ns", `{"name":"test"}`, "", "", "[]", now, now, 1, 2))

	accountDB, accountMock, _ := sqlmock.New()
	defer accountDB.Close()
	accountStore := account.NewAccountStore(accountDB)
	accountMock.ExpectQuery("SELECT a.id, a.name, a.type").
		WithArgs("testaccount").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "cluster_id", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}).
			AddRow("test-account-id", "testaccount", "personal", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, pq.StringArray(nil), pq.StringArray(nil)))
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("test-account-id", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	heartsDB, _, _ := sqlmock.New()
	defer heartsDB.Close()
	metricsDB, _, _ := sqlmock.New()
	defer metricsDB.Close()
	deploysDB, _, _ := sqlmock.New()
	defer deploysDB.Close()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET("/api/v1/agents/:account", ListAccountAgents(log, index, accountStore,
		heartstore.New(heartsDB), metricsstore.New(metricsDB), deploymentstore.NewStore(deploysDB),
		nil, nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/testaccount", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := indexMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("index expectations: %v", err)
	}
}

func TestListAccountAgents_PublishersPopulated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := logger.New("error", "json")
	now := time.Now()

	// agentindex: returns one public agent with a latest version
	indexDB, indexMock, _ := sqlmock.New()
	defer indexDB.Close()
	index := agentindex.NewIndexWithDB(indexDB)
	indexMock.ExpectQuery("SELECT a.account_id, a.name, a.registry").
		WithArgs("test-account-id", "public", defaultBlueprintListLimit, 0).
		WillReturnRows(sqlmock.NewRows(blueprintAccountListColumns(true)).
			AddRow("test-account-id", "test-agent", "registry.example.com", "public", nil, now, now,
				"build-1", "ns", `{"name":"test"}`, "", "", "[]", now, now, 1, 1))

	// accountStore: GetByName returns the account; GetAccountsForUser returns a personal account for resolvePublishers
	accountDB, accountMock, _ := sqlmock.New()
	defer accountDB.Close()
	accountStore := account.NewAccountStore(accountDB)
	accountMock.ExpectQuery("SELECT a.id, a.name, a.type").
		WithArgs("testaccount").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "cluster_id", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}).
			AddRow("test-account-id", "testaccount", "personal", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, pq.StringArray(nil), pq.StringArray(nil)))
	accountMock.ExpectQuery("SELECT.*WHERE am.user_id").
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "created_at", "updated_at", "display_name"}).
			AddRow("user-account-id", "janesmith", "personal", "", now, now, ""))

	// auditStore: returns "user-1" as the publisher of "test-agent"
	auditDB, auditMock, _ := sqlmock.New()
	defer auditDB.Close()
	auditStore := auditlog.NewStore(auditDB)
	auditMock.ExpectQuery("SELECT resource_id, actor_id FROM audit_logs").
		WithArgs("test-account-id", "agent.register", "agent").
		WillReturnRows(sqlmock.NewRows([]string{"resource_id", "actor_id"}).
			AddRow("test-agent", "user-1"))

	// workos stub: resolves "user-1" to Jane Smith
	workos := &stubUserGetter{users: map[string]*auth.User{
		"user-1": {FirstName: "Jane", LastName: "Smith"},
	}}

	// helper stores — errors are ignored by the handler when they occur
	heartsDB, _, _ := sqlmock.New()
	defer heartsDB.Close()
	metricsDB, _, _ := sqlmock.New()
	defer metricsDB.Close()
	deploysDB, _, _ := sqlmock.New()
	defer deploysDB.Close()

	router := gin.New()
	router.GET("/api/v1/agents/:account", ListAccountAgents(log, index, accountStore,
		heartstore.New(heartsDB), metricsstore.New(metricsDB), deploymentstore.NewStore(deploysDB),
		nil, auditStore, workos))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/testaccount", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	agents, ok := body["agents"].([]any)
	if !ok || len(agents) == 0 {
		t.Fatal("expected at least one agent in response")
	}

	agentResp, _ := agents[0].(map[string]any)
	publishers, ok := agentResp["publishers"].([]any)
	if !ok || len(publishers) == 0 {
		t.Fatalf("expected publishers on agent, got: %v", agentResp["publishers"])
	}

	pub, _ := publishers[0].(map[string]any)
	if pub["name"] != "Jane Smith" {
		t.Errorf("expected publisher name 'Jane Smith', got %v", pub["name"])
	}
	if pub["account"] != "janesmith" {
		t.Errorf("expected publisher account 'janesmith', got %v", pub["account"])
	}

	if err := indexMock.ExpectationsWereMet(); err != nil {
		t.Errorf("index mock: %v", err)
	}
	if err := auditMock.ExpectationsWereMet(); err != nil {
		t.Errorf("audit mock: %v", err)
	}
	if err := accountMock.ExpectationsWereMet(); err != nil {
		t.Errorf("account mock: %v", err)
	}
}

func TestCreateBlueprint_EmitsActiveAgentsEvent(t *testing.T) {
	eventReceived := make(chan struct{}, 1)
	omServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/events" {
			eventReceived <- struct{}{}
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer omServer.Close()

	omClient := openmeter.NewClient(omServer.URL)

	router, index, indexMock := setupAgentTestRouter()
	log := logger.New("error", "json")

	omDB, omMock, _ := sqlmock.New()
	defer omDB.Close()
	omMock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM agents").
		WithArgs("test-account-id").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	router.POST("/api/v1/agents/:account", injectTestAccount(), CreateBlueprint(log, index, nil, nil, nil, omClient, omDB))

	indexMock.ExpectBegin()
	indexMock.ExpectExec("INSERT INTO agents").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	indexMock.ExpectExec("DELETE FROM agent_versions").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 0))
	indexMock.ExpectCommit()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/testaccount", strings.NewReader(`{"name":"my-agent"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	select {
	case <-eventReceived:
	case <-time.After(2 * time.Second):
		t.Error("timed out waiting for active_agents event")
	}

	if err := omMock.ExpectationsWereMet(); err != nil {
		t.Errorf("openmeter db mock: %v", err)
	}
	if err := indexMock.ExpectationsWereMet(); err != nil {
		t.Errorf("index mock: %v", err)
	}
}
