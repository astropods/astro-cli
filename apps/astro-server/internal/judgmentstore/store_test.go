package judgmentstore

import (
	"database/sql"
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

func TestDeleteReturningVerdict(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery("DELETE FROM eval_dataset_judgments").
		WithArgs("dataset-1", "trace-1").
		WillReturnRows(sqlmock.NewRows([]string{"verdict"}).AddRow("bad"))

	store := NewStore(db)
	verdict, found, err := store.DeleteReturningVerdict("dataset-1", "trace-1")
	if err != nil {
		t.Fatalf("DeleteReturningVerdict: %v", err)
	}
	if !found {
		t.Fatal("DeleteReturningVerdict found = false, want true")
	}
	if verdict != VerdictBad {
		t.Fatalf("DeleteReturningVerdict verdict = %q, want bad", verdict)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestDeleteReturningVerdictNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery("DELETE FROM eval_dataset_judgments").
		WithArgs("dataset-1", "trace-missing").
		WillReturnError(sql.ErrNoRows)

	store := NewStore(db)
	_, found, err := store.DeleteReturningVerdict("dataset-1", "trace-missing")
	if err != nil {
		t.Fatalf("DeleteReturningVerdict: %v", err)
	}
	if found {
		t.Fatal("DeleteReturningVerdict found = true, want false")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestUpdateReturningPrevious(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery("WITH previous AS").
		WithArgs("dataset-1", "trace-1", "bad").
		WillReturnRows(sqlmock.NewRows([]string{"verdict"}).AddRow("good"))

	store := NewStore(db)
	previous, found, err := store.UpdateReturningPrevious("dataset-1", "trace-1", VerdictBad)
	if err != nil {
		t.Fatalf("UpdateReturningPrevious: %v", err)
	}
	if !found {
		t.Fatal("UpdateReturningPrevious found = false, want true")
	}
	if previous != VerdictGood {
		t.Fatalf("UpdateReturningPrevious previous = %q, want good", previous)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestUpdateReturningPreviousNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery("WITH previous AS").
		WithArgs("dataset-1", "trace-missing", "bad").
		WillReturnError(sql.ErrNoRows)

	store := NewStore(db)
	_, found, err := store.UpdateReturningPrevious("dataset-1", "trace-missing", VerdictBad)
	if err != nil {
		t.Fatalf("UpdateReturningPrevious: %v", err)
	}
	if found {
		t.Fatal("UpdateReturningPrevious found = true, want false")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestCriterionDimensionValid(t *testing.T) {
	for _, d := range CriterionDimensions {
		if !d.Valid() {
			t.Errorf("CriterionDimensions entry %q is not Valid()", d)
		}
	}
	if CriterionDimension("nonsense").Valid() {
		t.Fatal("unknown dimension reported Valid() = true")
	}
}

func TestCriterionCounts(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery("FROM eval_dataset_judgment_reasons").
		WithArgs("dataset-1").
		WillReturnRows(sqlmock.NewRows([]string{"dimension_key", "good_count", "bad_count"}).
			AddRow("accuracy", 2, 1).
			AddRow("tone", 0, 3))

	store := NewStore(db)
	counts, err := store.CriterionCounts("dataset-1")
	if err != nil {
		t.Fatalf("CriterionCounts: %v", err)
	}
	want := []CriterionCounts{
		{Dimension: DimensionAccuracy, GoodCount: 2, BadCount: 1},
		{Dimension: DimensionTone, GoodCount: 0, BadCount: 3},
	}
	if len(counts) != len(want) {
		t.Fatalf("CriterionCounts len = %d, want %d", len(counts), len(want))
	}
	for i, count := range counts {
		if count != want[i] {
			t.Errorf("CriterionCounts[%d] = %+v, want %+v", i, count, want[i])
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
