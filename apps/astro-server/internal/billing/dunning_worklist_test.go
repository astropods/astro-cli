package billing

import (
	"context"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// ListInDunning is the sweep's whole work set. Widening it to suspended
// accounts would re-evaluate every stopped account on every tick; narrowing it
// past past_due would leave a failed payment running forever. Assert the status
// filter and the limit reach the query as arguments.
func TestListInDunning_SelectsPastDueUnderTheLimit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectQuery("FROM account_billing_status WHERE status").
		WithArgs(string(StatusPastDue), 500).
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow("acct_1").AddRow("acct_2"))

	ids, err := NewStatusStore(db, 7).ListInDunning(context.Background(), 500)
	if err != nil {
		t.Fatalf("ListInDunning: %v", err)
	}
	if len(ids) != 2 || ids[0] != "acct_1" || ids[1] != "acct_2" {
		t.Errorf("ids = %v, want [acct_1 acct_2]", ids)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("work set query is wrong: %v", err)
	}
}

// A row error mid-iteration must surface. Returning the partial slice with a nil
// error would silently shrink the work set, and the accounts that fell off it
// keep running unpaid with nothing to report why.
func TestListInDunning_ReportsAPartialRead(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	rows := sqlmock.NewRows([]string{"account_id"}).
		AddRow("acct_1").
		RowError(1, context.DeadlineExceeded)
	rows.AddRow("acct_2")
	mock.ExpectQuery("FROM account_billing_status WHERE status").WillReturnRows(rows)

	if _, err := NewStatusStore(db, 7).ListInDunning(context.Background(), 500); err == nil {
		t.Fatal("err = nil, want the row error: a truncated work set must not look complete")
	}
}
