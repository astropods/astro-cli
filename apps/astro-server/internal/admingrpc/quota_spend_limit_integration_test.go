//go:build integration

package admingrpc

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/quota"
	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
)

// The owner has to be a member in the same statement: accounts_owner_member_fkey
// points back at account_members.
func seedSpendLimitAccount(t *testing.T, db *sql.DB, name string) string {
	t.Helper()
	var id string
	if err := db.QueryRow(`
		WITH acct AS (
			INSERT INTO accounts (name, owner_user_id)
			VALUES ($1, 'test-owner')
			RETURNING id
		), member AS (
			INSERT INTO account_members (account_id, user_id) SELECT id, 'test-owner' FROM acct
		)
		SELECT id FROM acct`, name,
	).Scan(&id); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM accounts WHERE id = $1`, id) })
	return id
}

func spendLimitTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL must be set for integration tests")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestSpendLimitRequest_RequestToApprovalToRaisedCeiling(t *testing.T) {
	db := spendLimitTestDB(t)
	ctx := context.Background()

	accountID := seedSpendLimitAccount(t, db, "spend-limit-chain-test")

	before, err := quota.SpendCeilingUSD(ctx, db, accountID)
	if err != nil {
		t.Fatalf("read ceiling before: %v", err)
	}
	if before != 1000 {
		t.Fatalf("ceiling before = %v, want the self-serve 1000", before)
	}

	var requestID string
	if err := db.QueryRow(
		`INSERT INTO quota_increase_requests
		   (account_id, feature_key, current_usage, current_quota, requested_amount, reason, requested_by)
		 VALUES ($1, $2, 812.4, 1000, 5000, 'monthly batch run', 'test-owner') RETURNING id`,
		accountID, quota.KeySpendLimit,
	).Scan(&requestID); err != nil {
		t.Fatalf("file request: %v", err)
	}

	srv := &Server{db: db, log: logger.New("error", "json")}
	resp, err := srv.ApproveQuotaIncrease(ctx, &adminv1.ApproveQuotaIncreaseRequest{
		RequestID:   requestID,
		GrantAmount: 5000,
		Note:        "approved for the batch run",
	})
	if err != nil {
		t.Fatalf("ApproveQuotaIncrease: %v", err)
	}
	if resp.Status != "approved" {
		t.Fatalf("status = %q, want approved", resp.Status)
	}

	after, err := quota.SpendCeilingUSD(ctx, db, accountID)
	if err != nil {
		t.Fatalf("read ceiling after: %v", err)
	}
	if after != 5000 {
		t.Errorf("ceiling after = %v, want the granted 5000", after)
	}

	list, err := srv.ListQuotaIncreaseRequests(ctx, &adminv1.ListQuotaIncreaseRequestsRequest{Status: "approved"})
	if err != nil {
		t.Fatalf("ListQuotaIncreaseRequests: %v", err)
	}
	var found *adminv1.QuotaIncreaseRequestProto
	for _, r := range list.Requests {
		if r.ID == requestID {
			found = r
			break
		}
	}
	if found == nil {
		t.Fatal("the approved request is missing from the list Queen reads")
	}
	if found.FeatureKey != quota.KeySpendLimit {
		t.Errorf("feature_key = %q, want %q", found.FeatureKey, quota.KeySpendLimit)
	}
	if found.GrantAmount != 5000 {
		t.Errorf("grant_amount = %v, want 5000", found.GrantAmount)
	}
	if found.ResolutionNote != "approved for the batch run" {
		t.Errorf("resolution_note = %q, want the note the admin left", found.ResolutionNote)
	}
}

func TestSpendLimitRequest_DenialLeavesTheCeilingAlone(t *testing.T) {
	db := spendLimitTestDB(t)
	ctx := context.Background()

	accountID := seedSpendLimitAccount(t, db, "spend-limit-deny-test")

	var requestID string
	if err := db.QueryRow(
		`INSERT INTO quota_increase_requests
		   (account_id, feature_key, requested_amount, reason, requested_by)
		 VALUES ($1, $2, 5000, 'monthly batch run', 'test-owner') RETURNING id`,
		accountID, quota.KeySpendLimit,
	).Scan(&requestID); err != nil {
		t.Fatalf("file request: %v", err)
	}

	srv := &Server{db: db, log: logger.New("error", "json")}
	if _, err := srv.DenyQuotaIncrease(ctx, &adminv1.DenyQuotaIncreaseRequest{
		RequestID: requestID,
		Note:      "no card on file",
	}); err != nil {
		t.Fatalf("DenyQuotaIncrease: %v", err)
	}

	after, err := quota.SpendCeilingUSD(ctx, db, accountID)
	if err != nil {
		t.Fatalf("read ceiling: %v", err)
	}
	if after != 1000 {
		t.Errorf("ceiling = %v, want the self-serve 1000 after a denial", after)
	}
}
