package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

func setupPutAgentEvaluationSetRouter(t *testing.T) (*gin.Engine, sqlmock.Sqlmock) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	router := gin.New()
	log := logger.New("error", "json")
	router.PUT("/api/v1/agents/:account/:name/evaluation-set", injectTestAccount(), PutAgentEvaluationSet(log, db))
	return router, mock
}

func putAgentEvaluationSet(router *gin.Engine, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, "/api/v1/agents/testaccount/test-agent/evaluation-set", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestPutAgentEvaluationSet_ActivatesCustomSet(t *testing.T) {
	router, mock := setupPutAgentEvaluationSetRouter(t)

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO eval_definitions").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO agent_evaluations").
		WithArgs("test-account-id", "test-agent", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	body := `{
		"evaluation_yaml": "schema: evaluation/v1\nevaluators:\n  - ref: preset/exposed-pii\n"
	}`
	rec := putAgentEvaluationSet(router, body)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp AgentEvaluationActivationResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.EvaluationRef)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPutAgentEvaluationSet_RejectsInvalidContent(t *testing.T) {
	router, mock := setupPutAgentEvaluationSetRouter(t)

	body := `{
		"evaluation_yaml": "schema: evaluation/v2\nevaluators:\n  - ref: preset/exposed-pii\n"
	}`
	rec := putAgentEvaluationSet(router, body)

	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPutAgentEvaluationSet_RejectsNullBody(t *testing.T) {
	router, mock := setupPutAgentEvaluationSetRouter(t)

	rec := putAgentEvaluationSet(router, "null")

	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPutAgentEvaluationSet_RejectsEmptyBody(t *testing.T) {
	router, mock := setupPutAgentEvaluationSetRouter(t)

	rec := putAgentEvaluationSet(router, `{}`)

	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())
}
