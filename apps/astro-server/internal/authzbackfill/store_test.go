package authzbackfill

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestStoreBackfillsBlueprintIDsInBatches(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck
	mock.ExpectExec("WITH batch AS").WithArgs(2).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("WITH batch AS").WithArgs(2).WillReturnResult(sqlmock.NewResult(0, 1))

	count, err := NewStore(db).BackfillBlueprintIDs(context.Background(), 2, false)
	if err != nil || count != 3 {
		t.Fatalf("BackfillBlueprintIDs() = %d, %v", count, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStoreDryRunCountsBlueprintIDsWithoutWriting(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck
	mock.ExpectQuery("SELECT count").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(4))

	count, err := NewStore(db).BackfillBlueprintIDs(context.Background(), 100, true)
	if err != nil || count != 4 {
		t.Fatalf("BackfillBlueprintIDs() = %d, %v", count, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
