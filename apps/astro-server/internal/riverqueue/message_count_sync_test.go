package riverqueue

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestMessageCountSyncArgs_Kind(t *testing.T) {
	args := MessageCountSyncArgs{}
	if kind := args.Kind(); kind != "metrics.message_count_sync" {
		t.Errorf("Kind() = %q, want %q", kind, "metrics.message_count_sync")
	}
}

func TestUpsertMessageCount_Insert(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	mock.ExpectExec("INSERT INTO agent_message_counts").
		WithArgs("acct-1", "bot", 100.0).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := upsertMessageCount(t.Context(), db, "acct-1", "bot", 100.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestUpsertMessageCount_Update(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	// Simulates an update (ON CONFLICT path) — same SQL, different value
	mock.ExpectExec("INSERT INTO agent_message_counts").
		WithArgs("acct-1", "bot", 200.0).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := upsertMessageCount(t.Context(), db, "acct-1", "bot", 200.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestUpsertMessageCount_CounterReset(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	// A lower value than before simulates a counter reset (pod restart)
	mock.ExpectExec("INSERT INTO agent_message_counts").
		WithArgs("acct-1", "bot", 5.0).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := upsertMessageCount(t.Context(), db, "acct-1", "bot", 5.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
