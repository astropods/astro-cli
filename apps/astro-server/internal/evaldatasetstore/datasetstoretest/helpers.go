// Package datasetstoretest provides sqlmock-based helpers for tests that
// exercise evaldatasetstore.Store. It is imported by test files in sibling
// packages so the column list and query expectations stay in one place when
// the eval_datasets schema evolves.
package datasetstoretest

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore"
)

// Columns is the column list returned by evaldatasetstore.Store.GetByDeploymentID.
var Columns = []string{
	"id", "deployment_id", "account_id", "langfuse_dataset_name", "item_count",
	"good_count", "bad_count", "created_at", "updated_at",
}

// ID returns the deterministic dataset row ID used by these sqlmock helpers.
func ID(depID string) string {
	return "dataset-" + depID
}

// NewMock builds a sqlmock-backed evaldatasetstore.Store.
func NewMock(t *testing.T) (sqlmock.Sqlmock, *evaldatasetstore.Store) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return mock, evaldatasetstore.NewStore(db)
}

// ExpectMissing queues a Get that returns no row.
func ExpectMissing(mock sqlmock.Sqlmock, depID string) {
	mock.ExpectQuery("SELECT .+ FROM eval_datasets").
		WithArgs(depID).
		WillReturnRows(sqlmock.NewRows(Columns))
}

// ExpectRow queues a Get that returns one eval_datasets row with explicit values.
func ExpectRow(mock sqlmock.Sqlmock, depID, datasetName string, itemCount, goodCount, badCount int) {
	mock.ExpectQuery("SELECT .+ FROM eval_datasets").
		WithArgs(depID).
		WillReturnRows(sqlmock.NewRows(Columns).AddRow(
			ID(depID), depID, "acct-1", datasetName, itemCount, goodCount, badCount, time.Now(), time.Now(),
		))
}

// ExpectExists queues a Get that returns a canonical eval-* row with zero counts.
func ExpectExists(mock sqlmock.Sqlmock, depID string) {
	ExpectRow(mock, depID, "eval-"+depID, 0, 0, 0)
}

// ExpectLegacyExists queues a Get that returns a pre-flip dep-* row with non-zero counts.
func ExpectLegacyExists(mock sqlmock.Sqlmock, depID string) {
	ExpectRow(mock, depID, "dep-"+depID, 2, 1, 1)
}

// ExpectCreate queues a Create insert for the canonical eval-* row.
func ExpectCreate(mock sqlmock.Sqlmock, depID, accountID string) {
	mock.ExpectExec("INSERT INTO eval_datasets").
		WithArgs(depID, accountID, "eval-"+depID).
		WillReturnResult(sqlmock.NewResult(1, 1))
}

// ExpectRepoint queues a Repoint update that flips the row to the canonical eval-* name.
func ExpectRepoint(mock sqlmock.Sqlmock, depID string) {
	mock.ExpectExec("UPDATE eval_datasets").
		WithArgs("eval-"+depID, depID).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

// LangfuseStatusServer returns an httptest.Server that responds to POST with
// the given status, and a *bool that becomes true on the first POST. Useful
// for stubbing langfuse.Client.CreateDataset in tests.
func LangfuseStatusServer(t *testing.T, status int) (*httptest.Server, *bool) {
	t.Helper()
	called := new(bool)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			*called = true
			w.WriteHeader(status)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, called
}
