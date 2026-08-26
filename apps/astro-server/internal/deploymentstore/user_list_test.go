package deploymentstore

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

var userDeploymentTestColumns = []string{
	"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace", "display_name",
	"status", "error_message", "deployed_at", "avatar_colors", "account_name", "access_ready",
}

func TestListVisibleDeploymentsForUserPageKeepsOneQueryForManyMemberships(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck
	store := NewStore(db)

	accountIDs := make([]string, 100)
	for i := range accountIDs {
		accountIDs[i] = fmt.Sprintf("00000000-0000-0000-0000-%012d", i+1)
	}
	mock.ExpectQuery(`(?s)FROM deployments d.*JOIN account_members am.*LIMIT \$8`).
		WithArgs("user-1", pq.Array(accountIDs), pq.Array([]string(nil)), pq.Array([]string(nil)), nil, nil, "", 51).
		WillReturnRows(sqlmock.NewRows(userDeploymentTestColumns))

	rows, err := store.ListVisibleDeploymentsForUserPage(
		context.Background(),
		"user-1",
		accountIDs,
		"",
		51,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("ListVisibleDeploymentsForUserPage: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %#v, want none", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListVisibleDeploymentsForUserPageUsesGlobalKeysetBoundary(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck
	store := NewStore(db)

	cursorTime := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	rowTime := cursorTime.Add(-time.Minute)
	mock.ExpectQuery(`(?s)JOIN account_members am.*am.user_id = \$1.*d.account_id = ANY.*ORDER BY d.deployed_at DESC, d.id DESC`).
		WithArgs(
			"user-1",
			pq.Array([]string{"11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222"}),
			pq.Array([]string(nil)),
			pq.Array([]string(nil)),
			cursorTime,
			"dep-cursor",
			"",
			51,
		).
		WillReturnRows(sqlmock.NewRows(userDeploymentTestColumns).AddRow(
			"dep-1", "11111111-1111-1111-1111-111111111111", nil, "agent", "build-1", "ns", "Agent",
			"active", nil, rowTime, nil, "alpha", true,
		))

	rows, err := store.ListVisibleDeploymentsForUserPage(
		context.Background(),
		"user-1",
		[]string{"11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222"},
		"",
		51,
		&UserDeploymentCursor{DeployedAt: cursorTime, ID: "dep-cursor"},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("ListVisibleDeploymentsForUserPage: %v", err)
	}
	if len(rows) != 1 || rows[0].Deployment.ID != "dep-1" || rows[0].AccountName != "alpha" || !rows[0].AccessReady {
		t.Fatalf("rows = %#v, want dep-1 in alpha", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListVisibleDeploymentsForUserPageSearchesNamesBeforePagination(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck
	store := NewStore(db)

	accountIDs := []string{"11111111-1111-1111-1111-111111111111"}
	mock.ExpectQuery(`(?s)FROM deployments d.*strpos\(lower\(d.agent_name\), lower\(\$7\)\).*strpos\(lower\(COALESCE\(d.display_name, ''\)\), lower\(\$7\)\).*ORDER BY d.deployed_at DESC, d.id DESC.*LIMIT \$8`).
		WithArgs("user-1", pq.Array(accountIDs), pq.Array([]string(nil)), pq.Array([]string(nil)), nil, nil, "support", 51).
		WillReturnRows(sqlmock.NewRows(userDeploymentTestColumns))

	rows, err := store.ListVisibleDeploymentsForUserPage(
		context.Background(), "user-1", accountIDs, "support", 51, nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("ListVisibleDeploymentsForUserPage: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %#v, want none", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListVisibleDeploymentIDsForUserKeepsAuthorizationInOneQuery(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck
	store := NewStore(db)

	requested := []string{
		"abc-def-ghi",
		"jkl-mno-pqr",
	}
	mock.ExpectQuery(`(?s)SELECT d.id.*JOIN account_members am.*am.user_id = \$1.*JOIN accounts a.*a.deleted_at IS NULL.*d.id = ANY\(\$2::varchar\[\]\).*d.status <> 'undeployed'`).
		WithArgs("user-1", pq.Array(requested), pq.Array([]string(nil)), pq.Array([]string(nil)), false).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).
			AddRow(requested[1]))

	visible, err := store.ListVisibleDeploymentIDsForUser(context.Background(), "user-1", requested)
	if err != nil {
		t.Fatalf("ListVisibleDeploymentIDsForUser: %v", err)
	}
	if len(visible) != 1 || visible[0] != requested[1] {
		t.Fatalf("visible = %#v, want only %s", visible, requested[1])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListReadableDeploymentIDsForUserAppliesFGAScope(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck
	store := NewStore(db)

	requested := []string{"abc-def-ghi", "jkl-mno-pqr"}
	fgaAccounts := []string{"11111111-1111-1111-1111-111111111111"}
	readable := []string{"abc-def-ghi"}
	mock.ExpectQuery(`(?s)SELECT d.id.*deployment_fga_sync.*desired_state = 'registered'.*d.id = ANY\(\$4::varchar\[\]\)`).
		WithArgs("user-1", pq.Array(requested), pq.Array(fgaAccounts), pq.Array(readable), false).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(readable[0]))

	visible, err := store.ListReadableDeploymentIDsForUser(
		context.Background(), "user-1", requested, fgaAccounts, readable,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 1 || visible[0] != readable[0] {
		t.Fatalf("visible = %#v", visible)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListReadableDeploymentHistoryIDsForUserIncludesUndeployed(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck
	store := NewStore(db)

	deploymentID := "abc-def-ghi"
	fgaAccountID := "11111111-1111-1111-1111-111111111111"
	mock.ExpectQuery(`(?s)SELECT d.id.*\$5::boolean OR d.status <> 'undeployed'.*deployment_fga_sync.*d.id = ANY\(\$4::varchar\[\]\)`).
		WithArgs(
			"user-1",
			pq.Array([]string{deploymentID}),
			pq.Array([]string{fgaAccountID}),
			pq.Array([]string{deploymentID}),
			true,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(deploymentID))

	visible, err := store.ListReadableDeploymentHistoryIDsForUser(
		context.Background(),
		"user-1",
		[]string{deploymentID},
		[]string{fgaAccountID},
		[]string{deploymentID},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 1 || visible[0] != deploymentID {
		t.Fatalf("visible history = %#v, want %q", visible, deploymentID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountsWithManagedDeploymentsUsesOneQuery(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck
	store := NewStore(db)

	accountIDs := []string{
		"11111111-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222",
	}
	mock.ExpectQuery(`(?s)SELECT DISTINCT d.account_id.*deployment_fga_sync.*authorization_resource_sync.*desired_state = 'registered'`).
		WithArgs(pq.Array(accountIDs)).
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow(accountIDs[0]))

	managed, err := store.AccountsWithManagedDeployments(context.Background(), accountIDs)
	if err != nil {
		t.Fatal(err)
	}
	if len(managed) != 1 || managed[0] != accountIDs[0] {
		t.Fatalf("managed = %#v", managed)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
