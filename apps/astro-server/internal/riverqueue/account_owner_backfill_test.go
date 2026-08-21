package riverqueue

import (
	"context"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

const (
	soleMemberRe  = `UPDATE accounts a\s+SET owner_user_id = m\.user_id`
	unownedScanRe = `SELECT a\.id, a\.name, ao\.workos_org_id`
	unownedCntRe  = `SELECT count\(\*\) FROM accounts WHERE owner_user_id IS NULL`
)

func ownerBackfillWorker(t *testing.T) (*AccountOwnerBackfillWorker, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &AccountOwnerBackfillWorker{db: db, log: logger.New("error", "json")}, mock
}

func runOwnerBackfill(t *testing.T, w *AccountOwnerBackfillWorker) error {
	t.Helper()
	return w.Work(context.Background(), nil)
}

// A single member cannot be the wrong owner, so that case resolves in one
// statement and never reaches WorkOS.
func TestAccountOwnerBackfill_AdoptsSoleMember(t *testing.T) {
	w, mock := ownerBackfillWorker(t)

	mock.ExpectExec(soleMemberRe).WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectQuery(unownedCntRe).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	if err := runOwnerBackfill(t, w); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected statements: %v", err)
	}
}

// Without a WorkOS client the multi-member accounts cannot be decided. They must
// be reported rather than guessed at, so a later pass still has work to find.
func TestAccountOwnerBackfill_ReportsUnresolvedWithoutWorkOS(t *testing.T) {
	w, mock := ownerBackfillWorker(t)

	mock.ExpectExec(soleMemberRe).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(unownedCntRe).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(4))

	if err := runOwnerBackfill(t, w); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected statements: %v", err)
	}
}

// A failed pass has to surface. Swallowing it would log "completed" over a
// column that is still short of rows.
func TestAccountOwnerBackfill_ReturnsErrorOnFailedUpdate(t *testing.T) {
	w, mock := ownerBackfillWorker(t)

	mock.ExpectExec(soleMemberRe).WillReturnError(errors.New("boom"))

	if err := runOwnerBackfill(t, w); err == nil {
		t.Fatal("expected the failure to propagate so River retries")
	}
}
