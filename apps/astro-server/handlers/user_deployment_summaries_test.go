package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"

	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/obssummary"
)

func TestListUserDeploymentSummariesReturnsOnlyAuthorizedCachedEntries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	visibleID := "abc-def-ghi"
	hiddenID := "jkl-mno-pqr"
	mock.ExpectQuery(`(?s)SELECT d.id.*JOIN account_members am.*JOIN accounts a.*d.id = ANY\(\$2::varchar\[\]\)`).
		WithArgs("user-1", pq.Array([]string{visibleID, hiddenID})).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(visibleID))

	cache := mapCache{}
	seedCache(t, cache, visibleID, &obssummary.Entry{TotalTraces: 11})
	seedCache(t, cache, hiddenID, &obssummary.Entry{TotalTraces: 99})

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET("/api/v1/me/deployment-summaries", ListUserDeploymentSummaries(
		logger.New("error", "json"),
		deploymentstore.NewStore(db),
		cache,
	))

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/me/deployment-summaries?deployment="+visibleID+"&deployment="+hiddenID,
		nil,
	)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var response DeploymentSummariesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(response.Summaries) != 1 || response.Summaries[visibleID].TotalTraces != 11 {
		t.Fatalf("summaries = %#v, want only visible deployment", response.Summaries)
	}
	if _, ok := response.Summaries[hiddenID]; ok {
		t.Fatal("hidden deployment summary leaked into response")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
