package evaldatasetstore

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestBumpCountsByIDReturnsErrorWhenDatasetMissing(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	mock.ExpectExec("UPDATE eval_datasets").
		WithArgs(1, 0, "missing-dataset").
		WillReturnResult(sqlmock.NewResult(0, 0))

	store := NewStore(db)
	err = store.BumpCountsByID("missing-dataset", 1, 0)
	if err == nil {
		t.Fatal("BumpCountsByID returned nil error, want missing dataset error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %v, want not found message", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
