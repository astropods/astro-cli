package experiment_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/experiment"
)

func TestStoreEnabledDefaultsMissingExperimentToFalse(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT enabled")).
		WithArgs("acct_123", experiment.FineGrainedAccess).
		WillReturnRows(sqlmock.NewRows([]string{"enabled"}))

	enabled, err := experiment.NewStore(db).Enabled(context.Background(), "acct_123", experiment.FineGrainedAccess)
	if err != nil || enabled {
		t.Fatalf("Enabled() = (%v, %v), want (false, nil)", enabled, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStoreReadsAndUpdatesExperiment(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT enabled")).
		WithArgs("acct_123", experiment.FineGrainedAccess).
		WillReturnRows(sqlmock.NewRows([]string{"enabled"}).AddRow(true))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO account_experiments")).
		WithArgs("acct_123", experiment.FineGrainedAccess, false).
		WillReturnResult(sqlmock.NewResult(0, 1))

	store := experiment.NewStore(db)
	enabled, err := store.Enabled(context.Background(), "acct_123", experiment.FineGrainedAccess)
	if err != nil || !enabled {
		t.Fatalf("Enabled() = (%v, %v), want (true, nil)", enabled, err)
	}
	if err := store.SetEnabled(context.Background(), "acct_123", experiment.FineGrainedAccess, false); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStoreRejectsUnconfiguredUse(t *testing.T) {
	t.Parallel()

	var store *experiment.Store
	if _, err := store.Enabled(context.Background(), "acct_123", experiment.FineGrainedAccess); err == nil {
		t.Fatal("Enabled() error = nil")
	}
	if err := store.SetEnabled(context.Background(), "acct_123", experiment.FineGrainedAccess, true); err == nil {
		t.Fatal("SetEnabled() error = nil")
	}
	if _, err := experiment.NewStore(nil).Enabled(context.Background(), "acct_123", experiment.FineGrainedAccess); err == nil {
		t.Fatal("Enabled() did not return an error")
	}
}
