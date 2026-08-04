package deploymentstore

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGetVisibleDeploymentsByAccountUsesPageOrdering(t *testing.T) {
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

	mock.ExpectQuery(`ORDER BY deployed_at DESC, id DESC`).
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	deployments, err := NewStore(db).GetVisibleDeploymentsByAccount("acct-1")
	if err != nil {
		t.Fatalf("GetVisibleDeploymentsByAccount: %v", err)
	}
	if len(deployments) != 0 {
		t.Fatalf("deployments = %#v, want none", deployments)
	}
}
