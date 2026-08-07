package deploymentstore

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestUpdateStatusWithTxCommitsCallbackAtomically(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE deployments`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO deployment_events`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO deployment_fga_sync`).WithArgs("dep_123").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = NewStore(db).UpdateStatusWithTx("dep_123", StatusUpdate{Status: StatusUndeployed}, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO deployment_fga_sync (deployment_id) VALUES ($1)`, "dep_123")
		return err
	})
	if err != nil {
		t.Fatalf("UpdateStatusWithTx() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateStatusWithTxRollsBackCallbackFailure(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck

	cause := errors.New("sync write failed")
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE deployments`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO deployment_events`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	err = NewStore(db).UpdateStatusWithTx("dep_123", StatusUpdate{Status: StatusUndeployed}, func(*sql.Tx) error {
		return cause
	})
	if !errors.Is(err, cause) {
		t.Fatalf("UpdateStatusWithTx() error = %v, want %v", err, cause)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateDisplayNameWithTxCommitsCallbackAtomically(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE deployments SET display_name`).
		WithArgs("dep_123", "Renamed agent").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE deployment_fga_sync`).
		WithArgs("dep_123").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = NewStore(db).UpdateDisplayNameWithTx("dep_123", "Renamed agent", func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE deployment_fga_sync SET updated_at = NOW() WHERE deployment_id = $1`, "dep_123")
		return err
	})
	if err != nil {
		t.Fatalf("UpdateDisplayNameWithTx() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateDisplayNameWithTxRollsBackCallbackFailure(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck

	cause := errors.New("sync write failed")
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE deployments SET display_name`).
		WithArgs("dep_123", "Renamed agent").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	err = NewStore(db).UpdateDisplayNameWithTx("dep_123", "Renamed agent", func(*sql.Tx) error {
		return cause
	})
	if !errors.Is(err, cause) {
		t.Fatalf("UpdateDisplayNameWithTx() error = %v, want %v", err, cause)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
