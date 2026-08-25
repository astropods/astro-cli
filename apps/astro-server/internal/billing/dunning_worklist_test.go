package billing

import (
	"context"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// ListForRecompute is the sweep's whole work set. Suspended accounts stay out,
// or every stopped account is re-evaluated on every tick; narrowing it past
// past_due would leave a failed payment running forever.
func TestListForRecompute_SelectsPastDueUnderTheLimit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectQuery(`FROM account_billing_status\s+WHERE \(status`).
		WithArgs(string(StatusPastDue), string(StatusActive), "{}", 500).
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow("acct_1").AddRow("acct_2"))

	ids, err := NewStatusStore(db, 7).ListForRecompute(context.Background(), 500)
	if err != nil {
		t.Fatalf("ListForRecompute: %v", err)
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
func TestListForRecompute_ReportsAPartialRead(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	rows := sqlmock.NewRows([]string{"account_id"}).
		AddRow("acct_1").
		RowError(1, context.DeadlineExceeded)
	rows.AddRow("acct_2")
	mock.ExpectQuery(`FROM account_billing_status\s+WHERE \(status`).WillReturnRows(rows)

	if _, err := NewStatusStore(db, 7).ListForRecompute(context.Background(), 500); err == nil {
		t.Fatal("err = nil, want the row error: a truncated work set must not look complete")
	}
}

// An account that was delinquent while it was exempt is stored active with its
// flags still raised. Without the second half of the predicate it sits in no
// work set at all, so removing the exemption leaves it running unpaid forever.
func TestListForRecompute_CatchesAnActiveRowWhoseFlagsDisagree(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	// The query decides this, so assert the shape of the predicate itself: an
	// active row is only work when a flag contradicts it.
	mock.ExpectQuery(`status = \$2 AND \(`).
		WithArgs(string(StatusPastDue), string(StatusActive), "{}", 500).
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow("acct_drifted"))

	ids, err := NewStatusStore(db, 7).ListForRecompute(context.Background(), 500)
	if err != nil {
		t.Fatalf("ListForRecompute: %v", err)
	}
	if len(ids) != 1 || ids[0] != "acct_drifted" {
		t.Errorf("ids = %v, want [acct_drifted]", ids)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("the predicate does not reach active rows: %v", err)
	}
}

// A carded account with spent credits is legitimately active. Selecting it would
// put a permanent row in the work set and re-read it on every tick forever.
func TestListForRecompute_LeavesLegitimatelyActiveRowsOut(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectQuery(`credits_exhausted AND NOT has_payment_method`).
		WithArgs(string(StatusPastDue), string(StatusActive), "{}", 500).
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}))

	ids, err := NewStatusStore(db, 7).ListForRecompute(context.Background(), 500)
	if err != nil {
		t.Fatalf("ListForRecompute: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("ids = %v, want none", ids)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("the card is not part of the credits predicate: %v", err)
	}
}

// An exempt account matches the drift half forever, and Recompute leaves its
// updated_at alone, so it would sort to the front of this bounded set on every
// tick and hold a slot real work needs. reconcileExempt covers those accounts.
func TestListForRecompute_ExcludesExemptAccounts(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectQuery(`account_id::text <> ALL`).
		WithArgs(string(StatusPastDue), string(StatusActive), `{"acct_a","acct_b"}`, 500).
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow("acct_other"))

	// Sorted, so the array is stable regardless of map iteration order.
	store := NewStatusStore(db, 7).WithExemptAccounts([]string{"acct_b", "acct_a"})
	ids, err := store.ListForRecompute(context.Background(), 500)
	if err != nil {
		t.Fatalf("ListForRecompute: %v", err)
	}
	if len(ids) != 1 || ids[0] != "acct_other" {
		t.Errorf("ids = %v, want [acct_other]", ids)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("exempt accounts are not excluded: %v", err)
	}
}

// With nothing exempt the parameter has to be an empty array, not NULL.
// `x <> ALL(NULL)` is NULL, so a nil would match no rows and silently empty the
// work set: every delinquent account would stop being swept.
func TestListForRecompute_NoExemptionsPassesAnEmptyArrayNotNull(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectQuery(`account_id::text <> ALL`).
		WithArgs(string(StatusPastDue), string(StatusActive), "{}", 500).
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow("acct_1"))

	ids, err := NewStatusStore(db, 7).ListForRecompute(context.Background(), 500)
	if err != nil {
		t.Fatalf("ListForRecompute: %v", err)
	}
	if len(ids) != 1 {
		t.Errorf("ids = %v, want the row: a NULL array would have matched nothing", ids)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("the exempt parameter is not an empty array: %v", err)
	}
}
