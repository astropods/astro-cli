package deploymentstore

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGetVisibleDeploymentsByAccountPage(t *testing.T) {
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

	now := time.Now()
	mock.ExpectQuery(`ORDER BY deployed_at DESC, id DESC`).
		WithArgs("acct-1", 1, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace", "display_name",
			"deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
			"status", "error_message", "error_details", "status_changed_at", "current_revision",
			"deployed_at", "undeployed_at", "avatar_colors", "avatar_updated_at", "count",
		}).AddRow(
			"deployment-2", "acct-1", nil, "second", "build-2", "namespace-2", "Second",
			`{}`, nil, nil, "cluster-2",
			"active", nil, []byte(`{"reason":"test"}`), now, 2,
			now, nil, nil, nil, 3,
		))

	deployments, total, err := NewStore(db).GetVisibleDeploymentsByAccountPage(
		context.Background(),
		"acct-1",
		1,
		1,
	)
	if err != nil {
		t.Fatalf("GetVisibleDeploymentsByAccountPage: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	if len(deployments) != 1 || deployments[0].ID != "deployment-2" {
		t.Fatalf("deployments = %#v, want deployment-2", deployments)
	}
	if deployments[0].EffectiveClusterID() != "cluster-2" {
		t.Fatalf("cluster = %q, want cluster-2", deployments[0].EffectiveClusterID())
	}
}

func TestGetVisibleDeploymentsByAccountPagePreservesTotalPastLastPage(t *testing.T) {
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
		WithArgs("acct-1", 50, 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "count"}))
	mock.ExpectQuery(`SELECT COUNT\(\*\)`).
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	deployments, total, err := NewStore(db).GetVisibleDeploymentsByAccountPage(
		context.Background(),
		"acct-1",
		50,
		100,
	)
	if err != nil {
		t.Fatalf("GetVisibleDeploymentsByAccountPage: %v", err)
	}
	if len(deployments) != 0 {
		t.Fatalf("deployments = %#v, want empty page", deployments)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
}

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
