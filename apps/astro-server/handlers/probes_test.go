package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

func newProbeTestHandler(t *testing.T) (*ProbeHandler, sqlmock.Sqlmock) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewProbeHandler(logger.New("error", "json"), agentindex.NewIndexWithDB(db), nil), mock
}

func serveProbe(t *testing.T, path string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	router := gin.New()
	router.GET(path, handler)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// sqlmock rejects any query it was not told to expect, so a 200 proves
// readiness issued none.
func TestReadyzIssuesNoQuery(t *testing.T) {
	h, mock := newProbeTestHandler(t)

	rec := serveProbe(t, "/readyz", h.Readyz())

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestReadyzReportsShutdown(t *testing.T) {
	h, _ := newProbeTestHandler(t)
	h.SetReady(false)

	rec := serveProbe(t, "/readyz", h.Readyz())

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLivezIssuesNoQuery(t *testing.T) {
	h, mock := newProbeTestHandler(t)

	rec := serveProbe(t, "/livez", h.Livez())

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestHealthzPingsRatherThanLists(t *testing.T) {
	h, mock := newProbeTestHandler(t)
	mock.ExpectPing()

	rec := serveProbe(t, "/healthz", h.Healthz())

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("healthz did not ping: %v", err)
	}
}
