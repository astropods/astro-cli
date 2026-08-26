package authorizationadmin

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

func TestCreateResetRejectsActiveOperation(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(`INSERT INTO authorization_admin_operations`).
		WillReturnError(&pq.Error{Code: "23505", Constraint: activeOperationConstraint})
	_, err = NewStore(db).CreateReset(context.Background(), "acct_123", true, nil)
	if !errors.Is(err, ErrOperationInProgress) {
		t.Fatalf("CreateReset() error = %v, want ErrOperationInProgress", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
