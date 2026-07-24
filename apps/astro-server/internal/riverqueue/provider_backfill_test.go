package riverqueue

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

func newProviderBackfillWorker(t *testing.T) (*ProviderBackfillWorker, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	// The two provider UPDATEs come from map iteration, so their order isn't
	// deterministic; match expectations by SQL+args rather than position.
	mock.MatchExpectationsInOrder(false)
	return &ProviderBackfillWorker{db: db, log: logger.New("error", "json")}, mock
}

const selectDeploymentsRe = `SELECT d\.id, d\.deployment_spec_json`

func TestProviderBackfill_SetsProviderFromSpecByKey(t *testing.T) {
	w, mock := newProviderBackfillWorker(t)
	ctx := context.Background()

	// Two knowledge stores with different providers, one model, and a custom
	// store with no provider — exercises correct per-key mapping and settling.
	spec := `{"models":{"m":{"provider":"anthropic"}},"knowledge":{"pg":{"provider":"postgres"},"vec":{"provider":"qdrant"},"custom":{}}}`

	mock.ExpectQuery(selectDeploymentsRe).WithArgs("", 200).
		WillReturnRows(sqlmock.NewRows([]string{"id", "deployment_spec_json"}).AddRow("dep-1", spec))
	mock.ExpectQuery(selectDeploymentsRe).WithArgs("dep-1", 200).
		WillReturnRows(sqlmock.NewRows([]string{"id", "deployment_spec_json"}))

	// Each declared provider updates its own row, matched by component_key.
	mock.ExpectExec(`UPDATE deployment_workloads\s+SET provider = \$1`).
		WithArgs("anthropic", "dep-1", "model", "m").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE deployment_workloads\s+SET provider = \$1`).
		WithArgs("postgres", "dep-1", "knowledge", "pg").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE deployment_workloads\s+SET provider = \$1`).
		WithArgs("qdrant", "dep-1", "knowledge", "vec").WillReturnResult(sqlmock.NewResult(0, 1))
	// The provider-less "custom" store is settled to '' so the deployment stops
	// matching the scan.
	mock.ExpectExec(`SET provider = ''`).
		WithArgs("dep-1").WillReturnResult(sqlmock.NewResult(0, 1))

	if err := w.Work(ctx, &river.Job[ProviderBackfillArgs]{}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestProviderBackfill_QueryErrorIsReturnedForRetry(t *testing.T) {
	w, mock := newProviderBackfillWorker(t)

	// A run that fails because the column isn't in place must surface the error
	// so River retries it, not report success and silently skip the backfill.
	mock.ExpectQuery(selectDeploymentsRe).WithArgs("", 200).
		WillReturnError(errors.New(`column "provider" does not exist`))

	if err := w.Work(context.Background(), &river.Job[ProviderBackfillArgs]{}); err == nil {
		t.Fatal("expected Work to return the query error so River retries")
	}
}

func TestProviderBackfill_NoDeploymentsIsNoOp(t *testing.T) {
	w, mock := newProviderBackfillWorker(t)
	ctx := context.Background()

	mock.ExpectQuery(selectDeploymentsRe).WithArgs("", 200).
		WillReturnRows(sqlmock.NewRows([]string{"id", "deployment_spec_json"}))

	if err := w.Work(ctx, &river.Job[ProviderBackfillArgs]{}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
