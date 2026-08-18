//go:build integration

package e2e

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	ds "github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
)

func TestUserDeploymentKeysetPaginationIsTimeZoneNeutral(t *testing.T) {
	db := testDB(t)
	// set_config is connection-local. Keep this test on one real Postgres
	// connection so both page reads use the requested session setting.
	db.SetMaxOpenConns(1)

	ctx := context.Background()
	accountName := "user-list-" + deployid.New()
	var accountID string
	if err := db.QueryRowContext(ctx, `
		INSERT INTO accounts (name, type)
		VALUES ($1, 'personal')
		RETURNING id
	`, accountName).Scan(&accountID); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM accounts WHERE id = $1", accountID)
	})

	userID := "user-list-" + deployid.New()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO account_members (account_id, user_id)
		VALUES ($1, $2)
	`, accountID, userID); err != nil {
		t.Fatalf("insert account membership: %v", err)
	}

	boundary := time.Date(2026, time.August, 3, 12, 0, 0, 123456000, time.UTC)
	type row struct {
		id         string
		deployedAt time.Time
	}
	wantRows := []row{
		{id: deployid.New(), deployedAt: boundary.Add(time.Minute)},
		{id: deployid.New(), deployedAt: boundary},
		{id: deployid.New(), deployedAt: boundary},
		{id: deployid.New(), deployedAt: boundary.Add(-time.Minute)},
	}
	for _, item := range wantRows {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO deployments (
				id, account_id, agent_name, build_id, namespace,
				deployment_spec_json, status, deployed_at
			)
			VALUES ($1, $2, 'agent', 'build', 'namespace', '{}', 'active', $3::timestamp)
		`, item.id, accountID, item.deployedAt.Format("2006-01-02 15:04:05.999999999")); err != nil {
			t.Fatalf("insert deployment %s: %v", item.id, err)
		}
	}
	sort.Slice(wantRows, func(i, j int) bool {
		if wantRows[i].deployedAt.Equal(wantRows[j].deployedAt) {
			return wantRows[i].id > wantRows[j].id
		}
		return wantRows[i].deployedAt.After(wantRows[j].deployedAt)
	})
	wantIDs := make([]string, 0, len(wantRows))
	for _, item := range wantRows {
		wantIDs = append(wantIDs, item.id)
	}

	store := ds.NewStore(db)
	visibleIDs, err := store.ListVisibleDeploymentIDsForUser(ctx, userID, wantIDs)
	if err != nil {
		t.Fatalf("authorize compact deployment IDs: %v", err)
	}
	sort.Strings(visibleIDs)
	sortedWantIDs := append([]string(nil), wantIDs...)
	sort.Strings(sortedWantIDs)
	if len(visibleIDs) != len(sortedWantIDs) {
		t.Fatalf("visible compact IDs = %v, want %v", visibleIDs, sortedWantIDs)
	}
	for i := range sortedWantIDs {
		if visibleIDs[i] != sortedWantIDs[i] {
			t.Fatalf("visible compact IDs = %v, want %v", visibleIDs, sortedWantIDs)
		}
	}

	for _, timeZone := range []string{"UTC", "Asia/Kathmandu"} {
		t.Run(timeZone, func(t *testing.T) {
			if _, err := db.ExecContext(
				ctx,
				"SELECT set_config('TimeZone', $1, false)",
				timeZone,
			); err != nil {
				t.Fatalf("set TimeZone: %v", err)
			}

			firstPage, err := store.ListVisibleDeploymentsForUserPage(
				ctx, userID, []string{accountID}, "", 2, nil, nil, nil,
			)
			if err != nil {
				t.Fatalf("first page: %v", err)
			}
			if len(firstPage) != 2 {
				t.Fatalf("first page has %d rows, want 2", len(firstPage))
			}

			boundaryRow := firstPage[len(firstPage)-1].Deployment
			secondPage, err := store.ListVisibleDeploymentsForUserPage(
				ctx,
				userID,
				[]string{accountID},
				"",
				2,
				&ds.UserDeploymentCursor{DeployedAt: boundaryRow.DeployedAt, ID: boundaryRow.ID},
				nil,
				nil,
			)
			if err != nil {
				t.Fatalf("second page: %v", err)
			}
			if len(secondPage) != 2 {
				t.Fatalf("second page has %d rows, want 2", len(secondPage))
			}

			gotIDs := make([]string, 0, 4)
			for _, page := range [][]ds.UserDeployment{firstPage, secondPage} {
				for _, deployment := range page {
					gotIDs = append(gotIDs, deployment.Deployment.ID)
				}
			}
			for i := range wantIDs {
				if gotIDs[i] != wantIDs[i] {
					t.Fatalf("combined page IDs = %v, want %v", gotIDs, wantIDs)
				}
			}
		})
	}
}
