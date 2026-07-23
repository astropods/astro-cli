package riverqueue

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

func TestAccountPurgeArgs_Kind(t *testing.T) {
	if kind := (AccountPurgeArgs{}).Kind(); kind != "account.purge" {
		t.Errorf("Kind() = %q, want %q", kind, "account.purge")
	}
}

func newPurgeWorker(t *testing.T) (*AccountPurgeWorker, sqlmock.Sqlmock, sqlmock.Sqlmock) {
	t.Helper()

	db, dbMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	deployDB, deployMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))

	w := &AccountPurgeWorker{
		db:            db,
		deployStore:   deploymentstore.NewStore(deployDB),
		retentionDays: 7,
		log:           logger.New("error", "json"),
		enqueueUndeploy: func(_ context.Context, _ string) error {
			return nil
		},
	}
	return w, dbMock, deployMock
}

var purgeDeployColumns = []string{
	"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace",
	"display_name", "deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
	"status", "error_message", "error_details", "status_changed_at", "current_revision",
	"deployed_at", "undeployed_at", "avatar_colors", "avatar_updated_at",
}

func TestAccountPurge_NoDeletedAccounts(t *testing.T) {
	w, dbMock, _ := newPurgeWorker(t)

	// No accounts past retention
	dbMock.ExpectQuery(`SELECT id FROM accounts WHERE deleted_at IS NOT NULL`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	err := w.Work(context.Background(), &river.Job[AccountPurgeArgs]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := dbMock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestAccountPurge_PurgesAccountWithNoDeployments(t *testing.T) {
	w, dbMock, deployMock := newPurgeWorker(t)

	// One account past retention
	dbMock.ExpectQuery(`SELECT id FROM accounts WHERE deleted_at IS NOT NULL`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("acct-1"))

	// No pending deployments
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(purgeDeployColumns))

	// Hard-delete
	dbMock.ExpectExec(`DELETE FROM accounts WHERE id`).
		WithArgs("acct-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := w.Work(context.Background(), &river.Job[AccountPurgeArgs]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := dbMock.ExpectationsWereMet(); err != nil {
		t.Errorf("db unmet: %v", err)
	}
	if err := deployMock.ExpectationsWereMet(); err != nil {
		t.Errorf("deploy unmet: %v", err)
	}
}

func TestAccountPurge_SkipsAccountWithPendingTeardown(t *testing.T) {
	var enqueuedIDs []string
	w, dbMock, deployMock := newPurgeWorker(t)
	w.enqueueUndeploy = func(_ context.Context, id string) error {
		enqueuedIDs = append(enqueuedIDs, id)
		return nil
	}

	dbMock.ExpectQuery(`SELECT id FROM accounts WHERE deleted_at IS NOT NULL`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("acct-1"))

	// One deployment still active (not undeployed)
	now := time.Now()
	rev := 1
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(purgeDeployColumns).AddRow(
			"dep-1", "acct-1", nil, "agent", "build-1", "ns-1",
			"Agent", `{}`, nil, nil, nil,
			"active", nil, json.RawMessage(nil), now, &rev,
			now, nil, nil, nil,
		))

	// Should NOT call DELETE FROM accounts — account is skipped
	err := w.Work(context.Background(), &river.Job[AccountPurgeArgs]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have re-enqueued undeploy for the active deployment
	if len(enqueuedIDs) != 1 || enqueuedIDs[0] != "dep-1" {
		t.Errorf("expected undeploy re-enqueued for dep-1, got %v", enqueuedIDs)
	}

	if err := dbMock.ExpectationsWereMet(); err != nil {
		t.Errorf("db unmet: %v", err)
	}
}

func TestAccountPurge_SkipsReenqueueForAlreadyUndeploying(t *testing.T) {
	var enqueuedIDs []string
	w, dbMock, deployMock := newPurgeWorker(t)
	w.enqueueUndeploy = func(_ context.Context, id string) error {
		enqueuedIDs = append(enqueuedIDs, id)
		return nil
	}

	dbMock.ExpectQuery(`SELECT id FROM accounts WHERE deleted_at IS NOT NULL`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("acct-1"))

	// Deployment already in undeploying state
	now := time.Now()
	rev := 1
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(purgeDeployColumns).AddRow(
			"dep-1", "acct-1", nil, "agent", "build-1", "ns-1",
			"Agent", `{}`, nil, nil, nil,
			"undeploying", nil, json.RawMessage(nil), now, &rev,
			now, nil, nil, nil,
		))

	err := w.Work(context.Background(), &river.Job[AccountPurgeArgs]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should NOT re-enqueue — already undeploying
	if len(enqueuedIDs) != 0 {
		t.Errorf("expected no re-enqueue for already undeploying dep, got %v", enqueuedIDs)
	}
}

func TestAccountPurge_ContinuesOnPerAccountError(t *testing.T) {
	w, dbMock, deployMock := newPurgeWorker(t)

	// Two accounts past retention
	dbMock.ExpectQuery(`SELECT id FROM accounts WHERE deleted_at IS NOT NULL`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).
			AddRow("acct-1").
			AddRow("acct-2"))

	// First account: deploy query fails
	deployMock.ExpectQuery(`SELECT`).
		WillReturnError(fmt.Errorf("db error"))

	// Second account: succeeds
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(purgeDeployColumns))
	dbMock.ExpectExec(`DELETE FROM accounts WHERE id`).
		WithArgs("acct-2").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := w.Work(context.Background(), &river.Job[AccountPurgeArgs]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := dbMock.ExpectationsWereMet(); err != nil {
		t.Errorf("db unmet: %v", err)
	}
	if err := deployMock.ExpectationsWereMet(); err != nil {
		t.Errorf("deploy unmet: %v", err)
	}
}
