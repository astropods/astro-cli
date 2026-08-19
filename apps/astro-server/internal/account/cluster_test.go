package account

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestSetClusterID_Assign(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	store := NewAccountStore(db)

	mock.ExpectExec("UPDATE accounts SET cluster_id").
		WithArgs(sqlmock.AnyArg(), "acct-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.SetClusterID("acct-1", "eu-west-1-managed"); err != nil {
		t.Fatalf("SetClusterID: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSetClusterID_Clear(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	store := NewAccountStore(db)

	mock.ExpectExec("UPDATE accounts SET cluster_id").
		WithArgs(sqlmock.AnyArg(), "acct-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.SetClusterID("acct-1", ""); err != nil {
		t.Fatalf("SetClusterID clear: %v", err)
	}
}

func TestSetClusterID_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	store := NewAccountStore(db)

	mock.ExpectExec("UPDATE accounts SET cluster_id").
		WithArgs(sqlmock.AnyArg(), "missing").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = store.SetClusterID("missing", "eu-west-1-managed")
	if err == nil {
		t.Fatal("expected error for missing account")
	}
}

// The plan an account is billed on turns on this address, so the query must
// require a verified one and must pin to the creator. Joining across members
// would let a later member's domain decide, and dropping the verified filter
// would let a self-asserted domain earn free usage.
func TestGetCreatorVerifiedEmail_RequiresVerifiedAndResolvesInOneOrder(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	store := NewAccountStore(db)

	mock.ExpectQuery(`me\.verified\s+ORDER BY me\.created_at, me\.email`).
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow("owner@postman.com"))

	got, err := store.GetCreatorVerifiedEmail("acct-1")
	if err != nil {
		t.Fatalf("GetCreatorVerifiedEmail: %v", err)
	}
	if got != "owner@postman.com" {
		t.Errorf("email = %q, want owner@postman.com", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// No verified address is an answer, not a failure: the account takes the
// standard plan rather than the job retrying forever.
func TestGetCreatorVerifiedEmail_NoRowIsEmptyNotAnError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	store := NewAccountStore(db)

	mock.ExpectQuery(`me\.verified`).
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"email"}))

	got, err := store.GetCreatorVerifiedEmail("acct-1")
	if err != nil {
		t.Fatalf("GetCreatorVerifiedEmail: %v", err)
	}
	if got != "" {
		t.Errorf("email = %q, want empty", got)
	}
}
