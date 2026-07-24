package knowledgestore

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestListByAccountPageUsesDeterministicOrdering(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet SQL expectations: %v", err)
		}
		_ = db.Close()
	})

	mock.ExpectQuery(`ORDER BY created_at DESC, id DESC`).
		WithArgs("acct-1", 50, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	stores, total, err := NewStore(db).ListByAccountPage(
		context.Background(),
		"acct-1",
		50,
		0,
	)
	if err != nil {
		t.Fatalf("ListByAccountPage: %v", err)
	}
	if total != 0 || len(stores) != 0 {
		t.Fatalf("stores = %#v, total = %d; want empty page", stores, total)
	}
}
