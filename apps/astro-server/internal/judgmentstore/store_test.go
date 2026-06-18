package judgmentstore

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestInsertRejectsInvalidVerdict(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewStore(db)
	err = store.Insert("dataset-1", "trace-1", Verdict("maybe"))
	if err == nil || !strings.Contains(err.Error(), `invalid verdict "maybe"`) {
		t.Fatalf("Insert error = %v, want invalid verdict", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestInsertReturnsAlreadyJudgedOnConflict(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectExec("INSERT INTO eval_dataset_judgments").
		WithArgs("dataset-1", "trace-1", "good").
		WillReturnResult(sqlmock.NewResult(0, 0))

	store := NewStore(db)
	if err := store.Insert("dataset-1", "trace-1", VerdictGood); err != ErrAlreadyJudged {
		t.Fatalf("Insert error = %v, want ErrAlreadyJudged", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestDelete(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectExec("DELETE FROM eval_dataset_judgments").
		WithArgs("dataset-1", "trace-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	store := NewStore(db)
	if err := store.Delete("dataset-1", "trace-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
